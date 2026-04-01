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

	tabBar := renderTabBar(m.tabs, m.activeIdx, m.palette, m.width)

	childView := ""
	if len(m.tabs) > 0 {
		childView = m.tabs[m.activeIdx].Model.View()
	}

	view := lipgloss.JoinVertical(lipgloss.Left, tabBar, childView)

	if m.cmdPalette.active {
		view = renderCommandPalette(&m.cmdPalette, m.palette, m.width, m.height, view)
	}
	if m.tabPicker {
		view = renderTabPicker(m.palette, m.width, m.height, view)
	}
	if m.themeOverlay.Active {
		view = ui.RenderThemeOverlay(m.themeOverlay, m.themes, m.palette, m.width, m.height, view)
	}
	if m.helpOverlay.Active {
		sections := m.activeHelpSections()
		view = ui.RenderHelpOverlay("Azure TUI Help", sections, m.palette, m.width, m.height, view)
	}

	return view
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

func renderTabPicker(palette ui.Palette, width, height int, base string) string {
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(palette.Accent)).
		Padding(0, 1)

	itemStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(palette.Text)).
		Padding(0, 1)

	hintStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(palette.Muted)).
		Padding(0, 1)

	rows := []string{
		titleStyle.Render("Open New Tab"),
		"",
		itemStyle.Render("1) Blob Storage"),
		itemStyle.Render("2) Service Bus"),
		itemStyle.Render("3) Key Vault"),
		"",
		hintStyle.Render("Press 1/2/3 or esc to cancel"),
	}

	content := lipgloss.JoinVertical(lipgloss.Left, rows...)

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(palette.BorderFocused)).
		Padding(1, 2).
		Render(content)

	return ui.PlaceOverlay(width, height, box, base)
}
