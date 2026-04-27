package ui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// RenderFilterLine produces the compact `/ <query>█` line shown when
// a column is actively filtered. Typing (showCursor=true) appends the
// cursor; otherwise the line shows `/ <query>` as a persistent badge
// that a filter is applied. The line is padded with spaces to `width`
// so the surrounding row keeps a consistent bg fill.
//
// Used in the focused column's footer slot so the filter scope stays
// adjacent to the column it filters without covering the column's
// table header.
func RenderFilterLine(query, cursorView string, styles Styles, width int, showCursor bool) string {
	if width <= 0 {
		return ""
	}
	chrome := styles.Chrome
	prompt := chrome.HeaderPathMuted.Render("/")
	body := chrome.HeaderPath.Render(query)
	line := prompt + " " + body
	if showCursor {
		if cursorView == "" {
			cursorView = "█"
		}
		line += cursorView
	}
	if pad := width - ansi.StringWidth(line); pad > 0 {
		line += strings.Repeat(" ", pad)
	}
	if ansi.StringWidth(line) > width {
		line = ansi.Truncate(line, width, "")
	}
	return line
}
