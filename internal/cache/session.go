package cache

// FetchSession accumulates streaming fetch results with key-based merge
// semantics. Each session represents one refresh of one list.
//
// Lifecycle:
//
//  1. Seed with the currently cached items via [NewFetchSession].
//  2. Call [FetchSession.Apply] for each page as it arrives. New keys are
//     appended; existing keys are updated in place (preserving position).
//  3. On the final page (done=true, err=nil) call [FetchSession.Finalize]
//     to drop items that were never observed during this session.
//  4. On error, discard the session without calling Finalize so that
//     accumulated state remains visible to the user.
//
// The embedded generation token lets callers reject stale pages from a
// cancelled or superseded fetch — see [FetchSession.Gen].
//
// FetchSession is single-use. Create a fresh one for each refresh.
type FetchSession[T any] struct {
	gen   int
	items []T
	seen  map[string]struct{}
	keyOf func(T) string
}

// NewFetchSession seeds a session with the currently displayed items and
// an application-supplied generation token. current is copied — the
// caller's slice is not retained. keyOf must return a stable identity for
// each item (e.g. the item's Name field).
func NewFetchSession[T any](current []T, gen int, keyOf func(T) string) *FetchSession[T] {
	items := make([]T, len(current))
	copy(items, current)
	return &FetchSession[T]{
		gen:   gen,
		items: items,
		seen:  make(map[string]struct{}, len(current)),
		keyOf: keyOf,
	}
}

// Gen returns the session's generation token. Callers should compare this
// against each incoming page and drop pages whose gen does not match the
// current session — that's how we ignore late arrivals from a fetch that
// was cancelled or superseded by a newer refresh.
func (s *FetchSession[T]) Gen() int { return s.gen }

// Items returns the current merged slice. The returned slice is owned by
// the session and must not be mutated by the caller. It is safe to read
// until the next call to [FetchSession.Apply] or [FetchSession.Finalize].
func (s *FetchSession[T]) Items() []T { return s.items }

// Apply merges a page of items into the session. For each item:
//   - if its key matches an existing entry, that entry is replaced in
//     place (preserving position in the list);
//   - otherwise it is appended to the end.
//
// Every key in the page is recorded as "seen" so that [FetchSession.Finalize]
// can distinguish items that still exist from items that have been
// deleted server-side.
func (s *FetchSession[T]) Apply(page []T) {
	for _, item := range page {
		k := s.keyOf(item)
		s.seen[k] = struct{}{}
		replaced := false
		for i := range s.items {
			if s.keyOf(s.items[i]) == k {
				s.items[i] = item
				replaced = true
				break
			}
		}
		if !replaced {
			s.items = append(s.items, item)
		}
	}
}

// Finalize drops items whose keys were never seen during the session and
// returns the resulting slice. Call this exactly once, when the final
// streaming page arrives successfully (done=true, err=nil). Do not call
// Finalize on an error path — keep the accumulated state so the user
// still sees what the fetch did manage to return.
func (s *FetchSession[T]) Finalize() []T {
	out := make([]T, 0, len(s.items))
	for _, item := range s.items {
		if _, ok := s.seen[s.keyOf(item)]; ok {
			out = append(out, item)
		}
	}
	return out
}
