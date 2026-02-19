package ui

import (
	"github.com/charmbracelet/lipgloss"
)

type ChromeStyles struct {
	Header      lipgloss.Style
	Meta        lipgloss.Style
	Pane        lipgloss.Style
	FocusedPane lipgloss.Style
	Status      lipgloss.Style
	Help        lipgloss.Style
	Error       lipgloss.Style
	FilterHint  lipgloss.Style
}

func NewChromeStyles(p Palette) ChromeStyles {
	pane := lipgloss.NewStyle().
		Foreground(lipgloss.Color(p.Text)).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(p.Border)).
		Padding(0, 1)

	return ChromeStyles{
		Header: lipgloss.NewStyle().
			Foreground(lipgloss.Color(p.Accent)).
			Bold(true).
			Padding(0, 1),
		Meta: lipgloss.NewStyle().
			Foreground(lipgloss.Color(p.Muted)).
			Padding(0, 1),
		Pane: pane,
		FocusedPane: pane.Copy().
			BorderForeground(lipgloss.Color(p.BorderFocused)),
		Status: lipgloss.NewStyle().
			Foreground(lipgloss.Color(p.Text)).
			Padding(0, 1),
		Help: lipgloss.NewStyle().
			Foreground(lipgloss.Color(p.Muted)).
			Padding(0, 1),
		Error: lipgloss.NewStyle().
			Foreground(lipgloss.Color(p.Danger)).
			Padding(0, 1),
		FilterHint: lipgloss.NewStyle().
			Foreground(lipgloss.Color(p.Accent)).
			Padding(0, 1),
	}
}
