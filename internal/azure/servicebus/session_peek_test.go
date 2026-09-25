package servicebus

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus"
)

type fakeSession struct {
	id        string
	seqs      []int64
	closed    bool
	from      *int64
	max       int
	completed []int64
	abandoned []int64
}

func (f *fakeSession) SessionID() string { return f.id }

func (f *fakeSession) PeekMessages(_ context.Context, maxCount int, opts *azservicebus.PeekMessagesOptions) ([]*azservicebus.ReceivedMessage, error) {
	f.max = maxCount
	var from int64
	if opts != nil && opts.FromSequenceNumber != nil {
		f.from = opts.FromSequenceNumber
		from = *opts.FromSequenceNumber
	}
	var out []*azservicebus.ReceivedMessage
	for _, s := range f.seqs {
		if s < from || len(out) == maxCount {
			continue
		}
		seq := s
		id := f.id
		out = append(out, &azservicebus.ReceivedMessage{MessageID: "m", SequenceNumber: &seq, SessionID: &id})
	}
	return out, nil
}

func (f *fakeSession) Close(context.Context) error { f.closed = true; return nil }

// fakeAccepter hands out sessions in order, then reports the broker's
// "no session available" timeout.
func fakeAccepter(sessions []*fakeSession) sessionAccepter {
	i := 0
	return func(ctx context.Context) (sessionHandle, error) {
		if i >= len(sessions) {
			return nil, &azservicebus.Error{Code: azservicebus.CodeTimeout}
		}
		s := sessions[i]
		i++
		return s, nil
	}
}

func collect(t *testing.T, accept sessionAccepter, maxCount int, from int64) []PeekedMessage {
	t.Helper()
	var got []PeekedMessage
	if err := peekSessions(context.Background(), accept, maxCount, from, func(b []PeekedMessage) { got = append(got, b...) }); err != nil {
		t.Fatalf("peekSessions: %v", err)
	}
	return got
}

// Messages from every session come back in one batch ordered by
// sequence number, and every accepted session is released.
func TestPeekSessionsMergesAcrossSessions(t *testing.T) {
	sessions := []*fakeSession{
		{id: "a", seqs: []int64{1, 4, 9}},
		{id: "b", seqs: []int64{2, 3}},
		{id: "c", seqs: []int64{5}},
	}
	got := collect(t, fakeAccepter(sessions), 50, 0)

	want := []int64{1, 2, 3, 4, 5, 9}
	if len(got) != len(want) {
		t.Fatalf("got %d messages, want %d", len(got), len(want))
	}
	for i, m := range got {
		if m.SequenceNumber != want[i] {
			t.Errorf("position %d: seq %d, want %d", i, m.SequenceNumber, want[i])
		}
	}
	if got[1].SessionID != "b" {
		t.Errorf("session ID not carried: %+v", got[1])
	}
	for _, s := range sessions {
		if !s.closed {
			t.Errorf("session %s left open", s.id)
		}
	}
}

// The union is cut to maxCount after sorting, so the batch is the
// lowest sequence numbers across sessions, not the first session's.
func TestPeekSessionsTruncatesAfterMerge(t *testing.T) {
	sessions := []*fakeSession{
		{id: "a", seqs: []int64{10, 11, 12}},
		{id: "b", seqs: []int64{1, 2}},
	}
	got := collect(t, fakeAccepter(sessions), 3, 0)
	if len(got) != 3 || got[0].SequenceNumber != 1 || got[2].SequenceNumber != 10 {
		t.Fatalf("truncated batch = %+v, want seqs 1,2,10", got)
	}
	// Each session was asked for the full maxCount so the cut-off is
	// exact.
	if sessions[0].max != 3 {
		t.Errorf("per-session peek asked for %d, want maxCount 3", sessions[0].max)
	}
}

// "Peek more" passes the starting sequence number into every session.
func TestPeekSessionsFromSequenceNumber(t *testing.T) {
	sessions := []*fakeSession{
		{id: "a", seqs: []int64{1, 2, 7}},
		{id: "b", seqs: []int64{3, 8}},
	}
	got := collect(t, fakeAccepter(sessions), 50, 5)
	if len(got) != 2 || got[0].SequenceNumber != 7 || got[1].SequenceNumber != 8 {
		t.Fatalf("from=5 batch = %+v, want seqs 7,8", got)
	}
	for _, s := range sessions {
		if s.from == nil || *s.from != 5 {
			t.Errorf("session %s did not receive FromSequenceNumber 5", s.id)
		}
	}
}

// An entity with no available sessions peeks as empty, not as an error.
func TestPeekSessionsNoSessions(t *testing.T) {
	got := collect(t, fakeAccepter(nil), 50, 0)
	if len(got) != 0 {
		t.Fatalf("expected no messages, got %d", len(got))
	}
}

// Our own accept deadline also ends the walk cleanly.
func TestPeekSessionsOwnDeadlineEndsWalk(t *testing.T) {
	s := &fakeSession{id: "a", seqs: []int64{1}}
	calls := 0
	accept := func(ctx context.Context) (sessionHandle, error) {
		calls++
		if calls == 1 {
			return s, nil
		}
		return nil, context.DeadlineExceeded
	}
	got := collect(t, accept, 50, 0)
	if len(got) != 1 || !s.closed {
		t.Fatalf("got %d messages, closed=%v", len(got), s.closed)
	}
}

// Any other accept failure is an error, and held sessions still close.
func TestPeekSessionsAcceptErrorReleasesSessions(t *testing.T) {
	s := &fakeSession{id: "a", seqs: []int64{1}}
	calls := 0
	accept := func(ctx context.Context) (sessionHandle, error) {
		calls++
		if calls == 1 {
			return s, nil
		}
		return nil, errors.New("boom")
	}
	err := peekSessions(context.Background(), accept, 50, 0, func([]PeekedMessage) { t.Fatal("sent on error") })
	if err == nil || err.Error() != "accept session: boom" {
		t.Fatalf("err = %v", err)
	}
	if !s.closed {
		t.Error("session left open after error")
	}
}

// A cancelled caller wins over the "no more sessions" reading.
func TestPeekSessionsCallerCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	accept := func(ctx context.Context) (sessionHandle, error) { return nil, context.DeadlineExceeded }
	err := peekSessions(ctx, accept, 50, 0, func([]PeekedMessage) {})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

// The walk stops at the session cap rather than locking forever.
func TestPeekSessionsCap(t *testing.T) {
	calls := 0
	accept := func(ctx context.Context) (sessionHandle, error) {
		calls++
		return &fakeSession{id: "s", seqs: []int64{int64(calls)}}, nil
	}
	got := collect(t, accept, 1000, 0)
	if calls != maxSessionsPerPeek || len(got) != maxSessionsPerPeek {
		t.Fatalf("accepted %d sessions, got %d messages, want %d", calls, len(got), maxSessionsPerPeek)
	}
}

// --- receive walk ---

func (f *fakeSession) ReceiveMessages(_ context.Context, maxCount int, _ *azservicebus.ReceiveMessagesOptions) ([]*azservicebus.ReceivedMessage, error) {
	if len(f.seqs) == 0 {
		return nil, context.DeadlineExceeded
	}
	msgs, _ := f.PeekMessages(context.Background(), maxCount, nil)
	return msgs, nil
}

func (f *fakeSession) CompleteMessage(_ context.Context, msg *azservicebus.ReceivedMessage, _ *azservicebus.CompleteMessageOptions) error {
	f.completed = append(f.completed, *msg.SequenceNumber)
	return nil
}

func (f *fakeSession) AbandonMessage(_ context.Context, msg *azservicebus.ReceivedMessage, _ *azservicebus.AbandonMessageOptions) error {
	f.abandoned = append(f.abandoned, *msg.SequenceNumber)
	return nil
}

// Each locked message settles through the session that locked it, and
// Close releases every session with its unsettled messages abandoned.
func TestReceiveSessionsSettlesThroughOwningSession(t *testing.T) {
	a := &fakeSession{id: "a", seqs: []int64{1, 2}}
	b := &fakeSession{id: "b", seqs: []int64{3}}
	locked, err := receiveSessions(context.Background(), fakeAccepter([]*fakeSession{a, b}), 50)
	if err != nil {
		t.Fatalf("receiveSessions: %v", err)
	}
	if locked.Len() != 3 {
		t.Fatalf("locked %d messages, want 3", locked.Len())
	}

	snap := locked.MessagesSnapshot()
	var bID string
	for _, m := range snap {
		if *m.raw.SequenceNumber == 3 {
			bID = m.OperationID()
		}
	}
	if err := locked.CompleteByID(context.Background(), bID); err != nil {
		t.Fatalf("CompleteByID: %v", err)
	}
	if len(b.completed) != 1 || b.completed[0] != 3 || len(a.completed) != 0 {
		t.Fatalf("complete went to the wrong session: a=%v b=%v", a.completed, b.completed)
	}

	locked.Close(context.Background())
	if !a.closed || !b.closed {
		t.Error("Close did not release every session")
	}
	if len(a.abandoned) != 2 || len(b.abandoned) != 0 {
		t.Errorf("abandon on close: a=%v b=%v, want a's two unsettled only", a.abandoned, b.abandoned)
	}
}

// The walk stops at maxCount and asks each session only for what is
// still needed.
func TestReceiveSessionsStopsAtMaxCount(t *testing.T) {
	a := &fakeSession{id: "a", seqs: []int64{1, 2, 3}}
	b := &fakeSession{id: "b", seqs: []int64{4, 5}}
	c := &fakeSession{id: "c", seqs: []int64{6}}
	locked, err := receiveSessions(context.Background(), fakeAccepter([]*fakeSession{a, b, c}), 4)
	if err != nil {
		t.Fatalf("receiveSessions: %v", err)
	}
	if locked.Len() != 4 {
		t.Fatalf("locked %d, want 4", locked.Len())
	}
	if b.max != 1 {
		t.Errorf("second session asked for %d, want the remaining 1", b.max)
	}
	if c.closed || c.max != 0 {
		t.Error("third session should never have been accepted")
	}
}

// A session with nothing receivable is skipped but stays held.
func TestReceiveSessionsSkipsEmptySession(t *testing.T) {
	empty := &fakeSession{id: "empty"}
	full := &fakeSession{id: "full", seqs: []int64{7}}
	locked, err := receiveSessions(context.Background(), fakeAccepter([]*fakeSession{empty, full}), 50)
	if err != nil {
		t.Fatalf("receiveSessions: %v", err)
	}
	if locked.Len() != 1 {
		t.Fatalf("locked %d, want 1", locked.Len())
	}
	locked.Close(context.Background())
	if !empty.closed {
		t.Error("empty session was not released on Close")
	}
}

// An accept failure releases what was already held.
func TestReceiveSessionsAcceptErrorReleases(t *testing.T) {
	a := &fakeSession{id: "a", seqs: []int64{1}}
	calls := 0
	accept := func(ctx context.Context) (sessionHandle, error) {
		calls++
		if calls == 1 {
			return a, nil
		}
		return nil, errors.New("boom")
	}
	if _, err := receiveSessions(context.Background(), accept, 50); err == nil {
		t.Fatal("expected error")
	}
	if !a.closed {
		t.Error("held session not released after accept error")
	}
}

// --- connection recovery ---

// A connection-lost failure rebuilds the namespace's client and runs
// the operation once more; anything else is returned as is.
func TestWithSessionRecoveryRebuildsClientOnce(t *testing.T) {
	svc := NewService(testCredential{id: "a"})
	ns := Namespace{Name: "ns", FQDN: "ns.servicebus.windows.net"}

	var seen []*azservicebus.Client
	connLost := &azservicebus.Error{Code: azservicebus.CodeConnectionLost}
	err := svc.withSessionRecovery(context.Background(), ns, "op", func(c *azservicebus.Client) error {
		seen = append(seen, c)
		if len(seen) == 1 {
			return fmt.Errorf("accept session: %w", connLost)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("withSessionRecovery: %v", err)
	}
	if len(seen) != 2 || seen[0] == seen[1] {
		t.Fatalf("expected a second attempt on a fresh client, got %d attempts (same client: %v)", len(seen), len(seen) == 2 && seen[0] == seen[1])
	}
	if svc.clients[ns.FQDN] != seen[1] {
		t.Error("the fresh client was not cached for later calls")
	}

	// Two failures in a row: the second is surfaced, not retried again.
	calls := 0
	err = svc.withSessionRecovery(context.Background(), ns, "op", func(c *azservicebus.Client) error {
		calls++
		return fmt.Errorf("accept session: %w", connLost)
	})
	if !isConnectionLost(err) || calls != 2 {
		t.Fatalf("err = %v after %d calls, want connection lost after exactly 2", err, calls)
	}

	// A different error does not trigger a rebuild.
	before := svc.clients[ns.FQDN]
	calls = 0
	err = svc.withSessionRecovery(context.Background(), ns, "op", func(c *azservicebus.Client) error {
		calls++
		return errors.New("boom")
	})
	if err == nil || calls != 1 || svc.clients[ns.FQDN] != before {
		t.Fatalf("plain error: err=%v calls=%d rebuilt=%v", err, calls, svc.clients[ns.FQDN] != before)
	}
}
