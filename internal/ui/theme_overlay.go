package ui

import (
	"strings"
)

type KeyMatcher interface {
	Matches(key string) bool
}

type ThemeKeyBindings struct {
	Up, Down, Apply, Cancel KeyMatcher
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
	if s.Query == "" {
		s.filtered = make([]int, len(schemes))
		for i := range schemes {
			s.filtered[i] = i
		}
	} else {
		q := strings.ToLower(s.Query)
		s.filtered = s.filtered[:0]
		for i, sc := range schemes {
			if strings.Contains(strings.ToLower(sc.Name), q) {
				s.filtered = append(s.filtered, i)
			}
		}
	}
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
		s.Active = false
	case key == "backspace":
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

func RenderThemeOverlay(state ThemeOverlayState, schemes []Scheme, styles Styles, width, height int, base string) string {
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

	return RenderOverlayList(OverlayListConfig{Title: "Themes", Query: state.Query}, items, state.CursorIdx, styles.Overlay, width, height, base)
}
