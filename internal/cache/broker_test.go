package cache

import (
	"context"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

func wrapNoop(p Page[string]) tea.Msg { return p }

// TestBrokerAbandonedSubscriberDoesNotWedgeStream is a regression test:
// subscribers legitimately stop draining their channel (handlers drop
// msg.Next for stale scopes; the parent drops pages for closed tabs).
// The worker must still finish the fetch and mark the stream Done —
// previously a full subscriber buffer blocked the worker forever and
// every later Subscribe joined the wedged "active" stream.
func TestBrokerAbandonedSubscriberDoesNotWedgeStream(t *testing.T) {
	b := NewBroker[string](NewMap[string](), func(s string) string { return s })

	// Subscribe but never run the returned cmd — the channel (cap 4) is
	// never drained, simulating an abandoned recv chain.
	fetchDone := make(chan struct{})
	cmd, _ := b.Subscribe("k", nil, func(ctx context.Context, send func([]string)) error {
		// Emit far more pages than the subscriber buffer holds. Sleep
		// past CoalesceInterval so each send flushes a page.
		for i := 0; i < 12; i++ {
			send([]string{"item"})
			time.Sleep(CoalesceInterval + 5*time.Millisecond)
		}
		close(fetchDone)
		return nil
	}, wrapNoop)
	_ = cmd // never invoked: subscriber is abandoned

	select {
	case <-fetchDone:
	case <-time.After(10 * time.Second):
		t.Fatal("fetch wedged: worker blocked on an abandoned subscriber")
	}

	// The stream must reach Done so future Subscribes start fresh
	// instead of joining a dead stream.
	deadline := time.Now().Add(5 * time.Second)
	for {
		infos := b.Streams()
		if len(infos) == 1 && infos[0].Status == StreamDone {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("stream never reached Done: %+v", infos)
		}
		time.Sleep(10 * time.Millisecond)
	}

	if got, ok := b.Get("k"); !ok || len(got) != 1 {
		t.Fatalf("store should hold the final snapshot, got %v ok=%v", got, ok)
	}
}

// TestBrokerLiveSubscriberGetsDonePage verifies drop-oldest delivery
// still hands a draining subscriber the final Done page with the full
// snapshot, even when intermediate pages were dropped.
func TestBrokerLiveSubscriberGetsDonePage(t *testing.T) {
	b := NewBroker[string](NewMap[string](), func(s string) string { return s })

	cmd, _ := b.Subscribe("k", nil, func(ctx context.Context, send func([]string)) error {
		for i := 0; i < 12; i++ {
			send([]string{string(rune('a' + i))})
			time.Sleep(CoalesceInterval + 5*time.Millisecond)
		}
		return nil
	}, wrapNoop)

	deadline := time.Now().Add(10 * time.Second)
	for {
		if time.Now().After(deadline) {
			t.Fatal("never received Done page")
		}
		msg := cmd()
		if msg == nil {
			t.Fatal("channel closed before Done page")
		}
		page := msg.(Page[string])
		if page.Done {
			if len(page.Items) != 12 {
				t.Fatalf("Done page should carry the full snapshot, got %d items", len(page.Items))
			}
			return
		}
		if page.Next == nil {
			t.Fatal("non-Done page missing Next")
		}
		cmd = page.Next
	}
}
