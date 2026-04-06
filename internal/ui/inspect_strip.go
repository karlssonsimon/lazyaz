package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// InspectField is a label/value pair shown in an inspect strip.
type InspectField struct {
	Label string
	Value string
}

// InspectStripHeight returns the number of rows an inspect strip with
// the given fields will occupy when rendered. Composed of: one blank
// spacer above, one title row, one row per field, one blank spacer
// below. Apps must subtract this from the list height in resize() so
// the strip fits inside the pane's fixed frame.
func InspectStripHeight(fields []InspectField) int {
	return 3 + len(fields)
}

// RenderInspectStrip renders an inspect detail strip — title row
// followed by aligned label/value rows — for embedding inside a pane
// (typically between the list body and the hint row, via ListPane.Footer).
//
// Unlike the old modal overlay, this strip lives inside the pane and
// updates live as the cursor moves through the list, so the user can
// keep browsing while details remain visible.
func RenderInspectStrip(title string, fields []InspectField, styles Styles, width int) string {
	if width <= 0 {
		return ""
	}

	maxLabel := 0
	for _, f := range fields {
		if w := lipgloss.Width(f.Label); w > maxLabel {
			maxLabel = w
		}
	}

	rows := make([]string, 0, 3+len(fields))
	rows = append(rows, "")
	rows = append(rows, styles.Accent.Render(truncateAnsi(title, width)))

	for _, f := range fields {
		label := styles.Muted.Render(padRight(f.Label, maxLabel))
		valueWidth := width - maxLabel - 2
		if valueWidth < 1 {
			valueWidth = 1
		}
		value := truncateAnsi(f.Value, valueWidth)
		rows = append(rows, label+"  "+value)
	}
	rows = append(rows, "")

	return strings.Join(rows, "\n")
}
