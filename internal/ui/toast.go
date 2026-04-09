package ui

import (
	icolor "image/color"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// ToastLevel mirrors appshell.NotificationLevel without importing it
// (the ui package is a leaf — appshell imports it, not the other way
// around). Callers convert at the boundary.
type ToastLevel int

const (
	ToastInfo ToastLevel = iota
	ToastSuccess
	ToastWarn
	ToastError
)

const (
	toastWidth        = 50 // outer width incl. left border + padding
	toastMaxLines     = 4  // hard clamp on message body lines
	toastTopMargin    = 1  // rows below the top of the screen
	toastRightMargin  = 2  // columns from the right edge
	toastVerticalGap  = 1  // blank rows between stacked toasts
	toastMaxOnScreen  = 5  // never render more than this many at once
	toastBorderCharL  = "▎"
	toastBorderCharR  = "▕"
	toastInnerPadding = 1 // spaces between border and message text
)

// Toast is a single notification rendered as a top-right popup.
// Toasts have a colored left border keyed off the level.
type Toast struct {
	Level   ToastLevel
	Message string
	Spinner bool      // when true, prepend animated spinner frame
	Time    time.Time // used for spinner frame computation
}

// RenderToasts paints the active toast stack on top of the given base
// view in the top-right corner. toasts is expected to be in
// newest-first order (Notifier.Active already returns it that way).
// If toasts is empty, the base view is returned untouched.
func RenderToasts(toasts []Toast, styles Styles, termWidth, termHeight int, base string) string {
	if len(toasts) == 0 || termWidth < toastWidth+toastRightMargin+1 {
		return base
	}

	if len(toasts) > toastMaxOnScreen {
		toasts = toasts[:toastMaxOnScreen]
	}

	startX := termWidth - toastWidth - toastRightMargin
	if startX < 0 {
		startX = 0
	}
	cursorY := toastTopMargin

	baseLines := strings.Split(base, "\n")
	for len(baseLines) < termHeight {
		baseLines = append(baseLines, "")
	}

	// gapRow is an opaque blank line the width of a toast, painted
	// between stacked toasts so the underlying view doesn't bleed
	// through the vertical gap. Without this the gap row shows
	// whatever was below — usually a pane border or footer hint —
	// which looks like the next toast lost its body.
	for ti, t := range toasts {
		boxLines := renderToastBox(t, styles)
		if cursorY+len(boxLines) > termHeight {
			break
		}
		for i, ol := range boxLines {
			row := cursorY + i
			baseLines[row] = overlayInto(baseLines[row], ol, startX, toastWidth)
		}
		cursorY += len(boxLines)

		// Skip gap rows between toasts — the top/bottom padding inside
		// each toast box already provides visual separation.
		if ti < len(toasts)-1 {
			cursorY += toastVerticalGap
		}
	}

	return strings.Join(baseLines[:termHeight], "\n")
}

// buildGapRow returns an opaque blank line of toastWidth columns,
// styled with the body background, for use as the spacer between
// stacked toasts. The body background matches the toast box so the
// gap visually belongs to the toast stack.
func buildGapRow(styles Styles) string {
	body := lipgloss.NewStyle().Background(styles.Chrome.Status.GetBackground())
	return body.Render(strings.Repeat(" ", toastWidth)) + ansiReset
}

// renderToastBox returns the rendered lines of a single toast box.
// The first column is the colored level border; the rest is the
// wrapped, clamped message body.
func renderToastBox(t Toast, styles Styles) []string {
	border, body := toastStyles(t.Level, styles)

	borderLW := lipgloss.Width(toastBorderCharL)
	borderRW := lipgloss.Width(toastBorderCharR)
	bodyW := toastWidth - borderLW - borderRW - toastInnerPadding*2
	if bodyW < 10 {
		bodyW = 10
	}

	msg := t.Message
	if t.Spinner {
		msg = SpinnerFrameAt(time.Since(t.Time)) + " " + msg
	}
	wrapped := wrapAndClamp(msg, bodyW, toastMaxLines)
	pad := strings.Repeat(" ", toastInnerPadding)
	left := border.Render(toastBorderCharL)
	right := border.Render(toastBorderCharR)

	// Build each line by hand: left border + styled body + right border + reset.
	// We avoid lipgloss Width() which can miscount columns with special chars.
	renderLine := func(text string) string {
		// Ensure the body content is exactly the right width.
		inner := pad + padRight(text, bodyW) + pad
		return left + body.Render(inner) + ansiReset + right + ansiReset
	}

	emptyLine := renderLine("")
	rendered := make([]string, 0, len(wrapped)+2)
	rendered = append(rendered, emptyLine)
	for _, line := range wrapped {
		rendered = append(rendered, renderLine(line))
	}
	rendered = append(rendered, emptyLine)
	return rendered
}

// toastStyles returns (left-border style, body style) for a level.
func toastStyles(level ToastLevel, styles Styles) (lipgloss.Style, lipgloss.Style) {
	body := lipgloss.NewStyle().
		Background(styles.Chrome.Status.GetBackground())

	var borderColor icolor.Color
	switch level {
	case ToastError:
		borderColor = styles.Danger.GetForeground()
	case ToastWarn:
		borderColor = styles.Warning.GetForeground()
	case ToastSuccess:
		// FocusBorder is the green base0B; reusing it keeps the
		// success-indicator color consistent with focused panes.
		borderColor = styles.FocusBorder.GetForeground()
	default:
		borderColor = styles.Accent.GetForeground()
	}

	border := lipgloss.NewStyle().
		Foreground(borderColor).
		Background(body.GetBackground()).
		Bold(true)

	return border, body
}

// wrapAndClamp word-wraps msg to width w (greedy), then clamps to at
// most maxLines lines. The last line gets a trailing "…" if anything
// was dropped.
func wrapAndClamp(msg string, w, maxLines int) []string {
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return []string{""}
	}

	var lines []string
	for _, paragraph := range strings.Split(msg, "\n") {
		lines = append(lines, wrapLine(paragraph, w)...)
	}

	if len(lines) <= maxLines {
		return lines
	}

	clipped := lines[:maxLines]
	last := clipped[maxLines-1]
	if lipgloss.Width(last) > w-1 {
		last = truncateAnsi(last, w-1)
	}
	clipped[maxLines-1] = last + "…"
	return clipped
}

// wrapLine greedy-wraps a single paragraph to the given width.
func wrapLine(p string, w int) []string {
	p = strings.TrimSpace(p)
	if p == "" {
		return []string{""}
	}

	words := strings.Fields(p)
	var lines []string
	var current string
	for _, word := range words {
		// Words longer than the width get hard-broken.
		for lipgloss.Width(word) > w {
			lines = append(lines, truncateAnsi(word, w))
			word = skipAnsi(word, w)
		}
		if current == "" {
			current = word
			continue
		}
		if lipgloss.Width(current)+1+lipgloss.Width(word) > w {
			lines = append(lines, current)
			current = word
			continue
		}
		current += " " + word
	}
	if current != "" {
		lines = append(lines, current)
	}
	return lines
}

// ansiReset is the SGR reset escape sequence. We prepend this to the
// overlay before pasting it into the base line so the overlay can't
// inherit any unclosed styling left over by truncateAnsi cutting the
// base mid-style. Without this the first cell of the overlay (the
// toast's colored border bar) would absorb whatever foreground or
// background was active in the truncated tail of the base line.
const ansiReset = "\x1b[0m"

// overlayInto pastes overlayCol into baseLine at startX, replacing the
// columns it covers. The overlay is treated as opaque (any cells
// underneath at columns [startX, startX+width) are dropped). Cells
// outside that range survive untouched.
//
// Uses ansi.Cut to correctly preserve ANSI styling state on both sides
// of the splice so colors don't bleed across the overlay boundaries.
func overlayInto(baseLine, overlayCol string, startX, width int) string {
	lineW := lipgloss.Width(baseLine)

	var out strings.Builder

	// Left portion of the base line: columns [0, startX).
	if startX > 0 {
		if lineW >= startX {
			out.WriteString(ansi.Cut(baseLine, 0, startX))
		} else {
			out.WriteString(baseLine)
			out.WriteString(strings.Repeat(" ", startX-lineW))
		}
	}

	out.WriteString(ansiReset)
	out.WriteString(overlayCol)
	out.WriteString(ansiReset)

	// Right portion of the base line: columns [startX+width, ...).
	rightCol := startX + width
	if lineW > rightCol {
		out.WriteString(ansi.Cut(baseLine, rightCol, lineW))
	}

	return out.String()
}
