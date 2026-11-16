package servicebus

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus"
)

// Settlements overlap, but never more than settleConcurrency at a time.
func TestSettleEachRunsConcurrentlyWithinTheBound(t *testing.T) {
	const n = settleConcurrency * 4
	var inFlight, peak, calls atomic.Int32

	errs := settleEach(n, func(int) error {
		now := inFlight.Add(1)
		for {
			seen := peak.Load()
			if now <= seen || peak.CompareAndSwap(seen, now) {
				break
			}
		}
		time.Sleep(2 * time.Millisecond)
		inFlight.Add(-1)
		calls.Add(1)
		return nil
	})

	if len(errs) != n || calls.Load() != n {
		t.Fatalf("settled %d of %d, with %d results", calls.Load(), n, len(errs))
	}
	if got := peak.Load(); got < 2 || got > settleConcurrency {
		t.Errorf("peak in flight = %d, want between 2 and %d", got, settleConcurrency)
	}
}

func TestSettleEachReportsErrorsByIndex(t *testing.T) {
	boom := errors.New("boom")
	errs := settleEach(5, func(i int) error {
		if i == 3 {
			return boom
		}
		return nil
	})
	for i, err := range errs {
		if (i == 3) != (err != nil) {
			t.Errorf("errs[%d] = %v", i, err)
		}
	}
}

func lockedForTest(n int) *ReceivedMessages {
	locked := &ReceivedMessages{}
	for i := 0; i < n; i++ {
		seq := int64(i)
		locked.messages = append(locked.messages, LockedMessage{
			ID:     "m",
			LockID: string(rune('a' + i)),
			raw:    &azservicebus.ReceivedMessage{MessageID: "m", SequenceNumber: &seq},
		})
	}
	return locked
}

// One failed settlement does not stop the others: every message is
// attempted, the successes are reported in the order asked for, and only
// they are marked consumed.
func TestCompleteIDsAttemptsEveryMessage(t *testing.T) {
	locked := lockedForTest(4)
	expired := errors.New("lock expired")

	locked.mu.Lock()
	completed, err := locked.completeIDsLocked(context.Background(), []string{"d", "b", "a", "c"}, func(_ context.Context, msg LockedMessage) error {
		if msg.LockID == "b" {
			return expired
		}
		return nil
	})
	locked.mu.Unlock()

	if !errors.Is(err, expired) {
		t.Fatalf("err = %v, want the failed settlement's error", err)
	}
	if got := strings.Join(completed, ","); got != "d,a,c" {
		t.Errorf("completed = %q, want d,a,c", got)
	}
	for _, msg := range locked.messages {
		if consumed := msg.raw == nil; consumed != (msg.LockID != "b") {
			t.Errorf("message %s consumed = %v", msg.LockID, consumed)
		}
	}
}

func TestCompleteIDsReportsUnknownID(t *testing.T) {
	locked := lockedForTest(1)

	locked.mu.Lock()
	completed, err := locked.completeIDsLocked(context.Background(), []string{"a", "zz"}, func(context.Context, LockedMessage) error {
		return nil
	})
	locked.mu.Unlock()

	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("err = %v, want not found for the unknown ID", err)
	}
	if len(completed) != 1 || completed[0] != "a" {
		t.Errorf("completed = %v, want the known ID only", completed)
	}
}

// Messages matched by message ID alone can share an ID. Each request
// settles its own message, never the same one twice.
func TestCompleteIDsSettlesDuplicateIDsOnceEach(t *testing.T) {
	first := &azservicebus.ReceivedMessage{MessageID: "dup"}
	second := &azservicebus.ReceivedMessage{MessageID: "dup"}
	locked := &ReceivedMessages{messages: []LockedMessage{{ID: "dup", raw: first}, {ID: "dup", raw: second}}}

	var mu sync.Mutex
	seen := map[*azservicebus.ReceivedMessage]int{}
	locked.mu.Lock()
	completed, err := locked.completeIDsLocked(context.Background(), []string{"dup", "dup", "dup"}, func(_ context.Context, msg LockedMessage) error {
		mu.Lock()
		defer mu.Unlock()
		seen[msg.raw]++
		return nil
	})
	locked.mu.Unlock()

	if len(completed) != 2 || seen[first] != 1 || seen[second] != 1 {
		t.Errorf("completed = %v, settled first %d and second %d times; want each once", completed, seen[first], seen[second])
	}
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("err = %v, want not found for the third request", err)
	}
}

func TestCompleteAllCountsSuccesses(t *testing.T) {
	s := &fakeSession{id: "s"}
	var messages []*azservicebus.ReceivedMessage
	for i := 0; i < 40; i++ {
		seq := int64(i)
		messages = append(messages, &azservicebus.ReceivedMessage{MessageID: "m", SequenceNumber: &seq})
	}

	n, err := completeAll(context.Background(), s, messages)

	if err != nil || n != len(messages) || len(s.completed) != len(messages) {
		t.Fatalf("completeAll = %d, %v with %d settled; want all %d", n, err, len(s.completed), len(messages))
	}
}

// hasPeekable asks from the start of the entity every time, so it does not
// depend on where the receiver's own peek cursor stands.
func TestHasPeekablePeeksFromTheStart(t *testing.T) {
	s := &fakeSession{id: "s", seqs: []int64{7}}

	found, err := hasPeekable(context.Background(), s)

	if err != nil || !found {
		t.Fatalf("hasPeekable = %v, %v; want true", found, err)
	}
	if s.from == nil || *s.from != firstSequenceNumber || s.max != 1 {
		t.Errorf("peeked %d from %v, want 1 from sequence number %d", s.max, s.from, firstSequenceNumber)
	}

	empty := &fakeSession{id: "e"}
	if found, err := hasPeekable(context.Background(), empty); err != nil || found {
		t.Errorf("hasPeekable on an empty source = %v, %v; want false", found, err)
	}
}
