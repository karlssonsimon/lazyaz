package dashapp

import (
	"fmt"

	"github.com/karlssonsimon/lazyaz/internal/fuzzy"
	"github.com/karlssonsimon/lazyaz/internal/keymap"
	"github.com/karlssonsimon/lazyaz/internal/ui"
)

// sortOption is one (field, direction) combo shown in the sort picker.
// isDefault marks the "no sort" entry that resets to the widget's
// natural ordering. Mirrors sbapp/entity_sort.go's option model so the
// two overlays look and feel the same.
type sortOption struct {
	label     string
	field     int
	desc      bool
	isDefault bool
}

// sortOverlayState is the dashboard's sort picker. Opened by the
// focused widget's "Sort by..." action (or the `s` direct keybind).
// Each (field, direction) combo is a separate selectable entry —
// matches sbapp's entitySortOverlay pattern instead of toggling.
type sortOverlayState struct {
	active    bool
	cursorIdx int
	query     string
	filtered  []int
	options   []sortOption
	widgetIdx int

	// activeField/activeDesc/hasSort snapshot the widget's view state
	// at open time so the IsActive marker shows next to the currently
	// applied option.
	activeField int
	activeDesc  bool
	hasSort     bool
}

// sortResult carries the user's choice back to Update. applied=false
// means the user cancelled. clear=true means "remove sort entirely"
// (the Default option).
type sortResult struct {
	applied bool
	clear   bool
	field   int
	desc    bool
}

// open builds the option list from the widget's SortFields, prepending
// a Default entry. Numeric prefixes ("1  ...", "2  ...") match sbapp's
// labelling. Cursor lands on the currently active combo if there is one.
func (s *sortOverlayState) open(widgetIdx int, fields []SortField, view widgetViewState) {
	options := make([]sortOption, 0, 1+len(fields)*2)
	options = append(options, sortOption{label: "Default", isDefault: true})
	for i, f := range fields {
		options = append(options,
			sortOption{label: f.Label + " ascending", field: i, desc: false},
			sortOption{label: f.Label + " descending", field: i, desc: true},
		)
	}
	for i := range options {
		options[i].label = fmt.Sprintf("%d  %s", i+1, options[i].label)
	}

	*s = sortOverlayState{
		active:      len(options) > 0,
		options:     options,
		widgetIdx:   widgetIdx,
		activeField: view.sortField,
		activeDesc:  view.sortDesc,
		hasSort:     view.hasSort,
	}

	// Land cursor on the currently applied option.
	if !s.hasSort {
		s.cursorIdx = 0 // Default
		return
	}
	for i, opt := range options {
		if !opt.isDefault && opt.field == s.activeField && opt.desc == s.activeDesc {
			s.cursorIdx = i
			return
		}
	}
}

func (s *sortOverlayState) close() {
	*s = sortOverlayState{}
}

func (s *sortOverlayState) refilter() {
	if s.query == "" {
		s.filtered = nil
		return
	}
	s.filtered = fuzzy.Filter(s.query, s.options, func(o sortOption) string { return o.label })
	if s.cursorIdx >= len(s.filtered) {
		s.cursorIdx = 0
		if len(s.filtered) > 0 {
			s.cursorIdx = len(s.filtered) - 1
		}
	}
}

func (s *sortOverlayState) selectedOption() (sortOption, bool) {
	if s.filtered != nil {
		if s.cursorIdx >= len(s.filtered) {
			return sortOption{}, false
		}
		return s.options[s.filtered[s.cursorIdx]], true
	}
	if s.cursorIdx >= len(s.options) {
		return sortOption{}, false
	}
	return s.options[s.cursorIdx], true
}

// handleKey mirrors sbapp/entity_sort.go's handler so sort interactions
// feel identical across both apps. ThemeUp/Down navigate, ThemeApply
// confirms, ThemeCancel clears the search (or closes if empty),
// BackspaceUp / printable chars edit the query, ctrl+v pastes.
func (s *sortOverlayState) handleKey(key string, km keymap.Keymap) sortResult {
	switch {
	case km.ThemeUp.Matches(key):
		if s.cursorIdx > 0 {
			s.cursorIdx--
		}
	case km.ThemeDown.Matches(key):
		n := len(s.options)
		if s.filtered != nil {
			n = len(s.filtered)
		}
		if s.cursorIdx < n-1 {
			s.cursorIdx++
		}
	case km.ThemeApply.Matches(key):
		if opt, ok := s.selectedOption(); ok {
			s.close()
			if opt.isDefault {
				return sortResult{applied: true, clear: true}
			}
			return sortResult{applied: true, field: opt.field, desc: opt.desc}
		}
	case km.ThemeCancel.Matches(key):
		if s.query != "" {
			s.query = ""
			s.filtered = nil
			s.cursorIdx = 0
		} else {
			s.close()
		}
	case km.BackspaceUp.Matches(key):
		if len(s.query) > 0 {
			s.query = s.query[:len(s.query)-1]
			s.refilter()
		}
	case key == "ctrl+v":
		if text := ui.ReadClipboard(); text != "" {
			s.query += text
			s.refilter()
		}
	default:
		if len(key) == 1 && key[0] >= 32 && key[0] < 127 {
			s.query += key
			s.refilter()
		}
	}
	return sortResult{}
}

// renderSortOverlay paints the picker on top of the base view. Matches
// sbapp's entity sort overlay: search query at the top, IsActive marker
// next to the currently applied combo, centered placement.
func (m Model) renderSortOverlay(base string) string {
	s := &m.sortOverlay
	indices := s.filtered
	if indices == nil {
		indices = make([]int, len(s.options))
		for i := range s.options {
			indices[i] = i
		}
	}
	items := make([]ui.OverlayItem, len(indices))
	for ci, oi := range indices {
		opt := s.options[oi]
		isActive := false
		if opt.isDefault {
			isActive = !s.hasSort
		} else if s.hasSort {
			isActive = opt.field == s.activeField && opt.desc == s.activeDesc
		}
		items[ci] = ui.OverlayItem{Label: opt.label, IsActive: isActive}
	}
	cfg := ui.OverlayListConfig{
		Title:      "Sort",
		Query:      s.query,
		CursorView: m.Cursor.View(),
		CloseHint:  m.Keymap.Cancel.Short(),
		MaxVisible: len(s.options),
		Center:     true,
	}
	return ui.RenderOverlayList(cfg, items, s.cursorIdx, m.Styles.Overlay, m.Width, m.Height, base)
}
