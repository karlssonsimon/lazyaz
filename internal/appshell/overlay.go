package appshell

import (
	"azure-storage/internal/azure"
	"azure-storage/internal/ui"
)

// OverlayResult describes how HandleOverlayKeys consumed a key press.
//
// When Handled is true the caller MUST return early (no further key
// dispatch) and may need to take one of the follow-up actions encoded
// in the other fields:
//
//   - SelectSub != nil: the user picked a subscription in the sub overlay.
//     The app should call its own selectSubscription(*SelectSub) so that
//     resource-specific navigation/fetch can kick off.
//   - ThemeSelected: the user applied a theme. The app should call its
//     own ApplyScheme(schemes[ThemeOverlay.ActiveThemeIdx]) so that
//     list delegates are repainted, then ui.SaveThemeName.
//
// Both follow-ups are expressed as fields (rather than callbacks) because
// each app's selectSubscription returns a tea.Cmd and mutates resource
// state, which can't be expressed generically in Model.
type OverlayResult struct {
	Handled       bool
	SelectSub     *azure.Subscription
	ThemeSelected bool
}

// HandleOverlayKeys dispatches a key press to any active overlay (subscription
// picker, help, theme, inspect). Returns an OverlayResult describing what
// happened. Callers must check Handled first and return early.
func (m *Model) HandleOverlayKeys(key string) OverlayResult {
	if m.SubOverlay.Active {
		if sub, ok := m.SubOverlay.HandleKey(key, ui.ThemeKeyBindings{
			Up:     m.Keymap.ThemeUp,
			Down:   m.Keymap.ThemeDown,
			Apply:  m.Keymap.ThemeApply,
			Cancel: m.Keymap.ThemeCancel,
		}, m.Subscriptions); ok {
			return OverlayResult{Handled: true, SelectSub: &sub}
		}
		return OverlayResult{Handled: true}
	}

	if !m.EmbeddedMode && m.HelpOverlay.Active {
		m.HelpOverlay.HandleKey(key, ui.HelpKeyBindings{
			Up:    m.Keymap.ThemeUp,
			Down:  m.Keymap.ThemeDown,
			Close: m.Keymap.ToggleHelp,
		})
		return OverlayResult{Handled: true}
	}

	if !m.EmbeddedMode && m.ThemeOverlay.Active {
		if m.ThemeOverlay.HandleKey(key, ui.ThemeKeyBindings{
			Up:     m.Keymap.ThemeUp,
			Down:   m.Keymap.ThemeDown,
			Apply:  m.Keymap.ThemeApply,
			Cancel: m.Keymap.ThemeCancel,
		}, m.Schemes) {
			return OverlayResult{Handled: true, ThemeSelected: true}
		}
		return OverlayResult{Handled: true}
	}

	// Inspect overlay — dismiss on inspect/esc/q.
	if m.InspectFields != nil {
		if m.Keymap.Inspect.Matches(key) || key == "esc" || key == "q" {
			m.InspectFields = nil
		}
		return OverlayResult{Handled: true}
	}

	return OverlayResult{}
}

// RenderOverlays paints the four standard overlays on top of the given base
// view, in the correct stacking order (inspect → subscription → theme → help).
// Apps should call this at the very end of their View() method.
func (m Model) RenderOverlays(view string) string {
	if m.InspectFields != nil {
		view = ui.RenderInspectOverlay(m.InspectTitle, m.InspectFields, m.Styles, m.Width, m.Height, view)
	}
	if m.SubOverlay.Active {
		view = ui.RenderSubscriptionOverlay(m.SubOverlay, m.Subscriptions, m.CurrentSub, m.Loading, m.LoadingStartedAt, m.Styles, m.Width, m.Height, view)
	}
	if !m.EmbeddedMode && m.ThemeOverlay.Active {
		view = ui.RenderThemeOverlay(m.ThemeOverlay, m.Schemes, m.Styles, m.Width, m.Height, view)
	}
	if !m.EmbeddedMode && m.HelpOverlay.Active {
		view = ui.RenderHelpOverlay(m.HelpOverlay, m.Styles, m.Width, m.Height, view)
	}
	return view
}
