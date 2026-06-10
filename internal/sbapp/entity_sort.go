package sbapp

import (
	"slices"

	"github.com/karlssonsimon/lazyaz/internal/azure/servicebus"
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
	ui.SearchableOverlay[entitySortOption]
}

type entitySortResult struct {
	applied bool
	field   entitySortField
	desc    bool
}

func (s *entitySortOverlayState) open(currentField entitySortField, currentDesc bool) {
	s.Open(entitySortOptions, func(o entitySortOption) string { return o.label })
	for i, opt := range entitySortOptions {
		if opt.field == currentField && opt.desc == currentDesc {
			s.CursorIdx = i
			break
		}
	}
}

func (s *entitySortOverlayState) handleKey(key string, km keymap.Keymap) entitySortResult {
	switch {
	case km.ThemeUp.Matches(key):
		s.Move(-1)
		return entitySortResult{}
	case km.ThemeDown.Matches(key):
		s.Move(1)
		return entitySortResult{}
	case km.ThemeApply.Matches(key):
		if opt, ok := s.Selected(); ok {
			s.Close()
			return entitySortResult{applied: true, field: opt.field, desc: opt.desc}
		}
		return entitySortResult{}
	case km.ThemeCancel.Matches(key):
		s.Cancel()
		return entitySortResult{}
	case key == "ctrl+v":
		if text := ui.ReadClipboard(); text != "" {
			s.TypeText(text)
		}
		return entitySortResult{}
	}
	s.HandleQueryKey(key)
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
	visible := s.Visible()
	items := make([]ui.OverlayItem, len(visible))
	for i, opt := range visible {
		items[i] = ui.OverlayItem{
			Label:    opt.label,
			IsActive: opt.field == m.entitySortField && opt.desc == m.entitySortDesc,
		}
	}
	cfg := ui.OverlayListConfig{
		Title:       "Sort Entities",
		Query:       s.Query,
		QueryCursor: s.QueryCaret,
		Cursor:      m.Cursor,
		CloseHint:   m.Keymap.Cancel.Short(),
		Bindings: &ui.OverlayBindings{

			MoveUp: m.Keymap.ThemeUp,

			MoveDown: m.Keymap.ThemeDown,

			Apply: m.Keymap.ThemeApply,

			Cancel: m.Keymap.ThemeCancel,

			Erase: m.Keymap.BackspaceUp,
		},
		MaxVisible: len(entitySortOptions),
		Center:     true,
	}
	return ui.RenderOverlayList(cfg, items, s.CursorIdx, m.Styles, m.Width, m.Height, base)
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
