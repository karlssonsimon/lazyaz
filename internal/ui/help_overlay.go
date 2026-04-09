package ui

import (
	"strings"

	"github.com/karlssonsimon/lazyaz/internal/fuzzy"
)

type HelpKeyBindings struct {
	Up, Down, Close, Cancel, Erase KeyMatcher
}

type HelpOverlayState struct {
	Active    bool
	Title     string
	CursorIdx int
	Query     string
	items     []helpItem
	filtered  []int
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
	s.CursorIdx = 0
	s.items = flattenSections(sections)
	s.filtered = nil
}

func (s *HelpOverlayState) Close() {
	s.Active = false
}

func (s *HelpOverlayState) HandleKey(key string, bindings HelpKeyBindings) {
	if s.filtered == nil {
		s.refilter()
	}

	switch {
	case bindings.Close.Matches(key), bindings.Cancel != nil && bindings.Cancel.Matches(key):
		if s.Query != "" {
			s.Query = ""
			s.refilter()
		} else {
			s.Active = false
		}
	case bindings.Up.Matches(key):
		if s.CursorIdx > 0 {
			s.CursorIdx--
		}
	case bindings.Down.Matches(key):
		if s.CursorIdx < len(s.filtered)-1 {
			s.CursorIdx++
		}
	case bindings.Erase != nil && bindings.Erase.Matches(key):
		if len(s.Query) > 0 {
			s.Query = s.Query[:len(s.Query)-1]
			s.refilter()
		}
	case key == "ctrl+v":
		if text := ReadClipboard(); text != "" {
			s.Query += text
			s.refilter()
		}
	default:
		if len(key) == 1 && key[0] >= 32 && key[0] < 127 {
			s.Query += key
			s.refilter()
		}
	}
}

// PasteText appends pasted text to the query and refilters.
func (s *HelpOverlayState) PasteText(text string) {
	s.Query += text
	s.refilter()
}

func (s *HelpOverlayState) refilter() {
	s.filtered = fuzzy.Filter(s.Query, s.items, func(item helpItem) string {
		return item.keys + " " + item.desc + " " + item.section
	})
	if s.CursorIdx >= len(s.filtered) {
		s.CursorIdx = max(0, len(s.filtered)-1)
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

func RenderHelpOverlay(state HelpOverlayState, closeHint, cursorView string, styles Styles, width, height int, base string) string {
	filtered := state.filtered
	if filtered == nil {
		filtered = make([]int, len(state.items))
		for i := range state.items {
			filtered[i] = i
		}
	}

	// Compute column widths from ALL items so the overlay stays stable during search.
	maxKeyW := 0
	maxDescW := 0
	maxSectW := 0
	for _, item := range state.items {
		if w := len(item.keys); w > maxKeyW {
			maxKeyW = w
		}
		if w := len(item.desc); w > maxDescW {
			maxDescW = w
		}
		if w := len(item.section); w > maxSectW {
			maxSectW = w
		}
	}

	items := make([]OverlayItem, len(filtered))
	for ci, idx := range filtered {
		item := state.items[idx]
		label := padRight(item.keys, maxKeyW) + "  " + item.desc
		items[ci] = OverlayItem{
			Label: label,
			Hint:  item.section,
		}
	}

	// Size to fit content: marker(2) + key + gap(2) + desc + gap(2) + section + padding(4).
	innerW := 2 + maxKeyW + 2 + maxDescW + 2 + maxSectW + 4
	if innerW < 60 {
		innerW = 60
	}
	if innerW > width-10 {
		innerW = width - 10
	}

	// Height based on total item count, not terminal size.
	maxVis := len(state.items) + 2
	if maxVis < 10 {
		maxVis = 10
	}
	if maxVis > 30 {
		maxVis = 30
	}

	cfg := OverlayListConfig{
		Title:      state.Title,
		Query:      state.Query,
		CursorView: cursorView,
		CloseHint:  closeHint,
		InnerWidth: innerW,
		MaxVisible: maxVis,
		Center:     true,
	}

	return RenderOverlayList(cfg, items, state.CursorIdx, styles.Overlay, width, height, base)
}
