package ui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// PaneHintHeight is the vertical space a hint bar occupies inside a pane.
const PaneHintHeight = 1

// PaneHint is a key/description pair for contextual pane hints.
type PaneHint struct {
	Key  string // e.g. "enter", "ctrl+f"
	Desc string // e.g. "open", "filter"
}

// RenderPaneHints renders a single-line hint bar for display at the bottom
// of a pane. Hints are separated by " · ". The result is truncated to width.
func RenderPaneHints(hints []PaneHint, styles Styles, width int) string {
	if len(hints) == 0 {
		return ""
	}

	var parts []string
	for _, h := range hints {
		parts = append(parts, h.Key+" "+styles.Muted.Render(h.Desc))
	}

	line := strings.Join(parts, styles.Muted.Render(" · "))

	// Truncate to available width.
	if lipgloss.Width(line) > width {
		line = line[:width]
	}

	return line
}
