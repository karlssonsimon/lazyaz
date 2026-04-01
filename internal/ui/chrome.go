package ui

import (
	"github.com/charmbracelet/lipgloss"
)

// ChromeStyles contains the application chrome styles (header, panes, status, etc.).
// Built by NewStyles from a Base16 scheme.
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
