package app

import (
	"github.com/karlssonsimon/lazyaz/internal/blobapp"
	"github.com/karlssonsimon/lazyaz/internal/keymap"
	"github.com/karlssonsimon/lazyaz/internal/kvapp"
	"github.com/karlssonsimon/lazyaz/internal/sbapp"
	"github.com/karlssonsimon/lazyaz/internal/ui"

	"github.com/charmbracelet/lipgloss"
)

func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return "loading..."
	}

	tabBar := renderTabBar(m.tabs, m.activeIdx, m.styles.TabBar, m.width)

	childView := ""
	if len(m.tabs) > 0 {
		childView = m.tabs[m.activeIdx].Model.View()
	}

	view := lipgloss.JoinVertical(lipgloss.Left, tabBar, childView)

	if m.cmdPalette.active {
		view = renderCommandPalette(&m.cmdPalette, m.styles.Overlay, m.width, m.height, view)
	}
	if m.tabPicker.active {
		view = renderTabPickerOverlay(&m.tabPicker, m.styles.Overlay, m.width, m.height, view)
	}
	if m.themeOverlay.Active {
		view = ui.RenderThemeOverlay(m.themeOverlay, m.schemes, m.styles, m.width, m.height, view)
	}
	if m.helpOverlay.Active {
		view = ui.RenderHelpOverlay(m.helpOverlay, m.styles, m.width, m.height, view)
	}

	return ui.RenderCanvas(view, m.width, m.height, m.styles.Bg)
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
			keymap.HelpEntry(km.ToggleHelp, "help"),
			keymap.HelpEntry(km.Quit, "quit"),
		},
	}
	return append([]ui.HelpSection{tabSection}, childSections...)
}

