package ui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

const conflictInnerWidth = 72

// RenderUploadConflictPrompt paints a modal asking how to resolve an
// upload conflict for blobName. Layout matches the confirm-modal language:
// CONFLICT pill (warning bg) + path breadcrumb, body explanation, footer
// with the five action keys.
func RenderUploadConflictPrompt(blobName string, styles Styles, width, height int, base string) string {
	innerW := conflictInnerWidth
	boxW := innerW + 6
	if boxW > width-4 {
		boxW = width - 4
		innerW = boxW - 6
	}
	if innerW < 40 {
		innerW = 40
		boxW = innerW + 6
	}

	ov := styles.Overlay
	rows := []string{renderConflictHeader(blobName, styles, innerW)}
	rows = append(rows, ov.Rule.Render(strings.Repeat("─", innerW)))
	rows = append(rows, "")
	rows = append(rows, "A blob with this name already exists in the container.")
	rows = append(rows, "")
	rows = append(rows, ov.Rule.Render(strings.Repeat("─", innerW)))
	rows = append(rows, renderConflictFooter(styles, innerW))

	content := lipgloss.JoinVertical(lipgloss.Left, rows...)
	box := ov.Box.Width(boxW).Render(content)
	return PlaceOverlay(width, height, box, base)
}

func renderConflictHeader(blobName string, styles Styles, innerW int) string {
	ov := styles.Overlay
	chevron := ov.Hint.Inline(true).Padding(0).Render(overlayChevron)
	right := ov.Hint.Inline(true).Padding(0).Render("esc cancel")

	badge := lipgloss.NewStyle().
		Foreground(ov.HeaderBadge.GetForeground()).
		Background(styles.Warning.GetForeground()).
		Bold(true).
		Padding(0, 1).
		Render("CONFLICT")

	budget := innerW - lipgloss.Width(badge) - lipgloss.Width(right) - 1 - lipgloss.Width(chevron)
	if budget < 8 {
		budget = 8
	}
	name := truncateLabel(blobName, budget)
	left := badge + chevron + ov.Input.Render(name)
	return overlayJustifyRow(left, right, innerW, ov)
}

func renderConflictFooter(styles Styles, innerW int) string {
	chrome := styles.Chrome
	ov := styles.Overlay
	dim := ov.Hint.Inline(true).Padding(0)

	mode := chrome.StatusMode.Render("CONFLICT")
	parts := []string{
		mode,
		chrome.StatusKey.Render("y") + dim.Render(" overwrite"),
		chrome.StatusKey.Render("n") + dim.Render(" skip"),
		chrome.StatusKey.Render("a") + dim.Render(" overwrite all"),
		chrome.StatusKey.Render("s") + dim.Render(" skip all"),
		chrome.StatusKey.Render("c") + dim.Render(" cancel"),
	}
	left := strings.Join(parts, dim.Render("  "))
	return overlayJustifyRow(left, "", innerW, ov)
}
