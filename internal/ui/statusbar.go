package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// StatusBarHeight is the total height of the status bar: 3 content lines + 2 padding (Padding(1,1)).
const StatusBarHeight = 5

// StatusBarItem is a label/value pair displayed in the status bar.
type StatusBarItem struct {
	Label string
	Value string
}

// StatusBarStyles contains all styles for the status bar.
type StatusBarStyles struct {
	Box   lipgloss.Style // Container with background and padding
	Label lipgloss.Style // Gray label text
	Value lipgloss.Style // Bold white value text
	Error lipgloss.Style // Bold red error text
	Gap   lipgloss.Style // Background-only style for spaces between segments
}

// RenderStatusBar renders a fixed-height status bar with label/value
// pairs on the left and a status message on the right. It always
// renders exactly 3 content lines. The isErr flag controls whether
// the status text is rendered with the Error style.
func RenderStatusBar(styles Styles, items []StatusBarItem, status string, isErr bool, width int) string {
	s := styles.StatusBar
	innerWidth := width - 2 // account for box horizontal padding

	// Build item lines.
	var lines []string
	for _, item := range items {
		if item.Value == "" {
			continue
		}
		label := s.Label.Render(item.Label)
		gap := s.Gap.Render(" ")
		value := s.Value.Render(item.Value)
		line := label + gap + value
		lines = append(lines, line)
	}

	// Right-align status on the first line.
	if status != "" {
		var styledStatus string
		if isErr {
			styledStatus = s.Error.Render(status)
		} else {
			styledStatus = s.Label.Render(status)
		}

		if len(lines) > 0 {
			first := lines[0]
			gap := innerWidth - lipgloss.Width(first) - lipgloss.Width(styledStatus)
			if gap < 1 {
				gap = 1
			}
			lines[0] = first + s.Gap.Render(strings.Repeat(" ", gap)) + styledStatus
		} else {
			gap := innerWidth - lipgloss.Width(styledStatus)
			if gap < 0 {
				gap = 0
			}
			lines = append(lines, s.Gap.Render(strings.Repeat(" ", gap))+styledStatus)
		}
	}

	// Pad each line to full width so the background is solid.
	for i, line := range lines {
		lineWidth := lipgloss.Width(line)
		if lineWidth < innerWidth {
			lines[i] = line + s.Gap.Render(strings.Repeat(" ", innerWidth-lineWidth))
		}
	}

	// Always render exactly 3 content lines.
	for len(lines) < 3 {
		lines = append(lines, s.Gap.Render(strings.Repeat(" ", innerWidth)))
	}

	content := strings.Join(lines[:3], "\n")
	return s.Box.Width(width).Render(content)
}
