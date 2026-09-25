package servicebus

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus"
)

// Session-enabled entities refuse a plain receiver, so peeking one
// means accepting sessions and peeking inside each. The walk holds
// every accepted session open until it is done: closing one early
// would let the broker hand it back on the next accept, and the walk
// could spin on a single session while never seeing the others. The
// cost is that the sessions are locked for the duration of the peek,
// which is a few seconds; live consumers wait that long for them.

const (
	// maxSessionsPerPeek bounds the walk. Beyond this the peek returns
	// what it has rather than locking an unbounded number of sessions.
	maxSessionsPerPeek = 64

	// sessionAcceptTimeout is how long one accept waits for a session
	// to become available. The broker otherwise waits close to a
	// minute before reporting that none are; the walk ends on the first
	// accept that times out, so this is also how long an exhausted
	// walk takes to notice.
	sessionAcceptTimeout = 4 * time.Second

	// sessionCloseTimeout bounds releasing the held sessions when the
	// caller's context is already gone.
	sessionCloseTimeout = 5 * time.Second
)

// sessionHandle is the slice of SessionReceiver the walks need, so
// they can be driven by a fake in tests. It is also a settler and a
// bulkReceiver, so locked messages and the move-all loop can use it
// in place of a plain receiver.
type sessionHandle interface {
	SessionID() string
	PeekMessages(ctx context.Context, maxMessageCount int, options *azservicebus.PeekMessagesOptions) ([]*azservicebus.ReceivedMessage, error)
	ReceiveMessages(ctx context.Context, maxMessages int, options *azservicebus.ReceiveMessagesOptions) ([]*azservicebus.ReceivedMessage, error)
	CompleteMessage(ctx context.Context, message *azservicebus.ReceivedMessage, options *azservicebus.CompleteMessageOptions) error
	AbandonMessage(ctx context.Context, message *azservicebus.ReceivedMessage, options *azservicebus.AbandonMessageOptions) error
	Close(ctx context.Context) error
}

// bulkReceiver is what resendAll needs from its source: a plain
// Receiver or a session handle.
type bulkReceiver interface {
	ReceiveMessages(ctx context.Context, maxMessages int, options *azservicebus.ReceiveMessagesOptions) ([]*azservicebus.ReceivedMessage, error)
	CompleteMessage(ctx context.Context, message *azservicebus.ReceivedMessage, options *azservicebus.CompleteMessageOptions) error
}

// sessionAccepter accepts the next available session of one entity.
type sessionAccepter func(ctx context.Context) (sessionHandle, error)

func queueSessionAccepter(client *azservicebus.Client, queueName string) sessionAccepter {
	return func(ctx context.Context) (sessionHandle, error) {
		return client.AcceptNextSessionForQueue(ctx, queueName, nil)
	}
}

func subscriptionSessionAccepter(client *azservicebus.Client, topicName, subName string) sessionAccepter {
	return func(ctx context.Context) (sessionHandle, error) {
		return client.AcceptNextSessionForSubscription(ctx, topicName, subName, nil)
	}
}

// sessionAccepterFor picks the queue or subscription accepter the same
// way newReceiver picks its receiver: subName == "" means a queue.
func sessionAccepterFor(client *azservicebus.Client, entityName, subName string) sessionAccepter {
	if subName == "" {
		return queueSessionAccepter(client, entityName)
	}
	return subscriptionSessionAccepter(client, entityName, subName)
}

// acceptNext runs one accept under the accept timeout and classifies
// the outcome: a handle, the end of the walk (ok == false, err == nil),
// or a real error.
func acceptNext(ctx context.Context, accept sessionAccepter) (h sessionHandle, ok bool, err error) {
	actx, cancel := context.WithTimeout(ctx, sessionAcceptTimeout)
	defer cancel()
	h, err = accept(actx)
	if err == nil {
		return h, true, nil
	}
	if ctx.Err() != nil {
		return nil, false, fmt.Errorf("accept session: %w", ctx.Err())
	}
	if noMoreSessions(err) {
		return nil, false, nil
	}
	return nil, false, fmt.Errorf("accept session: %w", err)
}

// peekSessions walks the entity's sessions and sends up to maxCount
// messages, ordered by sequence number across sessions like a plain
// peek. Each session is peeked for up to maxCount from
// fromSequenceNumber, so the union's first maxCount is complete: any
// message below the cut-off in a session is among that session's
// first maxCount. That keeps "peek more" from the last sequence number
// exact, the same as on a non-session entity.
func peekSessions(ctx context.Context, accept sessionAccepter, maxCount int, fromSequenceNumber int64, send func([]PeekedMessage)) error {
	var held []sessionHandle
	defer func() {
		cctx, cancel := context.WithTimeout(context.Background(), sessionCloseTimeout)
		defer cancel()
		for _, h := range held {
			_ = h.Close(cctx)
		}
	}()

	var opts *azservicebus.PeekMessagesOptions
	if fromSequenceNumber > 0 {
		opts = &azservicebus.PeekMessagesOptions{FromSequenceNumber: &fromSequenceNumber}
	}

	var messages []PeekedMessage
	for len(held) < maxSessionsPerPeek {
		h, ok, err := acceptNext(ctx, accept)
		if err != nil {
			return err
		}
		if !ok {
			break
		}
		held = append(held, h)

		peeked, err := h.PeekMessages(ctx, maxCount, opts)
		if err != nil {
			return fmt.Errorf("peek session %s: %w", h.SessionID(), err)
		}
		for _, msg := range peeked {
			messages = append(messages, peekedFromReceived(msg))
		}
	}

	sort.SliceStable(messages, func(i, j int) bool {
		return messages[i].SequenceNumber < messages[j].SequenceNumber
	})
	if len(messages) > maxCount {
		messages = messages[:maxCount]
	}
	if len(messages) > 0 {
		send(messages)
	}
	return nil
}

// noMoreSessions reports the accept failing because no session was
// available: either the broker's own timeout, or ours.
func noMoreSessions(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var sbErr *azservicebus.Error
	return errors.As(err, &sbErr) && sbErr.Code == azservicebus.CodeTimeout
}
