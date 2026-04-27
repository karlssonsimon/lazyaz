package sbapp

import (
	"slices"

	"github.com/karlssonsimon/lazyaz/internal/azure/servicebus"
	"github.com/karlssonsimon/lazyaz/internal/fuzzy"
	"github.com/karlssonsimon/lazyaz/internal/keymap"
	"github.com/karlssonsimon/lazyaz/internal/ui"
)

type entitySortField int

const (
	entitySortNone   entitySortField = iota
	entitySortName                   // alphabetical
	entitySortActive                 // by ActiveMsgCount
	entitySortDLQ                    // by DeadLetterCount
)

type entitySortOption struct {
	label string
	field entitySortField
	desc  bool
}

var entitySortOptions = []entitySortOption{
	{"1  Default", entitySortNone, false},
	{"2  Name ascending", entitySortName, false},
	{"3  Name descending", entitySortName, true},
	{"4  Active messages ascending", entitySortActive, false},
	{"5  Active messages descending", entitySortActive, true},
	{"6  Dead letters ascending", entitySortDLQ, false},
	{"7  Dead letters descending", entitySortDLQ, true},
}

type entitySortOverlayState struct {
	active    bool
	cursorIdx int
	query     string
	filtered  []int
}

type entitySortResult struct {
	applied bool
	field   entitySortField
	desc    bool
}

func (s *entitySortOverlayState) open(currentField entitySortField, currentDesc bool) {
	s.active = true
	s.query = ""
	s.filtered = nil
	s.cursorIdx = 0
	for i, opt := range entitySortOptions {
		if opt.field == currentField && opt.desc == currentDesc {
			s.cursorIdx = i
			break
		}
	}
}

func (s *entitySortOverlayState) refilter() {
	if s.query == "" {
		s.filtered = nil
		return
	}
	s.filtered = fuzzy.Filter(s.query, entitySortOptions, func(o entitySortOption) string { return o.label })
	if s.cursorIdx >= len(s.filtered) {
		s.cursorIdx = max(0, len(s.filtered)-1)
	}
}

func (s *entitySortOverlayState) selectedOption() (entitySortOption, bool) {
	if s.filtered != nil {
		if s.cursorIdx >= len(s.filtered) {
			return entitySortOption{}, false
		}
		return entitySortOptions[s.filtered[s.cursorIdx]], true
	}
	if s.cursorIdx >= len(entitySortOptions) {
		return entitySortOption{}, false
	}
	return entitySortOptions[s.cursorIdx], true
}

func (s *entitySortOverlayState) handleKey(key string, km keymap.Keymap) entitySortResult {
	switch {
	case km.ThemeUp.Matches(key):
		if s.cursorIdx > 0 {
			s.cursorIdx--
		}
	case km.ThemeDown.Matches(key):
		n := len(entitySortOptions)
		if s.filtered != nil {
			n = len(s.filtered)
		}
		if s.cursorIdx < n-1 {
			s.cursorIdx++
		}
	case km.ThemeApply.Matches(key):
		if opt, ok := s.selectedOption(); ok {
			s.active = false
			return entitySortResult{applied: true, field: opt.field, desc: opt.desc}
		}
	case km.ThemeCancel.Matches(key):
		if s.query != "" {
			s.query = ""
			s.filtered = nil
			s.cursorIdx = 0
		} else {
			s.active = false
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
	return entitySortResult{}
}

// sortAndFilterEntities applies the current sort field, direction, and DLQ
// filter to the entity list. Returns a new slice.
func sortAndFilterEntities(entities []servicebus.Entity, field entitySortField, desc bool, dlqFilter bool) []servicebus.Entity {
	out := make([]servicebus.Entity, 0, len(entities))

	if dlqFilter {
		for _, e := range entities {
			if e.DeadLetterCount > 0 {
				out = append(out, e)
			}
		}
	} else {
		out = append(out, entities...)
	}

	if field == entitySortNone {
		return out
	}

	slices.SortStableFunc(out, func(a, b servicebus.Entity) int {
		var cmp int
		switch field {
		case entitySortName:
			switch {
			case a.Name < b.Name:
				cmp = -1
			case a.Name > b.Name:
				cmp = 1
			}
		case entitySortActive:
			switch {
			case a.ActiveMsgCount < b.ActiveMsgCount:
				cmp = -1
			case a.ActiveMsgCount > b.ActiveMsgCount:
				cmp = 1
			}
		case entitySortDLQ:
			switch {
			case a.DeadLetterCount < b.DeadLetterCount:
				cmp = -1
			case a.DeadLetterCount > b.DeadLetterCount:
				cmp = 1
			}
		}
		if desc {
			cmp = -cmp
		}
		return cmp
	})

	return out
}

func (m Model) renderEntitySortOverlay(base string) string {
	s := &m.entitySortOverlay
	indices := s.filtered
	if indices == nil {
		indices = make([]int, len(entitySortOptions))
		for i := range entitySortOptions {
			indices[i] = i
		}
	}
	items := make([]ui.OverlayItem, len(indices))
	for ci, si := range indices {
		opt := entitySortOptions[si]
		items[ci] = ui.OverlayItem{
			Label:    opt.label,
			IsActive: opt.field == m.entitySortField && opt.desc == m.entitySortDesc,
		}
	}
	cfg := ui.OverlayListConfig{
		Title:      "Sort Entities",
		Query:      s.query,
		CursorView: m.Cursor.View(),
		CloseHint:  m.Keymap.Cancel.Short(),
		Bindings: &ui.OverlayBindings{

			MoveUp:   m.Keymap.ThemeUp,

			MoveDown: m.Keymap.ThemeDown,

			Apply:    m.Keymap.ThemeApply,

			Cancel:   m.Keymap.ThemeCancel,

			Erase:    m.Keymap.BackspaceUp,

		},
		MaxVisible: len(entitySortOptions),
		Center:     true,
	}
	return ui.RenderOverlayList(cfg, items, s.cursorIdx, m.Styles, m.Width, m.Height, base)
}

func entitySortLabel(field entitySortField, desc bool, dlqFilter bool) string {
	if dlqFilter {
		return "DLQ only"
	}
	for _, opt := range entitySortOptions {
		if opt.field == field && opt.desc == desc {
			return opt.label[3:] // strip "N  " prefix
		}
	}
	return ""
}
