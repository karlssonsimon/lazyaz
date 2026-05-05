package app

import (
	tea "charm.land/bubbletea/v2"

	"github.com/karlssonsimon/lazyaz/internal/fuzzy"
	"github.com/karlssonsimon/lazyaz/internal/keymap"
	"github.com/karlssonsimon/lazyaz/internal/ui"
)

// tabPickerEntry is one selectable item in the new-tab overlay. Most
// entries map cleanly to a TabKind (kind != 0 is implicit; TabBlob is 0
// but always paired with a non-nil action). Entries that need extra UX
// (a connection-string prompt, an Azurite preset) carry an action that
// emits the right tea.Msg instead of going straight through addTab.
type tabPickerEntry struct {
	name   string
	action func() tea.Msg
}

var tabPickerEntries = []tabPickerEntry{
	{name: "Blob Storage", action: func() tea.Msg { return tabPickerMsg{kind: TabBlob} }},
	{name: "Service Bus", action: func() tea.Msg { return tabPickerMsg{kind: TabServiceBus} }},
	{name: "Key Vault", action: func() tea.Msg { return tabPickerMsg{kind: TabKeyVault} }},
	{name: "Dashboard", action: func() tea.Msg { return tabPickerMsg{kind: TabDashboard} }},
	{name: "Azurite (local emulator)", action: func() tea.Msg { return openAzuriteTabMsg{} }},
	{name: "Blob (connection string)", action: func() tea.Msg { return openConnStringPromptMsg{} }},
}

type tabPickerState struct {
	active    bool
	cursorIdx int
	query     string
	filtered  []int // indices into tabPickerEntries
}

func (s *tabPickerState) open() {
	s.active = true
	s.query = ""
	s.cursorIdx = 0
	s.refilter()
}

func (s *tabPickerState) refilter() {
	s.filtered = fuzzy.Filter(s.query, tabPickerEntries, func(e tabPickerEntry) string {
		return e.name
	})
	if s.cursorIdx >= len(s.filtered) {
		s.cursorIdx = max(0, len(s.filtered)-1)
	}
}

// handleKey processes a key event. Returns the chosen entry's action
// (a function producing a tea.Msg) and true if the user confirmed a
// selection. The caller dispatches that message through Update.
func (s *tabPickerState) handleKey(key string, bindings ui.ThemeKeyBindings) (func() tea.Msg, bool) {
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
			entry := tabPickerEntries[s.filtered[s.cursorIdx]]
			s.active = false
			return entry.action, true
		}
	case bindings.Cancel.Matches(key):
		if s.query != "" {
			s.query = ""
			s.refilter()
		} else {
			s.active = false
		}
	case bindings.Erase != nil && bindings.Erase.Matches(key):
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
	return nil, false
}

func renderTabPickerOverlay(s *tabPickerState, closeHint, cursorView string, styles ui.Styles, km *keymap.Keymap, width, height int, base string) string {
	items := make([]ui.OverlayItem, len(s.filtered))
	for ci, ti := range s.filtered {
		items[ci] = ui.OverlayItem{Label: tabPickerEntries[ti].name}
	}

	cfg := ui.OverlayListConfig{
		Title:      "New Tab",
		Query:      s.query,
		CursorView: cursorView,
		CloseHint:  closeHint,
		MaxVisible: len(tabPickerEntries),
		Bindings: &ui.OverlayBindings{
			MoveUp:   km.ThemeUp,
			MoveDown: km.ThemeDown,
			Apply:    km.ThemeApply,
			Cancel:   km.Cancel,
			Erase:    km.BackspaceUp,
		},
	}
	return ui.RenderOverlayList(cfg, items, s.cursorIdx, styles, width, height, base)
}
