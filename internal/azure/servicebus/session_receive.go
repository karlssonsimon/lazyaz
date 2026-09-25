package servicebus

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus"
)

// sessionReceiveTimeout bounds one session's receive. The SDK reports a
// session with nothing receivable as a deadline on this context.
const sessionReceiveTimeout = 10 * time.Second

// receiveSessions is Receive for a session-enabled entity: accept a
// session, receive inside it, keep the session open as the settler for
// its messages, repeat until maxCount messages are held or the accepts
// run dry. Every accepted session stays in the set until Close, for
// the same reason the peek walk holds them: releasing one early hands
// it straight back on the next accept.
func receiveSessions(ctx context.Context, accept sessionAccepter, maxCount int) (*ReceivedMessages, error) {
	var held []sessionHandle
	release := func() {
		cctx, cancel := context.WithTimeout(context.Background(), sessionCloseTimeout)
		defer cancel()
		for _, h := range held {
			_ = h.Close(cctx)
		}
	}

	var locked []LockedMessage
	for len(locked) < maxCount && len(held) < maxSessionsPerPeek {
		h, ok, err := acceptNext(ctx, accept)
		if err != nil {
			release()
			return nil, err
		}
		if !ok {
			break
		}
		held = append(held, h)

		rctx, rcancel := context.WithTimeout(ctx, sessionReceiveTimeout)
		messages, err := h.ReceiveMessages(rctx, maxCount-len(locked), &azservicebus.ReceiveMessagesOptions{
			TimeAfterFirstMessage: time.Second,
		})
		rcancel()
		if err != nil && len(messages) == 0 {
			if ctx.Err() != nil {
				release()
				return nil, fmt.Errorf("receive session %s: %w", h.SessionID(), ctx.Err())
			}
			if errors.Is(err, context.DeadlineExceeded) {
				continue // nothing receivable in this session; keep it held and move on
			}
			release()
			return nil, fmt.Errorf("receive session %s: %w", h.SessionID(), err)
		}
		locked = append(locked, lockReceivedMessages(messages, h)...)
	}

	receivers := make([]settler, len(held))
	for i, h := range held {
		receivers[i] = h
	}
	return &ReceivedMessages{messages: locked, receivers: receivers}, nil
}

// resendAllSessions is resendAll across a session-enabled source: each
// accepted session is drained into the target and released before the
// next is accepted. Releasing is safe here because a drained session
// has nothing left for the accept to hand back; if it does, that is
// new traffic and moving it too is the point.
func (s *Service) resendAllSessions(ctx context.Context, targetClient *azservicebus.Client, accept sessionAccepter, targetName string, batchSize, maxMessages int) (int, error) {
	total := 0
	for sessions := 0; sessions < maxSessionsPerPeek; sessions++ {
		if maxMessages > 0 && total >= maxMessages {
			break
		}
		h, ok, err := acceptNext(ctx, accept)
		if err != nil {
			return total, err
		}
		if !ok {
			break
		}
		remaining := 0
		if maxMessages > 0 {
			remaining = maxMessages - total
		}
		n, err := s.drainSession(ctx, targetClient, h, targetName, batchSize, remaining)
		total += n
		_ = h.Close(ctx)
		if err != nil {
			return total, fmt.Errorf("session %s: %w", h.SessionID(), err)
		}
	}
	return total, nil
}

// drainSession moves one session's messages. It peeks first and asks
// resendAll for exactly that many, so an emptied session is recognised
// at once rather than after resendAll's 15-second silent receive; with
// one or two messages per session, that wait dominated the walk and
// ran a move-all out of time. The peek continues from where the last
// one stopped, so a session holding more than a batch is drained in
// rounds. Fewer received than peeked means the rest are not receivable
// (scheduled or deferred), and the session is left rather than spun on.
func (s *Service) drainSession(ctx context.Context, targetClient *azservicebus.Client, h sessionHandle, targetName string, batchSize, remaining int) (int, error) {
	total := 0
	for {
		want := batchSize
		if remaining > 0 && remaining-total < want {
			want = remaining - total
		}
		if want <= 0 {
			return total, nil
		}
		peeked, err := h.PeekMessages(ctx, want, nil)
		if err != nil {
			return total, fmt.Errorf("peek: %w", err)
		}
		if len(peeked) == 0 {
			return total, nil
		}
		n, err := s.resendAll(ctx, targetClient, h, targetName, len(peeked), len(peeked))
		total += n
		if err != nil {
			return total, err
		}
		if n < len(peeked) {
			return total, nil
		}
	}
}
