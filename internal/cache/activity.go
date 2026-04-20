package cache

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/karlssonsimon/lazyaz/internal/activity"
)

// BrokerActivityAdapter wraps a single broker stream (identified by key)
// as an activity.Activity. The adapter holds a reference to the broker
// so Snapshot() can re-query stream state and Cancel() can halt the fetch.
type BrokerActivityAdapter[T any] struct {
	broker *Broker[T]
	key    string
	id     string

	// Cached terminal info so Snapshot can still report a sensible
	// FinishedAt if the broker cleaned its internal stream entry
	// (e.g. user re-subscribed to the same key) before the registry's
	// 60s cleanup window elapsed.
	mu             sync.Mutex
	lastStatus     StreamStatus
	lastFinishedAt time.Time
}

// NewBrokerActivityAdapter builds an adapter for the given stream info.
// The info passed is used for the initial identity; subsequent Snapshot
// calls re-query the broker.
func NewBrokerActivityAdapter[T any](b *Broker[T], info StreamInfo) *BrokerActivityAdapter[T] {
	return &BrokerActivityAdapter[T]{
		broker: b,
		key:    info.Key,
		id:     "fetch:" + info.Key,
	}
}

func (a *BrokerActivityAdapter[T]) ID() string          { return a.id }
func (a *BrokerActivityAdapter[T]) Kind() activity.Kind { return activity.KindFetch }

func (a *BrokerActivityAdapter[T]) Title() string {
	parts := strings.Split(a.key, "\x00")
	return strings.Join(parts, " > ")
}

func (a *BrokerActivityAdapter[T]) Snapshot() activity.Snapshot {
	for _, s := range a.broker.Streams() {
		if s.Key != a.key {
			continue
		}
		if s.Status != StreamActive && !s.EndedAt.IsZero() {
			a.mu.Lock()
			a.lastStatus = s.Status
			a.lastFinishedAt = s.EndedAt
			a.mu.Unlock()
		}
		return activity.Snapshot{
			Status:     mapBrokerStatus(s.Status),
			StartedAt:  s.StartedAt,
			FinishedAt: s.EndedAt,
			Items:      s.Items,
			Err:        s.Err,
			Detail:     fmt.Sprintf("%d subs", s.Subs),
		}
	}
	// Stream vanished from broker (cleared, or replaced by a new subscribe).
	// Report the last-seen terminal state so the registry can age this
	// activity out of its 60s cleanup window. If we never saw a terminal
	// snapshot, stamp Now so cleanup can still fire.
	a.mu.Lock()
	status := a.lastStatus
	fa := a.lastFinishedAt
	a.mu.Unlock()
	if fa.IsZero() {
		fa = time.Now()
	}
	if status == StreamActive || status == 0 {
		status = StreamCancelled
	}
	return activity.Snapshot{
		Status:     mapBrokerStatus(status),
		FinishedAt: fa,
	}
}

func (a *BrokerActivityAdapter[T]) Cancel() { a.broker.Cancel(a.key) }

func mapBrokerStatus(s StreamStatus) activity.Status {
	switch s {
	case StreamActive:
		return activity.StatusRunning
	case StreamDone:
		return activity.StatusDone
	case StreamCancelled:
		return activity.StatusCancelled
	case StreamErrored:
		return activity.StatusErrored
	default:
		return activity.StatusRunning
	}
}
