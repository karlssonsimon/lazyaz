package cache

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	tea "charm.land/bubbletea/v2"
)

// StreamStatus describes the lifecycle of a broker stream.
type StreamStatus int

const (
	StreamActive    StreamStatus = iota // fetch in progress
	StreamDone                          // fetch completed successfully
	StreamCancelled                     // user cancelled (data kept)
	StreamErrored                       // fetch ended with error
)

func (s StreamStatus) String() string {
	switch s {
	case StreamActive:
		return "active"
	case StreamDone:
		return "done"
	case StreamCancelled:
		return "cancelled"
	case StreamErrored:
		return "errored"
	default:
		return "unknown"
	}
}

// StreamInfo is a snapshot of a stream's state, intended for the UI
// layer that lists ongoing/completed fetches.
type StreamInfo struct {
	Key       string
	Status    StreamStatus
	Items     int
	Subs      int
	StartedAt time.Time
	EndedAt   time.Time // zero while active
	Err       error
}

// stream is the internal state for one in-flight (or recently completed)
// keyed fetch. The broker's mutex protects all fields.
type stream[T any] struct {
	ctx       context.Context
	cancel    context.CancelFunc
	status    StreamStatus
	items     []T // latest merged snapshot
	subs      map[int64]chan Page[T]
	startedAt time.Time
	endedAt   time.Time
	err       error
}

// Broker coordinates shared, streaming fetches across multiple
// subscribers. Two tabs requesting the same key share a single API
// call; late joiners receive the accumulated snapshot immediately,
// then live updates as more pages arrive.
//
// Broker is safe for concurrent use. The underlying Store is only
// accessed from the worker goroutine (writes) and under the mutex
// (reads via Get), matching the same safety model as Loader.
type Broker[T any] struct {
	store   Store[T]
	keyOf   func(T) string
	mu      sync.Mutex
	streams map[string]*stream[T]
	nextSub atomic.Int64
}

// NewBroker creates a Broker backed by the given Store. keyOf returns
// a stable identity for each item, used by FetchSession to merge
// streamed pages.
func NewBroker[T any](store Store[T], keyOf func(T) string) *Broker[T] {
	return &Broker[T]{
		store:   store,
		keyOf:   keyOf,
		streams: make(map[string]*stream[T]),
	}
}

// Get returns cached items from the underlying store.
func (b *Broker[T]) Get(key string) ([]T, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.store.Get(key)
}

// Set writes items directly to the underlying store.
func (b *Broker[T]) Set(key string, items []T) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.store.Set(key, items)
}

// Streams returns a snapshot of all tracked streams for UI display.
func (b *Broker[T]) Streams() []StreamInfo {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]StreamInfo, 0, len(b.streams))
	for key, s := range b.streams {
		out = append(out, StreamInfo{
			Key:       key,
			Status:    s.status,
			Items:     len(s.items),
			Subs:      len(s.subs),
			StartedAt: s.startedAt,
			EndedAt:   s.endedAt,
			Err:       s.err,
		})
	}
	return out
}

// Cancel stops an in-flight fetch for the given key. Accumulated data
// is preserved in the store and subscribers receive a final Done page.
// No-op if the key has no active stream.
func (b *Broker[T]) Cancel(key string) {
	b.mu.Lock()
	s, ok := b.streams[key]
	if !ok || s.status != StreamActive {
		b.mu.Unlock()
		return
	}
	s.status = StreamCancelled
	s.endedAt = time.Now()
	b.mu.Unlock()
	s.cancel()
}

// Subscribe joins an existing stream or starts a new one. Returns a
// tea.Cmd that delivers Page[T] messages via wrapMsg, exactly like
// Loader.Fetch.
//
// seed is the currently displayed items. For a new stream it seeds
// the merge session; for an existing stream it is ignored (the catch-up
// snapshot replaces it).
//
// fetchFn is only called when a new stream is started. If a stream for
// key is already active, the subscriber joins it and fetchFn is ignored.
//
// The returned int64 is the subscription ID, needed for Unsubscribe.
func (b *Broker[T]) Subscribe(
	key string,
	seed []T,
	fetchFn func(ctx context.Context, send func([]T)) error,
	wrapMsg func(Page[T]) tea.Msg,
) (tea.Cmd, int64) {
	subID := b.nextSub.Add(1)
	ch := make(chan Page[T], 4)

	b.mu.Lock()
	s, exists := b.streams[key]

	if exists && s.status == StreamActive {
		// Join existing active stream.
		s.subs[subID] = ch
		// Send catch-up snapshot.
		snapshot := append([]T(nil), s.items...)
		b.mu.Unlock()
		go func() {
			ch <- Page[T]{Key: key, Items: snapshot, Done: false}
		}()
		return b.recv(key, subID, ch, wrapMsg), subID
	}

	// Start new stream. Clean up any finished stream for this key.
	ctx, cancel := context.WithCancel(context.Background())
	s = &stream[T]{
		ctx:       ctx,
		cancel:    cancel,
		status:    StreamActive,
		subs:      map[int64]chan Page[T]{subID: ch},
		startedAt: time.Now(),
	}
	b.streams[key] = s
	b.mu.Unlock()

	go b.worker(ctx, key, seed, fetchFn, s)

	return b.recv(key, subID, ch, wrapMsg), subID
}

// Unsubscribe removes a subscriber from a stream. If the stream has no
// remaining subscribers the fetch continues to completion (warming the
// cache). The subscriber's channel is closed.
func (b *Broker[T]) Unsubscribe(key string, subID int64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	s, ok := b.streams[key]
	if !ok {
		return
	}
	if ch, exists := s.subs[subID]; exists {
		delete(s.subs, subID)
		close(ch)
	}
}

// worker runs the fetch on a background goroutine. Very similar to
// Loader.worker but fans out snapshots to all subscribers.
//
// The worker enforces an idle timeout: if no data arrives within
// DefaultIdleTimeout the context is cancelled. As long as pages keep
// flowing the fetch runs indefinitely.
func (b *Broker[T]) worker(
	ctx context.Context,
	key string,
	seed []T,
	fetchFn func(ctx context.Context, send func([]T)) error,
	s *stream[T],
) {
	// Idle timeout: cancel the fetch if no data arrives for too long.
	idleTimer := time.NewTimer(DefaultIdleTimeout)
	defer idleTimer.Stop()
	idleCtx, idleCancel := context.WithCancel(ctx)
	defer idleCancel()
	go func() {
		select {
		case <-idleTimer.C:
			idleCancel()
		case <-idleCtx.Done():
		}
	}()

	session := NewFetchSession(seed, 0, b.keyOf)
	var lastFlush time.Time
	dirty := false

	emit := func(items []T, done bool, err error) {
		b.mu.Lock()
		s.items = items
		if done {
			if err != nil && s.status == StreamActive {
				s.status = StreamErrored
				s.err = err
			} else if s.status == StreamActive {
				s.status = StreamDone
			}
			s.endedAt = time.Now()
		}
		// Copy subscriber map under lock — iterate outside.
		subs := make(map[int64]chan Page[T], len(s.subs))
		for id, ch := range s.subs {
			subs[id] = ch
		}
		b.mu.Unlock()

		page := Page[T]{Key: key, Items: items, Done: done, Err: err}
		for _, ch := range subs {
			select {
			case ch <- page:
			case <-ctx.Done():
				return
			}
		}
		lastFlush = time.Now()
		dirty = false
	}

	send := func(items []T) {
		// Reset idle timer — data is still flowing.
		if !idleTimer.Stop() {
			select {
			case <-idleTimer.C:
			default:
			}
		}
		idleTimer.Reset(DefaultIdleTimeout)

		session.Apply(items)
		dirty = true
		if lastFlush.IsZero() || time.Since(lastFlush) >= CoalesceInterval {
			snapshot := append([]T(nil), session.Items()...)
			b.mu.Lock()
			b.store.Set(key, snapshot)
			b.mu.Unlock()
			emit(snapshot, false, nil)
		}
	}

	err := fetchFn(idleCtx, send)

	// If cancelled via broker.Cancel, ctx is done and status is already
	// set. Emit final snapshot so subscribers see Done.
	if ctx.Err() != nil {
		b.mu.Lock()
		snapshot := append([]T(nil), session.Items()...)
		b.store.Set(key, snapshot)
		b.mu.Unlock()
		emit(snapshot, true, nil)
		return
	}

	var final []T
	if err == nil {
		final = session.Finalize()
	} else {
		final = append([]T(nil), session.Items()...)
		_ = dirty
	}
	b.mu.Lock()
	b.store.Set(key, final)
	b.mu.Unlock()
	emit(final, true, err)
}

// recv builds the chained tea.Cmd that delivers pages from a
// subscriber's channel, exactly matching the Loader.recv pattern.
func (b *Broker[T]) recv(
	key string,
	subID int64,
	ch <-chan Page[T],
	wrapMsg func(Page[T]) tea.Msg,
) tea.Cmd {
	return func() tea.Msg {
		page, ok := <-ch
		if !ok {
			return nil
		}
		if !page.Done {
			page.Next = b.recv(key, subID, ch, wrapMsg)
		}
		return wrapMsg(page)
	}
}

// Reset cancels all active streams, clears all cached data from the
// store, and removes all stream tracking. Used after an az login to
// invalidate everything.
func (b *Broker[T]) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, s := range b.streams {
		if s.status == StreamActive {
			s.status = StreamCancelled
			s.endedAt = time.Now()
			s.cancel()
		}
	}
	b.streams = make(map[string]*stream[T])
	b.store.Clear()
}

// ClearStream removes a completed/cancelled/errored stream from the
// broker's tracking. Active streams are not removed. Used to keep the
// stream list tidy.
func (b *Broker[T]) ClearStream(key string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	s, ok := b.streams[key]
	if !ok {
		return
	}
	if s.status == StreamActive {
		return
	}
	delete(b.streams, key)
}
