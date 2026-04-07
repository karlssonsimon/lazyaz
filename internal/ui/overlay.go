package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

const (
	overlayInnerWidth = 60
	overlayBoxWidth   = overlayInnerWidth + 6 // padding(2+2) + border(1+1)
	overlayMaxVisible = 20
)

// OverlayItem is a single entry in an overlay list.
type OverlayItem struct {
	Label    string
	Desc     string // optional second line below label (muted)
	Hint     string // right-aligned secondary text (shortcut, author, etc.)
	IsActive bool   // shows a marker (e.g. * for current theme)
}

// OverlayListConfig configures the dimensions and placement of an overlay list.
type OverlayListConfig struct {
	Title      string
	Query      string
	CloseHint  string // label shown right-aligned in the header (e.g. "esc"); blank to omit
	InnerWidth int    // content width; 0 = default (60)
	MaxVisible int    // max visible items; 0 = default (20)
	Center     bool   // center vertically instead of 1/5 from top
}

// RenderOverlayList renders a configurable overlay with a title bar, search
// input, and a scrollable item list. The command palette, theme picker, and
// help overlay use this to maintain a consistent look.
func RenderOverlayList(cfg OverlayListConfig, items []OverlayItem, cursor int, styles OverlayStyles, termWidth, termHeight int, base string) string {
	innerW := cfg.InnerWidth
	if innerW <= 0 {
		innerW = overlayInnerWidth
	}
	boxW := innerW + 6
	if boxW > termWidth-4 {
		boxW = termWidth - 4
		innerW = boxW - 6
	}
	if innerW < 20 {
		innerW = 20
	}

	maxVis := cfg.MaxVisible
	if maxVis <= 0 {
		maxVis = overlayMaxVisible
	}

	// The Normal/Cursor styles include Padding(0,1) = 2 chars horizontal.
	padH := styles.Normal.GetHorizontalPadding()
	contentW := innerW - padH

	normalStyle := styles.Normal.Width(innerW)
	cursorStyle := styles.Cursor.Width(innerW)

	var rows []string

	// Header: title left, close hint right (caller-supplied so the
	// label honors the keymap's actual cancel binding).
	titleText := styles.Title.Render(cfg.Title)
	closeLabel := cfg.CloseHint
	if closeLabel == "" {
		closeLabel = "esc" // sensible default if the caller didn't pass one
	}
	closeText := styles.Hint.Render(closeLabel)
	titleW := lipgloss.Width(titleText)
	closeW := lipgloss.Width(closeText)
	gap := innerW - titleW - closeW
	if gap < 1 {
		gap = 1
	}
	rows = append(rows, titleText+strings.Repeat(" ", gap)+closeText)

	// Search input.
	if cfg.Query == "" {
		rows = append(rows, styles.NoMatch.Render("█"))
	} else {
		rows = append(rows, styles.Input.Render(cfg.Query+"█"))
	}
	rows = append(rows, "")

	// Pre-render a single empty row for padding (avoids repeated lipgloss work).
	emptyRow := normalStyle.Render("")

	// Scrollable item list.
	if len(items) == 0 {
		rows = append(rows, styles.NoMatch.Render("No matches"))
		// Pad remaining rows.
		for i := 1; i < maxVis; i++ {
			rows = append(rows, emptyRow)
		}
	} else {
		visible := min(maxVis, len(items))

		// Scroll window around cursor.
		start := 0
		if cursor >= start+visible {
			start = cursor - visible + 1
		}
		if cursor < start {
			start = cursor
		}
		end := start + visible
		if end > len(items) {
			end = len(items)
			start = max(0, end-visible)
		}

		itemRows := 0
		for ci := start; ci < end; ci++ {
			item := items[ci]
			marker := "  "
			if item.IsActive {
				marker = "• "
			}

			label := marker + item.Label
			hint := item.Hint

			nameWidth := contentW
			if hint != "" {
				nameWidth = contentW - lipgloss.Width(hint) - 2
			}
			if nameWidth < 10 {
				nameWidth = 10
			}
			entry := padRight(label, nameWidth)
			if hint != "" {
				entry += "  " + hint
			}

			style := normalStyle
			if ci == cursor {
				style = cursorStyle
			}
			rows = append(rows, style.Render(entry))
			itemRows++

			if item.Desc != "" {
				rows = append(rows, style.Render("  "+item.Desc))
				itemRows++
			}
		}

		// Pad to constant height based on max visible row count.
		// If items have Desc fields, each takes 2 rows.
		hasDesc := len(items) > 0 && items[0].Desc != ""
		rowsPerItem := 1
		if hasDesc {
			rowsPerItem = 2
		}
		targetRows := maxVis * rowsPerItem
		for i := itemRows; i < targetRows; i++ {
			rows = append(rows, emptyRow)
		}
	}

	content := lipgloss.JoinVertical(lipgloss.Left, rows...)
	box := styles.Box.Width(boxW).Render(content)

	if cfg.Center {
		return PlaceOverlay(termWidth, termHeight, box, base)
	}
	return placeOverlayTop(termWidth, termHeight, box, base)
}

// placeOverlayTop places the overlay near the top (1/5 down) and centered
// horizontally.
func placeOverlayTop(width, height int, overlay, base string) string {
	overlayLines := strings.Split(overlay, "\n")
	baseLines := strings.Split(base, "\n")

	for len(baseLines) < height {
		baseLines = append(baseLines, "")
	}

	oH := len(overlayLines)
	oW := 0
	for _, l := range overlayLines {
		if w := lipgloss.Width(l); w > oW {
			oW = w
		}
	}

	startY := height / 5
	startX := (width - oW) / 2
	if startY < 1 {
		startY = 1
	}
	if startX < 0 {
		startX = 0
	}
	if startY+oH > height {
		startY = max(0, height-oH)
	}

	for i, ol := range overlayLines {
		row := startY + i
		if row >= len(baseLines) {
			break
		}
		line := baseLines[row]
		lineW := lipgloss.Width(line)

		var out strings.Builder
		if startX > 0 {
			if lineW >= startX {
				out.WriteString(truncateAnsi(line, startX))
			} else {
				out.WriteString(line)
				out.WriteString(strings.Repeat(" ", startX-lineW))
			}
		}
		out.WriteString(ol)
		rightCol := startX + oW
		if lineW > rightCol {
			out.WriteString(skipAnsi(line, rightCol))
		}
		baseLines[row] = out.String()
	}

	return strings.Join(baseLines[:height], "\n")
}

// PlaceOverlay places the overlay centered on screen.
func PlaceOverlay(width, height int, overlay, base string) string {
	overlayLines := strings.Split(overlay, "\n")
	baseLines := strings.Split(base, "\n")

	for len(baseLines) < height {
		baseLines = append(baseLines, "")
	}

	oH := len(overlayLines)
	oW := 0
	for _, l := range overlayLines {
		if w := lipgloss.Width(l); w > oW {
			oW = w
		}
	}

	startY := (height - oH) / 2
	startX := (width - oW) / 2
	if startY < 0 {
		startY = 0
	}
	if startX < 0 {
		startX = 0
	}

	for i, ol := range overlayLines {
		row := startY + i
		if row >= len(baseLines) {
			break
		}
		line := baseLines[row]
		lineW := lipgloss.Width(line)

		var out strings.Builder
		if startX > 0 {
			if lineW >= startX {
				out.WriteString(truncateAnsi(line, startX))
			} else {
				out.WriteString(line)
				out.WriteString(strings.Repeat(" ", startX-lineW))
			}
		}
		out.WriteString(ol)
		rightCol := startX + oW
		if lineW > rightCol {
			out.WriteString(skipAnsi(line, rightCol))
		}
		baseLines[row] = out.String()
	}

	return strings.Join(baseLines[:height], "\n")
}

func skipAnsi(s string, skipWidth int) string {
	runes := []rune(s)
	for i := 0; i <= len(runes); i++ {
		prefix := string(runes[:i])
		if lipgloss.Width(prefix) >= skipWidth {
			return string(runes[i:])
		}
	}
	return ""
}

// padRight pads s with spaces to reach the given display width.
func padRight(s string, width int) string {
	w := lipgloss.Width(s)
	if w >= width {
		return s
	}
	return s + strings.Repeat(" ", width-w)
}

func truncateAnsi(s string, maxWidth int) string {
	if lipgloss.Width(s) <= maxWidth {
		return s
	}
	runes := []rune(s)
	for i := len(runes); i > 0; i-- {
		candidate := string(runes[:i])
		if lipgloss.Width(candidate) <= maxWidth {
			return candidate
		}
	}
	return ""
}
