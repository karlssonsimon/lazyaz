package ui

import (
	"github.com/karlssonsimon/lazyaz/internal/fuzzy"
)

type KeyMatcher interface {
	Matches(key string) bool
}

type ThemeKeyBindings struct {
	Up, Down, Apply, Cancel, Erase KeyMatcher
}

type ThemeOverlayState struct {
	Active         bool
	ActiveThemeIdx int
	CursorIdx      int
	Query          string
	filtered       []int
}

func (s *ThemeOverlayState) Open() {
	s.Active = true
	s.Query = ""
	s.filtered = nil
	s.CursorIdx = s.ActiveThemeIdx
}

func (s *ThemeOverlayState) refilter(schemes []Scheme) {
	s.filtered = fuzzy.Filter(s.Query, schemes, func(sc Scheme) string { return sc.Name })
	if s.CursorIdx >= len(s.filtered) {
		s.CursorIdx = max(0, len(s.filtered)-1)
	}
}

func (s *ThemeOverlayState) selectedThemeIdx() (int, bool) {
	if len(s.filtered) == 0 || s.CursorIdx >= len(s.filtered) {
		return 0, false
	}
	return s.filtered[s.CursorIdx], true
}

func (s *ThemeOverlayState) HandleKey(key string, bindings ThemeKeyBindings, schemes []Scheme) (applied bool) {
	if len(schemes) == 0 {
		s.Active = false
		return false
	}

	if s.filtered == nil {
		s.refilter(schemes)
	}

	switch {
	case bindings.Up.Matches(key):
		if s.CursorIdx > 0 {
			s.CursorIdx--
		}
	case bindings.Down.Matches(key):
		if s.CursorIdx < len(s.filtered)-1 {
			s.CursorIdx++
		}
	case bindings.Apply.Matches(key):
		if idx, ok := s.selectedThemeIdx(); ok {
			s.ActiveThemeIdx = idx
			s.Active = false
			return true
		}
	case bindings.Cancel.Matches(key):
		if s.Query != "" {
			s.Query = ""
			s.refilter(schemes)
		} else {
			s.Active = false
		}
	case bindings.Erase != nil && bindings.Erase.Matches(key):
		if len(s.Query) > 0 {
			s.Query = s.Query[:len(s.Query)-1]
			s.refilter(schemes)
		}
	default:
		if len(key) == 1 && key[0] >= 32 && key[0] < 127 {
			s.Query += key
			s.refilter(schemes)
		}
	}
	return false
}

func RenderThemeOverlay(state ThemeOverlayState, closeHint, cursorView string, schemes []Scheme, styles Styles, width, height int, base string) string {
	filtered := state.filtered
	if filtered == nil {
		filtered = make([]int, len(schemes))
		for i := range schemes {
			filtered[i] = i
		}
	}

	items := make([]OverlayItem, len(filtered))
	for ci, ti := range filtered {
		items[ci] = OverlayItem{
			Label:    schemes[ti].Name,
			IsActive: ti == state.ActiveThemeIdx,
		}
	}

	return RenderOverlayList(OverlayListConfig{Title: "Themes", Query: state.Query, CursorView: cursorView, CloseHint: closeHint}, items, state.CursorIdx, styles.Overlay, width, height, base)
}
