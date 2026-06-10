package ui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// RenderFilterLine produces the compact `/ <query>█` line shown when
// a column is actively filtered. Typing (showCursor=true) draws the
// cursor *on* the rune at cursorPos so blink-off shows the underlying
// character; otherwise the line shows `/ <query>` as a persistent
// badge that a filter is applied. The line is padded with spaces to
// `width` so the surrounding row keeps a consistent bg fill.
//
// cursorView is the pre-styled cursor glyph as returned by
// `cursor.Model.View()` with `SetChar` set to the rune-at-cursor —
// most callers use `ui.PrepareCursor` to build the three pieces. When
// cursorView is empty, falls back to a static "█".
//
// Used in the focused column's footer slot so the filter scope stays
// adjacent to the column it filters without covering the column's
// table header.
func RenderFilterLine(before, cursorView, after string, query string, styles Styles, width int, showCursor bool) string {
	if width <= 0 {
		return ""
	}
	chrome := styles.Chrome
	prompt := chrome.HeaderPathMuted.Render("/")
	var body string
	if showCursor {
		if cursorView == "" {
			cursorView = "█"
		}
		body = chrome.HeaderPath.Render(before) + cursorView + chrome.HeaderPath.Render(after)
	} else {
		body = chrome.HeaderPath.Render(query)
	}
	line := prompt + " " + body
	if pad := width - ansi.StringWidth(line); pad > 0 {
		line += strings.Repeat(" ", pad)
	}
	if ansi.StringWidth(line) > width {
		line = ansi.Truncate(line, width, "")
	}
	return line
}
