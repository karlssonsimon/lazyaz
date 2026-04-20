package activity

import (
	"reflect"
	"sort"
	"sync"
	"time"
)

// tickInterval is how often the ticker polls activities for snapshot
// changes. 250ms is a good balance between UI liveness and wakeups.
const tickInterval = 250 * time.Millisecond

// maxTerminal is the cap on how many terminal-status activities the
// registry retains. Older ones are dropped (oldest by FinishedAt).
// Running / WaitingInput activities are never dropped by this cap.
const maxTerminal = 1000

// Registry owns the set of currently-tracked activities. One instance
// lives on appshell.Model and is shared across every tab in a session.
//
// Safe for concurrent use. Callers never mutate returned snapshots.
type Registry struct {
	clock Clock

	mu         sync.Mutex
	activities map[string]Activity
	lastSnap   map[string]Snapshot
	subs       map[int64]chan Event
	nextSubID  int64

	tickerStop chan struct{}
	tickerMu   sync.Mutex // guards tickerStop
}

// NewRegistry builds a Registry. clock is used for cleanup deadlines.
// Pass RealClock{} in production.
func NewRegistry(clock Clock) *Registry {
	return &Registry{
		clock:      clock,
		activities: make(map[string]Activity),
		lastSnap:   make(map[string]Snapshot),
		subs:       make(map[int64]chan Event),
	}
}

// Register adds a to the registry. The returned unregister func removes
// a immediately, regardless of status (bypasses the 60s cleanup window).
// Calling unregister twice is safe; later calls are no-ops.
func (r *Registry) Register(a Activity) (unregister func()) {
	r.mu.Lock()
	r.activities[a.ID()] = a
	r.lastSnap[a.ID()] = a.Snapshot()
	r.mu.Unlock()

	r.notify()
	r.maybeStartTicker()

	var once sync.Once
	return func() {
		once.Do(func() {
			r.mu.Lock()
			delete(r.activities, a.ID())
			delete(r.lastSnap, a.ID())
			r.mu.Unlock()
			r.notify()
		})
	}
}

// Snapshot returns a point-in-time view of every tracked activity with
// its most recent Snapshot. Safe to call from any goroutine.
func (r *Registry) Snapshot() []ActivityView {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]ActivityView, 0, len(r.activities))
	for _, a := range r.activities {
		out = append(out, ActivityView{Activity: a, Snapshot: a.Snapshot()})
	}
	return out
}

// Events returns a buffered per-caller event channel and a cancel func.
// Callers must invoke cancel() when done so the registry can remove the
// subscriber (otherwise the ticker keeps running with a phantom sub).
//
// Events fire at most once per tick (coalesced).
func (r *Registry) Events() (ch <-chan Event, cancel func()) {
	r.mu.Lock()
	id := r.nextSubID
	r.nextSubID++
	c := make(chan Event, 1)
	r.subs[id] = c
	r.mu.Unlock()

	r.maybeStartTicker()

	var once sync.Once
	return c, func() {
		once.Do(func() {
			r.mu.Lock()
			delete(r.subs, id)
			close(c)
			r.mu.Unlock()
			r.maybeStopTicker()
		})
	}
}

// Cleanup caps the number of retained terminal-status activities at
// maxTerminal. When the cap is exceeded, the oldest (by FinishedAt)
// are dropped. Running / WaitingInput activities are never touched.
// Called automatically on each tick; exported for tests.
func (r *Registry) Cleanup() {
	r.mu.Lock()
	type entry struct {
		id         string
		finishedAt time.Time
	}
	var terminals []entry
	for id, a := range r.activities {
		snap := a.Snapshot()
		if !snap.Status.Terminal() {
			continue
		}
		fa := snap.FinishedAt
		if fa.IsZero() {
			// No FinishedAt stamped — treat the activity as the
			// oldest possible so it's the first to go if we hit the cap.
			fa = time.Time{}
		}
		terminals = append(terminals, entry{id, fa})
	}
	if len(terminals) <= maxTerminal {
		r.mu.Unlock()
		return
	}
	sort.Slice(terminals, func(i, j int) bool {
		return terminals[i].finishedAt.Before(terminals[j].finishedAt)
	})
	toRemove := len(terminals) - maxTerminal
	for i := 0; i < toRemove; i++ {
		delete(r.activities, terminals[i].id)
		delete(r.lastSnap, terminals[i].id)
	}
	r.mu.Unlock()
	r.notify()
}

func (r *Registry) notify() {
	r.mu.Lock()
	subs := make([]chan Event, 0, len(r.subs))
	for _, c := range r.subs {
		subs = append(subs, c)
	}
	r.mu.Unlock()
	for _, c := range subs {
		select {
		case c <- Event{}:
		default:
		}
	}
}

func (r *Registry) tickOnce() {
	r.Cleanup()
	r.mu.Lock()
	changed := false
	for id, a := range r.activities {
		cur := a.Snapshot()
		if !reflect.DeepEqual(r.lastSnap[id], cur) {
			r.lastSnap[id] = cur
			changed = true
		}
	}
	r.mu.Unlock()
	if changed {
		r.notify()
	}
}

func (r *Registry) maybeStartTicker() {
	r.tickerMu.Lock()
	defer r.tickerMu.Unlock()
	if r.tickerStop != nil {
		return
	}
	r.mu.Lock()
	haveSubs := len(r.subs) > 0
	r.mu.Unlock()
	if !haveSubs {
		return
	}

	stop := make(chan struct{})
	r.tickerStop = stop
	go func() {
		t := time.NewTicker(tickInterval)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				r.tickOnce()
			case <-stop:
				return
			}
		}
	}()
}

func (r *Registry) maybeStopTicker() {
	r.tickerMu.Lock()
	defer r.tickerMu.Unlock()
	if r.tickerStop == nil {
		return
	}
	r.mu.Lock()
	haveSubs := len(r.subs) > 0
	r.mu.Unlock()
	if haveSubs {
		return
	}
	close(r.tickerStop)
	r.tickerStop = nil
}
