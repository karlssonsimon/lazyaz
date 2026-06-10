package ui

import (
	"charm.land/bubbles/v2/cursor"
	"github.com/karlssonsimon/lazyaz/internal/fuzzy"
	"github.com/karlssonsimon/lazyaz/internal/keymap"
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
	QueryCaret     int // rune index of the edit caret within Query
	filtered       []int
}

func (s *ThemeOverlayState) Open() {
	s.Active = true
	s.Query = ""
	s.QueryCaret = 0
	s.filtered = nil
	s.CursorIdx = s.ActiveThemeIdx
}

// PasteText inserts text at the cursor and refilters.
func (s *ThemeOverlayState) PasteText(text string, schemes []Scheme) {
	if text == "" {
		return
	}
	ti := TextInput{Value: s.Query, Cursor: s.QueryCaret}
	ti.Insert(text)
	s.Query = ti.Value
	s.QueryCaret = ti.Cursor
	s.refilter(schemes)
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
		return false
	case bindings.Down.Matches(key):
		if s.CursorIdx < len(s.filtered)-1 {
			s.CursorIdx++
		}
		return false
	case bindings.Apply.Matches(key):
		if idx, ok := s.selectedThemeIdx(); ok {
			s.ActiveThemeIdx = idx
			s.Active = false
			return true
		}
		return false
	case bindings.Cancel.Matches(key):
		if s.Query != "" {
			s.Query = ""
			s.QueryCaret = 0
			s.refilter(schemes)
		} else {
			s.Active = false
		}
		return false
	case key == "ctrl+v":
		if text := ReadClipboard(); text != "" {
			s.PasteText(text, schemes)
		}
		return false
	}
	ti := TextInput{Value: s.Query, Cursor: s.QueryCaret}
	if ti.HandleKey(key) {
		changed := ti.Value != s.Query
		s.Query = ti.Value
		s.QueryCaret = ti.Cursor
		if changed {
			s.refilter(schemes)
		}
	}
	return false
}

func RenderThemeOverlay(state ThemeOverlayState, closeHint string, cur cursor.Model, schemes []Scheme, styles Styles, km *keymap.Keymap, width, height int, base string) string {
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

	cfg := OverlayListConfig{
		Title:       "Themes",
		Query:       state.Query,
		QueryCursor: state.QueryCaret,
		Cursor:      cur,
		CloseHint:   closeHint,
		Bindings: &OverlayBindings{
			MoveUp:   km.ThemeUp,
			MoveDown: km.ThemeDown,
			Apply:    km.ThemeApply,
			Cancel:   km.Cancel,
			Erase:    km.BackspaceUp,
		},
	}
	return RenderOverlayList(cfg, items, state.CursorIdx, styles, width, height, base)
}
