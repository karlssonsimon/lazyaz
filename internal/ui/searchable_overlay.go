package ui

import "github.com/karlssonsimon/lazyaz/internal/fuzzy"

// SearchableOverlay holds common state for searchable overlay lists.
type SearchableOverlay[T any] struct {
	Active    bool
	CursorIdx int
	Query     string

	items    []T
	filtered []int
	key      func(T) string
}

func (s *SearchableOverlay[T]) Open(items []T, key func(T) string) {
	s.Active = true
	s.CursorIdx = 0
	s.Query = ""
	s.items = items
	s.filtered = nil
	s.key = key
}

func (s *SearchableOverlay[T]) Close() {
	*s = SearchableOverlay[T]{}
}

func (s *SearchableOverlay[T]) TypeText(text string) {
	if text == "" {
		return
	}
	s.Query += text
	s.refilter()
}

func (s *SearchableOverlay[T]) Backspace() {
	if len(s.Query) == 0 {
		return
	}
	s.Query = s.Query[:len(s.Query)-1]
	s.refilter()
}

func (s *SearchableOverlay[T]) Cancel() bool {
	if s.Query != "" {
		s.Query = ""
		s.filtered = nil
		s.CursorIdx = 0
		return false
	}
	s.Close()
	return true
}

func (s *SearchableOverlay[T]) Move(delta int) {
	n := len(s.Visible())
	if n == 0 {
		return
	}
	s.CursorIdx += delta
	if s.CursorIdx < 0 {
		s.CursorIdx = 0
	}
	if s.CursorIdx >= n {
		s.CursorIdx = n - 1
	}
}

func (s *SearchableOverlay[T]) Visible() []T {
	if s.filtered == nil {
		return s.items
	}
	visible := make([]T, len(s.filtered))
	for i, idx := range s.filtered {
		visible[i] = s.items[idx]
	}
	return visible
}

func (s *SearchableOverlay[T]) Selected() (T, bool) {
	visible := s.Visible()
	if s.CursorIdx < len(visible) {
		return visible[s.CursorIdx], true
	}
	var zero T
	return zero, false
}

func (s *SearchableOverlay[T]) refilter() {
	if s.Query == "" {
		s.filtered = nil
		s.CursorIdx = 0
		return
	}
	s.filtered = fuzzy.Filter(s.Query, s.items, s.key)
	if s.CursorIdx >= len(s.filtered) {
		s.CursorIdx = max(0, len(s.filtered)-1)
	}
}
