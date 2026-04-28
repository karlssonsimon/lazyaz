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
	toastWidth       = 64
	toastMaxLines    = 3
	toastTopMargin   = 1
	toastRightMargin = 2
	toastVerticalGap = 1
	toastMaxOnScreen = 5
	toastBarChar     = "▍"
	// Layout: bar(1) + lpad(1) + icon(1) + iconGap(2) + msg + rpad(2)
	toastLPad    = 1
	toastIconGap = 2
	toastRPad    = 2
)

// Toast is a single notification rendered as a top-right popup.
// Toasts have a colored left bar keyed off the level.
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

		if ti < len(toasts)-1 {
			cursorY += toastVerticalGap
		}
	}

	return strings.Join(baseLines[:termHeight], "\n")
}

// renderToastBox returns the rendered lines of a single toast card.
// Layout: colored left bar + level icon + message body, wrapping to
// continuation lines indented under the message column. No top/bottom
// padding — the card is as tall as the wrapped message.
func renderToastBox(t Toast, styles Styles) []string {
	body, barStyle, iconStyle := toastStyles(t.Level, styles)

	icon := levelIcon(t.Level)
	if t.Spinner {
		icon = SpinnerFrameAt(time.Since(t.Time))
	}

	contentW := toastWidth - 1 - toastLPad - 1 - toastIconGap - toastRPad
	if contentW < 10 {
		contentW = 10
	}

	lines := wrapAndClamp(t.Message, contentW, toastMaxLines)
	if len(lines) == 0 {
		lines = []string{""}
	}

	bar := barStyle.Render(toastBarChar)
	leftPad := body.Render(strings.Repeat(" ", toastLPad))
	gap := body.Render(strings.Repeat(" ", toastIconGap))
	rightPad := body.Render(strings.Repeat(" ", toastRPad))
	indent := body.Render(strings.Repeat(" ", toastLPad+1+toastIconGap))

	rendered := make([]string, 0, len(lines))
	for i, line := range lines {
		msg := body.Render(padRight(line, contentW))
		var row string
		if i == 0 {
			row = bar + leftPad + iconStyle.Render(icon) + gap + msg + rightPad
		} else {
			row = bar + indent + msg + rightPad
		}
		rendered = append(rendered, row+ansiReset)
	}
	return rendered
}

// toastStyles returns body/bar/icon styles for a level.
func toastStyles(level ToastLevel, styles Styles) (lipgloss.Style, lipgloss.Style, lipgloss.Style) {
	bg := styles.Chrome.Status.GetBackground()
	body := lipgloss.NewStyle().
		Foreground(styles.Chrome.Status.GetForeground()).
		Background(bg)

	col := levelColor(level, styles)
	bar := lipgloss.NewStyle().Foreground(col).Background(bg).Bold(true)
	icon := lipgloss.NewStyle().Foreground(col).Background(bg).Bold(true)
	return body, bar, icon
}

func levelColor(level ToastLevel, styles Styles) icolor.Color {
	switch level {
	case ToastError:
		return styles.Danger.GetForeground()
	case ToastWarn:
		return styles.Warning.GetForeground()
	case ToastSuccess:
		return styles.Ok.GetForeground()
	default:
		return styles.Accent.GetForeground()
	}
}

func levelIcon(level ToastLevel) string {
	switch level {
	case ToastError:
		return "✗"
	case ToastWarn:
		return "!"
	case ToastSuccess:
		return "✓"
	default:
		return "·"
	}
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
// toast's colored bar) would absorb whatever foreground or background
// was active in the truncated tail of the base line.
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
