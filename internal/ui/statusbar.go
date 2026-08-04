package ui

import (
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

const StatusBarHeight = 1

// pathSeparator is the breadcrumb glyph between brand/path segments.
// Chosen to match the visual mockup; falls back gracefully on terminals
// that lack the glyph since lipgloss treats it as a single-cell rune.
const pathSeparator = " › "

func RenderAppHeader(cfg HeaderConfig, styles Styles, width int) string {
	if width <= 0 {
		return ""
	}

	chrome := styles.Chrome
	sep := chrome.HeaderPathMuted.Render(pathSeparator)

	parts := make([]string, 0, len(cfg.Path)+1)
	if cfg.Brand != "" {
		parts = append(parts, chrome.HeaderBrand.Render(cfg.Brand))
	}
	for _, segment := range cfg.Path {
		if segment == "" {
			continue
		}
		parts = append(parts, chrome.HeaderPath.Render(segment))
	}

	left := strings.Join(parts, sep)
	return fitStatusLine(left, cfg.Meta, width, chrome.HeaderPathMuted)
}

func RenderStatusLine(cfg StatusLineConfig, styles Styles, width int) string {
	if width <= 0 {
		return ""
	}

	chrome := styles.Chrome
	parts := make([]string, 0, len(cfg.Actions)+1)
	if cfg.Mode != "" {
		parts = append(parts, chrome.StatusMode.Render(cfg.Mode))
	}
	for _, action := range cfg.Actions {
		if action.Key == "" {
			continue
		}
		label := action.Label
		if label != "" {
			label = " " + label
		}
		parts = append(parts, chrome.StatusKey.Render(action.Key)+chrome.Help.Render(label))
	}

	left := strings.Join(parts, chrome.Help.Render("  "))
	right := ""
	switch {
	// A pending count owns the right edge while it lasts — vim's
	// bottom-right count display. Rendering it there keeps the left-hand
	// hints from shifting as digits are typed.
	case cfg.Count > 0:
		right = chrome.StatusMode.Render(strconv.Itoa(cfg.Count))
	case cfg.Message != "" && cfg.IsError:
		right = chrome.Error.Render(cfg.Message)
	case cfg.Message != "":
		right = chrome.Help.Render(cfg.Message)
	}
	return fitStatusLine(left, right, width, chrome.Help)
}

func fitStatusLine(left, right string, width int, fill lipgloss.Style) string {
	if width <= 0 {
		return ""
	}
	gap := width - ansi.StringWidth(left) - ansi.StringWidth(right)
	if right == "" {
		gap = width - ansi.StringWidth(left)
	}
	if gap < 1 && right != "" {
		line := left + " " + right
		if ansi.StringWidth(line) > width {
			return ansi.Truncate(line, width, "")
		}
		return line
	}
	if gap < 0 {
		gap = 0
	}
	// The fill style may carry horizontal padding (chrome.Help does),
	// which renders on top of the gap and used to push the line past
	// width — truncating the right segment's tail. Budget the frame out
	// of the gap so the rendered filler occupies exactly gap columns.
	filler := ""
	if gap > 0 {
		if n := gap - fill.GetHorizontalFrameSize(); n >= 0 {
			filler = fill.Render(strings.Repeat(" ", n))
		} else {
			filler = strings.Repeat(" ", gap)
		}
	}
	line := left + filler + right
	if ansi.StringWidth(line) > width {
		return ansi.Truncate(line, width, "")
	}
	return line
}
