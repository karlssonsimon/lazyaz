package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// InspectField is a label/value pair shown in the inspect overlay.
type InspectField struct {
	Label string
	Value string
}

// RenderInspectOverlay renders a detail overlay showing fields for the selected item.
func RenderInspectOverlay(title string, fields []InspectField, styles Styles, width, height int, base string) string {
	o := styles.Overlay

	// Compute label column width.
	maxLabel := 0
	for _, f := range fields {
		if w := lipgloss.Width(f.Label); w > maxLabel {
			maxLabel = w
		}
	}

	var rows []string
	rows = append(rows, o.Title.Render(title), "")

	for _, f := range fields {
		label := o.Hint.Render(padRight(f.Label, maxLabel))
		value := o.Input.Render(f.Value)
		rows = append(rows, label+"  "+value)
	}

	rows = append(rows, "", o.Hint.Render("K close"))

	content := strings.Join(rows, "\n")

	innerW := maxLabel + 2 + 40
	if innerW < 50 {
		innerW = 50
	}
	if innerW > width-10 {
		innerW = width - 10
	}

	boxW := innerW + 6
	box := lipgloss.NewStyle().
		Width(boxW).
		Background(o.BoxBg).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(o.Box.GetBorderBottomForeground()).
		BorderBackground(o.BoxBg).
		Padding(1, 2).
		Render(content)

	return PlaceOverlay(width, height, box, base)
}
