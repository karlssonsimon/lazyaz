package activity

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

// fakeActivity is a test-controllable Activity implementation.
type fakeActivity struct {
	mu       sync.Mutex
	id       string
	kind     Kind
	title    string
	snap     Snapshot
	cancelCh chan struct{}
}

func newFakeActivity(id string, k Kind) *fakeActivity {
	return &fakeActivity{
		id:       id,
		kind:     k,
		title:    "fake " + id,
		snap:     Snapshot{Status: StatusRunning, StartedAt: time.Unix(0, 0)},
		cancelCh: make(chan struct{}, 1),
	}
}

func (f *fakeActivity) ID() string       { return f.id }
func (f *fakeActivity) Kind() Kind       { return f.kind }
func (f *fakeActivity) Title() string    { f.mu.Lock(); defer f.mu.Unlock(); return f.title }
func (f *fakeActivity) Snapshot() Snapshot {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.snap
}
func (f *fakeActivity) Cancel() {
	select {
	case f.cancelCh <- struct{}{}:
	default:
	}
}
func (f *fakeActivity) setSnap(s Snapshot) { f.mu.Lock(); f.snap = s; f.mu.Unlock() }

func TestRegistryRegisterListsActivity(t *testing.T) {
	r := NewRegistry(NewFakeClock(time.Unix(0, 0)))
	a := newFakeActivity("a1", KindFetch)
	_ = r.Register(a)

	got := r.Snapshot()
	if len(got) != 1 {
		t.Fatalf("want 1 activity, got %d", len(got))
	}
	if got[0].Activity.ID() != "a1" {
		t.Fatalf("want a1, got %q", got[0].Activity.ID())
	}
	if got[0].Snapshot.Status != StatusRunning {
		t.Fatalf("want Running, got %v", got[0].Snapshot.Status)
	}
}

func TestRegistryUnregisterRemovesActivity(t *testing.T) {
	r := NewRegistry(NewFakeClock(time.Unix(0, 0)))
	a := newFakeActivity("a1", KindFetch)
	unreg := r.Register(a)
	unreg()

	if got := r.Snapshot(); len(got) != 0 {
		t.Fatalf("want 0 activities after unregister, got %d", len(got))
	}
}

func TestRegistryEventsFireOnRegister(t *testing.T) {
	r := NewRegistry(NewFakeClock(time.Unix(0, 0)))
	ch, cancel := r.Events()
	defer cancel()

	_ = r.Register(newFakeActivity("a1", KindFetch))

	select {
	case <-ch:
		// ok
	case <-time.After(1 * time.Second):
		t.Fatal("no event within 1s of Register")
	}
}

func TestRegistryEventsFireOnSnapshotChange(t *testing.T) {
	r := NewRegistry(NewFakeClock(time.Unix(0, 0)))
	a := newFakeActivity("a1", KindFetch)
	_ = r.Register(a)

	ch, cancel := r.Events()
	defer cancel()
	drain(ch) // drop the initial register event if any

	a.setSnap(Snapshot{Status: StatusRunning, Items: 42, StartedAt: time.Unix(0, 0)})

	select {
	case <-ch:
	case <-time.After(1 * time.Second):
		t.Fatal("no event within 1s of snapshot change")
	}
}

func TestRegistryEventsCoalesce(t *testing.T) {
	r := NewRegistry(NewFakeClock(time.Unix(0, 0)))
	a := newFakeActivity("a1", KindFetch)
	_ = r.Register(a)

	ch, cancel := r.Events()
	defer cancel()
	drain(ch)

	for i := 0; i < 50; i++ {
		a.setSnap(Snapshot{Status: StatusRunning, Items: i, StartedAt: time.Unix(0, 0)})
	}

	time.Sleep(400 * time.Millisecond)
	received := 0
	for {
		select {
		case <-ch:
			received++
		default:
			if received >= 50 {
				t.Fatalf("expected events to coalesce, got %d", received)
			}
			return
		}
	}
}

func TestRegistryKeepsAllTerminalsBelowCap(t *testing.T) {
	r := NewRegistry(NewFakeClock(time.Unix(1000, 0)))
	for i := 0; i < 5; i++ {
		a := newFakeActivity(fmt.Sprintf("a%d", i), KindFetch)
		a.setSnap(Snapshot{
			Status:     StatusDone,
			StartedAt:  time.Unix(900, 0),
			FinishedAt: time.Unix(int64(1000+i), 0),
		})
		_ = r.Register(a)
	}
	r.Cleanup()
	if got := len(r.Snapshot()); got != 5 {
		t.Fatalf("below cap: want 5, got %d", got)
	}
}

func TestRegistryDropsOldestTerminalsBeyondCap(t *testing.T) {
	r := NewRegistry(NewFakeClock(time.Unix(0, 0)))
	total := maxTerminal + 10
	for i := 0; i < total; i++ {
		a := newFakeActivity(fmt.Sprintf("a%d", i), KindFetch)
		a.setSnap(Snapshot{
			Status:     StatusDone,
			StartedAt:  time.Unix(int64(i), 0),
			FinishedAt: time.Unix(int64(i+1), 0),
		})
		_ = r.Register(a)
	}
	r.Cleanup()
	got := r.Snapshot()
	if len(got) != maxTerminal {
		t.Fatalf("want %d after cap, got %d", maxTerminal, len(got))
	}
	// None of the 10 oldest (a0..a9) should remain.
	for _, v := range got {
		for i := 0; i < 10; i++ {
			if v.Activity.ID() == fmt.Sprintf("a%d", i) {
				t.Fatalf("oldest %q should have been dropped", v.Activity.ID())
			}
		}
	}
}

func TestRegistryDoesNotCountRunningTowardCap(t *testing.T) {
	r := NewRegistry(NewFakeClock(time.Unix(0, 0)))
	for i := 0; i < maxTerminal+50; i++ {
		a := newFakeActivity(fmt.Sprintf("r%d", i), KindFetch)
		_ = r.Register(a)
	}
	r.Cleanup()
	if got := len(r.Snapshot()); got != maxTerminal+50 {
		t.Fatalf("running: want all %d retained, got %d", maxTerminal+50, got)
	}
}

func drain(ch <-chan Event) {
	for {
		select {
		case <-ch:
		default:
			return
		}
	}
}
