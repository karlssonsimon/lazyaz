package cache

import (
	"context"

	tea "charm.land/bubbletea/v2"
)

// Page represents a batch of results from a progressive load.
// Items contains all items accumulated so far (not just the current page).
// When Done is true, the load is complete and Items holds the final result.
// Next is the command to receive the next page; nil when Done.
type Page[T any] struct {
	Key   string
	Items []T
	Done  bool
	Err   error
	Next  tea.Cmd
}

// Loader wraps a Store with progressive, channel-based loading.
// It manages background fetches that deliver results page by page,
// cancelling any in-flight fetch when a new one starts.
type Loader[T any] struct {
	store  Store[T]
	cancel context.CancelFunc
}

// NewLoader creates a Loader backed by the given Store.
func NewLoader[T any](store Store[T]) *Loader[T] {
	return &Loader[T]{store: store}
}

// Get returns cached items from the underlying store.
func (l *Loader[T]) Get(key string) ([]T, bool) {
	return l.store.Get(key)
}

// Set writes items directly to the underlying store.
func (l *Loader[T]) Set(key string, items []T) {
	l.store.Set(key, items)
}

// Fetch starts a progressive background load. Any previous in-flight
// fetch on this Loader is cancelled.
//
// fetchFn runs in a goroutine and should call send() for each page of
// results from the Azure SDK pager. The context passed to fetchFn is
// cancelled if a new Fetch starts or the load is abandoned.
//
// wrapMsg converts each Page into a concrete tea.Msg for the app's
// Update loop. The returned tea.Cmd produces the first page message.
func (l *Loader[T]) Fetch(
	key string,
	fetchFn func(ctx context.Context, send func([]T)) error,
	wrapMsg func(Page[T]) tea.Msg,
) tea.Cmd {
	return l.fetch(key, false, fetchFn, wrapMsg)
}

// FetchFresh skips the cached emission and goes straight to the network,
// forcing the user to see the fetch happening. Used for explicit refresh
// actions where "instant from cache" would be confusing.
func (l *Loader[T]) FetchFresh(
	key string,
	fetchFn func(ctx context.Context, send func([]T)) error,
	wrapMsg func(Page[T]) tea.Msg,
) tea.Cmd {
	return l.fetch(key, true, fetchFn, wrapMsg)
}

func (l *Loader[T]) fetch(
	key string,
	fresh bool,
	fetchFn func(ctx context.Context, send func([]T)) error,
	wrapMsg func(Page[T]) tea.Msg,
) tea.Cmd {
	if l.cancel != nil {
		l.cancel()
	}

	ctx, cancel := context.WithCancel(context.Background())
	l.cancel = cancel

	// Note: no "cached emit" here. Callers are expected to hydrate their
	// own views from cache (via Get) before starting a fetch, so the
	// loader only streams authoritative network pages. This keeps
	// FetchSession merge semantics clean — every Page we deliver
	// contributes to the "seen" sweep set. The fresh parameter is
	// retained for API compatibility; with no cached emit to skip, it
	// no longer has an observable effect.

	ch := make(chan Page[T], 2)

	go func() {
		defer close(ch)
		var all []T
		send := func(items []T) {
			all = append(all, items...)
			out := make([]T, len(all))
			copy(out, all)
			l.store.Set(key, out)
			select {
			case ch <- Page[T]{Key: key, Items: out}:
			case <-ctx.Done():
			}
		}
		err := fetchFn(ctx, send)
		if ctx.Err() != nil {
			return
		}
		l.store.Set(key, all)
		done := Page[T]{Key: key, Items: all, Done: true, Err: err}
		select {
		case ch <- done:
		case <-ctx.Done():
		}
	}()

	return recvCmd(ch, wrapMsg, ctx)
}

func recvCmd[T any](ch <-chan Page[T], wrapMsg func(Page[T]) tea.Msg, ctx context.Context) tea.Cmd {
	return func() tea.Msg {
		select {
		case page, ok := <-ch:
			if !ok || ctx.Err() != nil {
				return nil
			}
			if !page.Done {
				page.Next = recvCmd(ch, wrapMsg, ctx)
			}
			return wrapMsg(page)
		case <-ctx.Done():
			return nil
		}
	}
}
