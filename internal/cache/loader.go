package cache

import (
	"context"
	"sync/atomic"
	"time"

	tea "charm.land/bubbletea/v2"
)

// CoalesceInterval is the minimum wall-clock time between snapshot
// emissions during a streaming fetch. The first page and the final page
// are always emitted regardless of timing; intermediate pages within
// the interval are merged into the in-flight session and held until the
// next emission window. Picked to give roughly 20fps streaming UX.
const CoalesceInterval = 50 * time.Millisecond

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

// Loader streams paginated results from a backing fetch function,
// merges them by key, coalesces snapshots, and pushes the result back
// to a Bubble Tea Update loop as Page[T] messages.
//
// The merge and coalesce both run on the worker goroutine, so the UI
// thread only ever sees pre-merged snapshots and never blocks on the
// stream's running cost. The previous design did the merge inside
// Update, which made a 100k-item load freeze the UI for minutes.
//
// Each call to Fetch cancels any previous in-flight fetch on the same
// loader. Cancellation is enforced by both context propagation and a
// monotonic generation counter — late messages from a superseded fetch
// are dropped before reaching the UI.
type Loader[T any] struct {
	store  Store[T]
	keyOf  func(T) string
	cancel context.CancelFunc
	gen    atomic.Int64
}

// NewLoader creates a Loader backed by the given Store. keyOf returns
// a stable identity for each item (typically a Name or ID field) and
// is used by the merge to deduplicate streamed pages.
func NewLoader[T any](store Store[T], keyOf func(T) string) *Loader[T] {
	return &Loader[T]{store: store, keyOf: keyOf}
}

// Get returns cached items from the underlying store.
func (l *Loader[T]) Get(key string) ([]T, bool) {
	return l.store.Get(key)
}

// Set writes items directly to the underlying store. Useful for
// hydrating the cache from a non-streaming source.
func (l *Loader[T]) Set(key string, items []T) {
	l.store.Set(key, items)
}

// Fetch starts a progressive background load. Any previous in-flight
// fetch on this Loader is cancelled.
//
// seed is the currently displayed items (used to seed the merge so
// pre-existing entries stay visible during the stream and Finalize can
// sweep server-deleted items at the end). Pass nil for a fresh load.
//
// fetchFn runs in the worker goroutine and should call send() for each
// raw page from the SDK pager. The context is cancelled if a new Fetch
// starts on the same loader, so fetchFn should propagate it to the SDK.
//
// wrapMsg converts each merged Page snapshot into a concrete tea.Msg
// for the app's Update loop. The returned tea.Cmd produces the first
// snapshot message; each subsequent snapshot is delivered via the
// message's Next field, in the standard Bubble Tea streaming pattern.
func (l *Loader[T]) Fetch(
	key string,
	seed []T,
	fetchFn func(ctx context.Context, send func([]T)) error,
	wrapMsg func(Page[T]) tea.Msg,
) tea.Cmd {
	if l.cancel != nil {
		l.cancel()
	}

	gen := l.gen.Add(1)
	ctx, cancel := context.WithCancel(context.Background())
	l.cancel = cancel

	ch := make(chan Page[T], 2)

	go l.worker(ctx, key, seed, fetchFn, ch)

	return l.recv(ch, ctx, gen, wrapMsg)
}

// worker runs the actual fetch on a background goroutine. It maintains
// an internal FetchSession (the merge state), drains pages from the
// fetch callback, and emits coalesced Page snapshots to ch. The final
// snapshot is always emitted (so the UI sees Done) regardless of
// timing.
func (l *Loader[T]) worker(
	ctx context.Context,
	key string,
	seed []T,
	fetchFn func(ctx context.Context, send func([]T)) error,
	ch chan<- Page[T],
) {
	defer close(ch)

	session := NewFetchSession(seed, 0, l.keyOf)
	var lastFlush time.Time
	dirty := false

	emit := func(items []T, done bool, err error) bool {
		select {
		case ch <- Page[T]{Key: key, Items: items, Done: done, Err: err}:
			lastFlush = time.Now()
			dirty = false
			return true
		case <-ctx.Done():
			return false
		}
	}

	send := func(items []T) {
		session.Apply(items)
		dirty = true
		// Time-based coalescing: if enough time has elapsed since the
		// last snapshot (or this is the first emission), flush now.
		// Otherwise hold and let subsequent pages pile in.
		if lastFlush.IsZero() || time.Since(lastFlush) >= CoalesceInterval {
			snapshot := append([]T(nil), session.Items()...)
			l.store.Set(key, snapshot)
			emit(snapshot, false, nil)
		}
	}

	err := fetchFn(ctx, send)
	if ctx.Err() != nil {
		return
	}

	// Final snapshot. On success, sweep unseen items via Finalize so the
	// UI sees the authoritative server state. On error, keep the
	// accumulated state (the user still sees what we managed to load).
	var final []T
	if err == nil {
		final = session.Finalize()
	} else {
		final = append([]T(nil), session.Items()...)
		_ = dirty // suppress unused-when-error
	}
	l.store.Set(key, final)
	emit(final, true, err)
}

// recv builds the tea.Cmd that delivers one Page from the worker's
// channel, then chains itself for the next snapshot via Page.Next.
// Drops messages whose generation no longer matches the loader's
// current generation — that's how superseded fetches get filtered out
// before reaching the UI's Update.
func (l *Loader[T]) recv(
	ch <-chan Page[T],
	ctx context.Context,
	capturedGen int64,
	wrapMsg func(Page[T]) tea.Msg,
) tea.Cmd {
	return func() tea.Msg {
		select {
		case page, ok := <-ch:
			if !ok || ctx.Err() != nil {
				return nil
			}
			if l.gen.Load() != capturedGen {
				return nil
			}
			if !page.Done {
				page.Next = l.recv(ch, ctx, capturedGen, wrapMsg)
			}
			return wrapMsg(page)
		case <-ctx.Done():
			return nil
		}
	}
}
