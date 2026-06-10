package dashapp

import (
	"fmt"

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
	ui.SearchableOverlay[sortOption]
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
		widgetIdx:   widgetIdx,
		activeField: view.sortField,
		activeDesc:  view.sortDesc,
		hasSort:     view.hasSort,
	}
	s.Open(options, func(o sortOption) string { return o.label })

	// Land cursor on the currently applied option.
	if !s.hasSort {
		s.CursorIdx = 0 // Default
		return
	}
	for i, opt := range options {
		if !opt.isDefault && opt.field == s.activeField && opt.desc == s.activeDesc {
			s.CursorIdx = i
			return
		}
	}
}

func (s *sortOverlayState) close() {
	*s = sortOverlayState{}
}

// handleKey mirrors sbapp/entity_sort.go's handler so sort interactions
// feel identical across both apps. ThemeUp/Down navigate, ThemeApply
// confirms, ThemeCancel clears the search (or closes if empty),
// BackspaceUp / printable chars edit the query, ctrl+v pastes.
func (s *sortOverlayState) handleKey(key string, km keymap.Keymap) sortResult {
	switch {
	case km.ThemeUp.Matches(key):
		s.Move(-1)
		return sortResult{}
	case km.ThemeDown.Matches(key):
		s.Move(1)
		return sortResult{}
	case km.ThemeApply.Matches(key):
		if opt, ok := s.Selected(); ok {
			s.close()
			if opt.isDefault {
				return sortResult{applied: true, clear: true}
			}
			return sortResult{applied: true, field: opt.field, desc: opt.desc}
		}
		return sortResult{}
	case km.ThemeCancel.Matches(key):
		if !s.Cancel() {
			return sortResult{}
		}
		s.close()
		return sortResult{}
	case key == "ctrl+v":
		if text := ui.ReadClipboard(); text != "" {
			s.TypeText(text)
		}
		return sortResult{}
	}
	s.HandleQueryKey(key)
	return sortResult{}
}

// renderSortOverlay paints the picker on top of the base view. Matches
// sbapp's entity sort overlay: search query at the top, IsActive marker
// next to the currently applied combo, centered placement.
func (m Model) renderSortOverlay(base string) string {
	s := &m.sortOverlay
	visible := s.Visible()
	items := make([]ui.OverlayItem, len(visible))
	for i, opt := range visible {
		isActive := false
		if opt.isDefault {
			isActive = !s.hasSort
		} else if s.hasSort {
			isActive = opt.field == s.activeField && opt.desc == s.activeDesc
		}
		items[i] = ui.OverlayItem{Label: opt.label, IsActive: isActive}
	}
	cfg := ui.OverlayListConfig{
		Title:       "Sort",
		Query:       s.Query,
		QueryCursor: s.QueryCaret,
		Cursor:      m.Cursor,
		CloseHint:   m.Keymap.Cancel.Short(),
		Bindings: &ui.OverlayBindings{
			MoveUp:   m.Keymap.ThemeUp,
			MoveDown: m.Keymap.ThemeDown,
			Apply:    m.Keymap.ThemeApply,
			Cancel:   m.Keymap.ThemeCancel,
			Erase:    m.Keymap.BackspaceUp,
		},
		MaxVisible: len(visible),
		Center:     true,
	}
	return ui.RenderOverlayList(cfg, items, s.CursorIdx, m.Styles, m.Width, m.Height, base)
}
