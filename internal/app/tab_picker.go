package app

import (
	"github.com/karlssonsimon/lazyaz/internal/fuzzy"
	"github.com/karlssonsimon/lazyaz/internal/ui"
)

var tabKinds = []struct {
	kind TabKind
	name string
}{
	{TabBlob, "Blob Storage"},
	{TabServiceBus, "Service Bus"},
	{TabKeyVault, "Key Vault"},
}

type tabPickerState struct {
	active    bool
	cursorIdx int
	query     string
	filtered  []int // indices into tabKinds
}

func (s *tabPickerState) open() {
	s.active = true
	s.query = ""
	s.cursorIdx = 0
	s.refilter()
}

func (s *tabPickerState) refilter() {
	s.filtered = fuzzy.Filter(s.query, tabKinds, func(tk struct {
		kind TabKind
		name string
	}) string {
		return tk.name
	})
	if s.cursorIdx >= len(s.filtered) {
		s.cursorIdx = max(0, len(s.filtered)-1)
	}
}

// handleKey processes a key event. Returns the selected TabKind and true if
// the user confirmed a selection.
func (s *tabPickerState) handleKey(key string, bindings ui.ThemeKeyBindings) (TabKind, bool) {
	switch {
	case bindings.Up.Matches(key):
		if s.cursorIdx > 0 {
			s.cursorIdx--
		}
	case bindings.Down.Matches(key):
		if s.cursorIdx < len(s.filtered)-1 {
			s.cursorIdx++
		}
	case bindings.Apply.Matches(key):
		if len(s.filtered) > 0 && s.cursorIdx < len(s.filtered) {
			kind := tabKinds[s.filtered[s.cursorIdx]].kind
			s.active = false
			return kind, true
		}
	case bindings.Cancel.Matches(key):
		s.active = false
	case bindings.Erase != nil && bindings.Erase.Matches(key):
		if len(s.query) > 0 {
			s.query = s.query[:len(s.query)-1]
			s.refilter()
		}
	default:
		if len(key) == 1 && key[0] >= 32 && key[0] < 127 {
			s.query += key
			s.refilter()
		}
	}
	return 0, false
}

func renderTabPickerOverlay(s *tabPickerState, closeHint, cursorView string, styles ui.OverlayStyles, width, height int, base string) string {
	items := make([]ui.OverlayItem, len(s.filtered))
	for ci, ti := range s.filtered {
		items[ci] = ui.OverlayItem{Label: tabKinds[ti].name}
	}

	cfg := ui.OverlayListConfig{
		Title:      "New Tab",
		Query:      s.query,
		CursorView: cursorView,
		CloseHint:  closeHint,
		MaxVisible: len(tabKinds),
	}
	return ui.RenderOverlayList(cfg, items, s.cursorIdx, styles, width, height, base)
}
