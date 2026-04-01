package cache

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"
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
	if l.cancel != nil {
		l.cancel()
	}

	ctx, cancel := context.WithCancel(context.Background())
	l.cancel = cancel

	ch := make(chan Page[T], 1)
	go func() {
		defer close(ch)
		var all []T
		send := func(items []T) {
			all = append(all, items...)
			out := make([]T, len(all))
			copy(out, all)
			select {
			case ch <- Page[T]{Key: key, Items: out}:
			case <-ctx.Done():
			}
		}
		err := fetchFn(ctx, send)
		if ctx.Err() != nil {
			return
		}
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
