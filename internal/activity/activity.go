package activity

import "time"

// Kind identifies a broad category of activity. Adapters set this once at
// creation time; it never changes.
type Kind int

const (
	KindUpload   Kind = iota // file uploads to blob storage
	KindFetch                // cache broker list fetches
	KindDownload             // reserved for future
)

// Status is the lifecycle state. Running means "in flight"; WaitingInput
// means "needs user to answer a prompt before it can continue" (e.g. an
// upload paused on a conflict). The other three are terminal.
type Status int

const (
	StatusRunning Status = iota
	StatusWaitingInput
	StatusDone
	StatusCancelled
	StatusErrored
)

// Activity is implemented by each producer (upload, fetch). The registry
// calls Snapshot() on its ticker and passes the results to observers.
// Implementations must be safe to call from any goroutine.
type Activity interface {
	ID() string
	Kind() Kind
	Title() string
	Snapshot() Snapshot
	Cancel()
}

// Snapshot is the immutable point-in-time view of an activity. The
// registry never mutates one after producing it.
type Snapshot struct {
	Status      Status
	StartedAt   time.Time
	FinishedAt  time.Time // zero while running
	TotalBytes  int64     // 0 if not meaningful (fetches)
	DoneBytes   int64
	Items       int       // for fetches; uploads leave 0
	Skipped     int       // for uploads with conflict-skips; fetches leave 0
	BytesPerSec float64
	Detail      string
	Err         error
}

// ActivityView pairs an Activity with its most recent Snapshot. The
// registry produces these so observers don't need to call Snapshot
// themselves.
type ActivityView struct {
	Activity Activity
	Snapshot Snapshot
}

// Event is delivered on the registry's event channel when anything
// changes (register, unregister, snapshot diff). It carries no payload —
// observers re-read Snapshot() to get current state.
type Event struct{}

// Terminal reports whether s is in a state that will not transition
// further (Done, Cancelled, Errored).
func (s Status) Terminal() bool {
	return s == StatusDone || s == StatusCancelled || s == StatusErrored
}
