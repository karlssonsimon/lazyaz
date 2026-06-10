package ui

import (
	"strings"

	"charm.land/bubbles/v2/cursor"
	"github.com/karlssonsimon/lazyaz/internal/fuzzy"
	"github.com/karlssonsimon/lazyaz/internal/keymap"
)

type HelpKeyBindings struct {
	Up, Down, Close, Cancel, Erase KeyMatcher
}

type HelpOverlayState struct {
	Active     bool
	Title      string
	CursorIdx  int
	Query      string
	QueryCaret int // rune index of the edit caret within Query
	items      []helpItem
	visible    []OverlayItem // rebuilt on refilter; what gets rendered
	maxKeyW    int           // stable column width across queries
}

type helpItem struct {
	keys    string
	desc    string
	section string
}

type HelpSection struct {
	Title string
	Items []string
}

func (s *HelpOverlayState) Open(title string, sections []HelpSection) {
	s.Active = true
	s.Title = title
	s.Query = ""
	s.QueryCaret = 0
	s.CursorIdx = 0
	s.items = flattenSections(sections)
	s.maxKeyW = 0
	for _, it := range s.items {
		if w := len(it.keys); w > s.maxKeyW {
			s.maxKeyW = w
		}
	}
	s.refilter()
}

func (s *HelpOverlayState) Close() {
	s.Active = false
}

func (s *HelpOverlayState) HandleKey(key string, bindings HelpKeyBindings) {
	if s.visible == nil {
		s.refilter()
	}

	switch {
	case bindings.Close.Matches(key), bindings.Cancel != nil && bindings.Cancel.Matches(key):
		if s.Query != "" {
			s.Query = ""
			s.QueryCaret = 0
			s.refilter()
		} else {
			s.Active = false
		}
		return
	case bindings.Up.Matches(key):
		s.moveCursor(-1)
		return
	case bindings.Down.Matches(key):
		s.moveCursor(1)
		return
	case key == "ctrl+v":
		if text := ReadClipboard(); text != "" {
			s.PasteText(text)
		}
		return
	}
	// Editing + cursor movement keys route to the query buffer.
	ti := TextInput{Value: s.Query, Cursor: s.QueryCaret}
	if ti.HandleKey(key) {
		changed := ti.Value != s.Query
		s.Query = ti.Value
		s.QueryCaret = ti.Cursor
		if changed {
			s.refilter()
		}
	}
}

// PasteText inserts text at the cursor and refilters.
func (s *HelpOverlayState) PasteText(text string) {
	if text == "" {
		return
	}
	ti := TextInput{Value: s.Query, Cursor: s.QueryCaret}
	ti.Insert(text)
	s.Query = ti.Value
	s.QueryCaret = ti.Cursor
	s.refilter()
}

// moveCursor advances by delta, skipping over header rows so the user
// never lands on a non-selectable section title.
func (s *HelpOverlayState) moveCursor(delta int) {
	if len(s.visible) == 0 {
		return
	}
	next := s.CursorIdx + delta
	for next >= 0 && next < len(s.visible) {
		if !s.visible[next].IsHeader {
			s.CursorIdx = next
			return
		}
		next += delta
	}
}

func (s *HelpOverlayState) refilter() {
	matched := fuzzy.Filter(s.Query, s.items, func(item helpItem) string {
		return item.keys + " " + item.desc + " " + item.section
	})

	s.visible = s.visible[:0]
	// Always group by section. The header sits above its items — when
	// filtering, sections with no matches simply don't appear, so the
	// surviving headers double as the "this match is in section X" cue.
	var lastSection string
	for _, idx := range matched {
		it := s.items[idx]
		if it.section != lastSection {
			s.visible = append(s.visible, OverlayItem{
				Label:    strings.ToUpper(it.section),
				IsHeader: true,
			})
			lastSection = it.section
		}
		s.visible = append(s.visible, OverlayItem{
			Label: padRight(it.keys, s.maxKeyW) + "  " + it.desc,
		})
	}

	// Snap cursor to a real item (skip headers, clamp to bounds).
	if s.CursorIdx >= len(s.visible) {
		s.CursorIdx = max(0, len(s.visible)-1)
	}
	if s.CursorIdx < len(s.visible) && s.visible[s.CursorIdx].IsHeader {
		s.moveCursor(1)
		if s.visible[s.CursorIdx].IsHeader {
			s.moveCursor(-1)
		}
	}
}

func flattenSections(sections []HelpSection) []helpItem {
	var items []helpItem
	for _, section := range sections {
		for _, entry := range section.Items {
			keys, desc := parseHelpEntry(entry)
			items = append(items, helpItem{
				keys:    keys,
				desc:    desc,
				section: section.Title,
			})
		}
	}
	return items
}

func parseHelpEntry(s string) (keys, desc string) {
	if idx := strings.Index(s, "  "); idx >= 0 {
		return s[:idx], s[idx+2:]
	}
	return s, ""
}

func RenderHelpOverlay(state HelpOverlayState, closeHint string, cur cursor.Model, styles Styles, km *keymap.Keymap, width, height int, base string) string {
	if state.visible == nil {
		state.refilter()
	}

	// Compute desc width from ALL items so the overlay stays stable
	// during search.
	maxDescW := 0
	for _, item := range state.items {
		if w := len(item.desc); w > maxDescW {
			maxDescW = w
		}
	}

	// Size to fit content: key + gap(2) + desc + padding(4).
	innerW := state.maxKeyW + 2 + maxDescW + 4
	if innerW < 60 {
		innerW = 60
	}
	if innerW > width-10 {
		innerW = width - 10
	}

	// Height accounts for the worst-case visible list (items + their
	// section headers) capped at 30 so the box fits comfortably.
	maxVis := len(state.items) + 8
	if maxVis < 10 {
		maxVis = 10
	}
	if maxVis > 30 {
		maxVis = 30
	}

	var bindings *OverlayBindings
	if km != nil {
		bindings = &OverlayBindings{
			MoveUp:   km.ThemeUp,
			MoveDown: km.ThemeDown,
			Cancel:   km.ToggleHelp,
			Erase:    km.BackspaceUp,
		}
	}
	cfg := OverlayListConfig{
		Title:          state.Title,
		Query:          state.Query,
		QueryCursor:    state.QueryCaret,
		Cursor:         cur,
		CloseHint:      closeHint,
		InnerWidth:     innerW,
		MaxVisible:     maxVis,
		Center:         true,
		Bindings:       bindings,
		NoActiveMarker: true,
	}

	return RenderOverlayList(cfg, state.visible, state.CursorIdx, styles, width, height, base)
}
