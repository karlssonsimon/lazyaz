package dashapp

import (
	"github.com/karlssonsimon/lazyaz/internal/keymap"
	"github.com/karlssonsimon/lazyaz/internal/ui"

	tea "charm.land/bubbletea/v2"
)

// actionMenuState is the dashboard's lightweight action menu. It opens
// over the focused widget on `a`, lists the actions for the cursor row,
// and fires the selected one on enter (or on the action's own keybind).
type actionMenuState struct {
	active  bool
	cursor  int
	actions []Action
}

func (s *actionMenuState) open(actions []Action) {
	*s = actionMenuState{
		active:  len(actions) > 0,
		actions: actions,
	}
}

func (s *actionMenuState) close() {
	*s = actionMenuState{}
}

// handleKey processes a key while the menu is open. Returns the action
// to fire (with selected=true) when the user confirms, or selected=false
// for navigation / cancellation. Direct action keybinds are also honored
// here so the menu doesn't trap them.
func (s *actionMenuState) handleKey(key string, km keymap.Keymap) (Action, bool) {
	// Direct keybind from inside the menu — fire and close.
	for _, a := range s.actions {
		if a.Key == key {
			s.close()
			return a, true
		}
	}
	switch {
	case km.WidgetScrollUp.Matches(key):
		if s.cursor > 0 {
			s.cursor--
		}
	case km.WidgetScrollDown.Matches(key):
		if s.cursor < len(s.actions)-1 {
			s.cursor++
		}
	case km.OpenFocused.Matches(key) || km.OpenFocusedAlt.Matches(key):
		if s.cursor >= 0 && s.cursor < len(s.actions) {
			a := s.actions[s.cursor]
			s.close()
			return a, true
		}
	case km.Cancel.Matches(key):
		s.close()
	}
	return Action{}, false
}

// renderActionMenu paints the menu overlay on top of the base view.
// Uses the shared overlay-list renderer for visual consistency with the
// theme/sub/help pickers.
func (m Model) renderActionMenu(base string) string {
	items := make([]ui.OverlayItem, len(m.actionMenu.actions))
	for i, a := range m.actionMenu.actions {
		items[i] = ui.OverlayItem{Label: a.Label, Hint: a.Key}
	}
	cfg := ui.OverlayListConfig{
		Title:      "Widget actions",
		CloseHint:  m.Keymap.Cancel.Short(),
		HideSearch: true,
	}
	return ui.RenderOverlayList(cfg, items, m.actionMenu.cursor, m.Styles.Overlay, m.Width, m.Height, base)
}

// fireAction is what gets called when the menu yields an action — just
// produces the action's command if any. Lifted out so handleKey can
// invoke it from both the menu confirm and direct-key paths.
func fireAction(a Action) tea.Cmd {
	return a.Cmd
}
