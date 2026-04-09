package cache

import (
	"time"

	tea "charm.land/bubbletea/v2"
)

// CoalesceInterval is the minimum wall-clock time between snapshot
// emissions during a streaming fetch. The first page and the final page
// are always emitted regardless of timing; intermediate pages within
// the interval are merged into the in-flight session and held until the
// next emission window. Picked to give roughly 20fps streaming UX.
const CoalesceInterval = 50 * time.Millisecond

// DefaultIdleTimeout is how long a stream can go without receiving
// data before the broker cancels it. As long as pages keep arriving
// the fetch runs indefinitely.
const DefaultIdleTimeout = 60 * time.Second

// Page represents a snapshot of a streaming load. Items is the merged
// state of everything seen so far (deduplicated by key, with per-key
// updates landing on the original position). When Done is true the
// stream is finished and Items is the finalised set (server-deleted
// items have been swept). Next is the command to receive the next
// snapshot; nil when Done.
type Page[T any] struct {
	Key   string
	Items []T
	Done  bool
	Err   error
	Next  tea.Cmd
}
