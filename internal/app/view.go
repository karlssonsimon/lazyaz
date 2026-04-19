package app

import (
	"time"

	"github.com/karlssonsimon/lazyaz/internal/appshell"
	"github.com/karlssonsimon/lazyaz/internal/blobapp"
	"github.com/karlssonsimon/lazyaz/internal/dashapp"
	"github.com/karlssonsimon/lazyaz/internal/keymap"
	"github.com/karlssonsimon/lazyaz/internal/kvapp"
	"github.com/karlssonsimon/lazyaz/internal/sbapp"
	"github.com/karlssonsimon/lazyaz/internal/ui"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func (m Model) View() tea.View {
	if m.width == 0 || m.height == 0 {
		v := tea.NewView("loading...")
		v.AltScreen = true
		return v
	}

	tabBar := renderTabBar(m.tabs, m.activeIdx, m.styles.TabBar, m.width)

	childView := ""
	if len(m.tabs) > 0 {
		childView = m.tabs[m.activeIdx].Model.View().Content
	}

	view := lipgloss.JoinVertical(lipgloss.Left, tabBar, childView)

	// Toasts paint before any modal overlays so the picker/help still
	// covers them when open — modals are deliberate user actions and
	// should win over passive notifications.
	if active := m.notifier.Active(time.Now()); len(active) > 0 {
		view = ui.RenderToasts(notifierToToasts(active), m.styles, m.width, m.height, view)
	}

	closeHint := m.keymap.Cancel.Short()
	cursorView := m.cursor.View()
	if m.cmdPalette.active {
		view = renderCommandPalette(&m.cmdPalette, closeHint, cursorView, m.styles.Overlay, m.width, m.height, view)
	}
	if m.tabPicker.active {
		view = renderTabPickerOverlay(&m.tabPicker, closeHint, cursorView, m.styles.Overlay, m.width, m.height, view)
	}
	if m.tenantPicker.active {
		title := "Switch Tenant"
		if m.tenantPicker.loading {
			title += " ..."
		}
		view = ui.RenderOverlayList(ui.OverlayListConfig{
			Title:      title,
			Query:      m.tenantPicker.query,
			CursorView: cursorView,
			CloseHint:  closeHint,
			MaxVisible: 12,
			Center:     true,
		}, m.tenantPicker.visibleItems(), m.tenantPicker.cursor, m.styles.Overlay, m.width, m.height, view)
	}
	if m.themeOverlay.Active {
		view = ui.RenderThemeOverlay(m.themeOverlay, closeHint, cursorView, m.schemes, m.styles, m.width, m.height, view)
	}
	if m.helpOverlay.Active {
		view = ui.RenderHelpOverlay(m.helpOverlay, closeHint, cursorView, m.styles, m.width, m.height, view)
	}
	if m.notificationsOverlay.Active {
		view = ui.RenderNotificationsOverlay(m.notificationsOverlay, closeHint, notifierToEntries(m.notifier.Snapshot()), m.styles, m.width, m.height, view)
	}
	if m.streamOverlay.Active {
		view = ui.RenderStreamOverlay(m.streamOverlay, closeHint, m.collectStreams(), m.styles, m.width, m.height, view)
	}

	out := tea.NewView(ui.RenderCanvas(view, m.width, m.height, m.styles.Bg))
	out.AltScreen = true
	out.MouseMode = tea.MouseModeCellMotion
	return out
}

// notifierToToasts converts the notifier's domain types to the
// renderer's leaf types so the ui package stays free of an appshell
// import (which would cycle).
func notifierToToasts(ns []appshell.Notification) []ui.Toast {
	out := make([]ui.Toast, len(ns))
	for i, n := range ns {
		out[i] = ui.Toast{Level: notifierLevelToToast(n.Level), Message: n.Message, Spinner: n.Spinner, Time: n.Time}
	}
	return out
}

func notifierToEntries(ns []appshell.Notification) []ui.NotificationEntry {
	out := make([]ui.NotificationEntry, len(ns))
	for i, n := range ns {
		out[i] = ui.NotificationEntry{
			Time:    n.Time,
			Level:   notifierLevelToToast(n.Level),
			Message: n.Message,
		}
	}
	return out
}

func notifierLevelToToast(l appshell.NotificationLevel) ui.ToastLevel {
	switch l {
	case appshell.LevelError:
		return ui.ToastError
	case appshell.LevelWarn:
		return ui.ToastWarn
	case appshell.LevelSuccess:
		return ui.ToastSuccess
	default:
		return ui.ToastInfo
	}
}

func (m Model) activeHelpSections() []ui.HelpSection {
	var childSections []ui.HelpSection
	if len(m.tabs) > 0 {
		switch child := m.tabs[m.activeIdx].Model.(type) {
		case blobapp.Model:
			childSections = child.HelpSections()
		case sbapp.Model:
			childSections = child.HelpSections()
		case kvapp.Model:
			childSections = child.HelpSections()
		case dashapp.Model:
			childSections = child.HelpSections()
		}
	}
	km := m.keymap
	tabSection := ui.HelpSection{
		Title: "Tabs",
		Items: []string{
			keymap.HelpEntry(km.CommandPalette, "command palette"),
			keymap.HelpEntry(km.NewTab, "new tab"),
			keymap.HelpEntry(km.CloseTab, "close tab"),
			keymap.HelpEntry(km.PrevTab, "prev tab"),
			keymap.HelpEntry(km.NextTab, "next tab"),
			"alt+1..9  jump to tab",
			keymap.HelpEntry(km.ToggleThemePicker, "theme picker"),
			keymap.HelpEntry(km.ToggleStreams, "stream manager"),
			keymap.HelpEntry(km.ToggleHelp, "help"),
			keymap.HelpEntry(km.Quit, "quit"),
		},
	}
	return append([]ui.HelpSection{tabSection}, childSections...)
}

