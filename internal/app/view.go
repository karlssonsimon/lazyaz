package app

import (
	"azure-storage/internal/blobapp"
	"azure-storage/internal/kvapp"
	"azure-storage/internal/sbapp"
	"azure-storage/internal/ui"

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
	if m.tabPicker {
		view = renderTabPicker(m.styles.Overlay, m.width, m.height, view)
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
	return m.keymap.helpSections(childSections)
}

func renderTabPicker(overlay ui.OverlayStyles, width, height int, base string) string {
	rows := []string{
		overlay.Title.Render("Open New Tab"),
		"",
		overlay.Normal.Render("1) Blob Storage"),
		overlay.Normal.Render("2) Service Bus"),
		overlay.Normal.Render("3) Key Vault"),
		"",
		overlay.Hint.Render("Press 1/2/3 or esc to cancel"),
	}

	content := lipgloss.JoinVertical(lipgloss.Left, rows...)
	box := overlay.Box.Render(content)

	return ui.PlaceOverlay(width, height, box, base)
}
