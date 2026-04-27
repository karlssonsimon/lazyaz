package ui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

const StatusBarHeight = 1

// StatusBarItem is a label/value pair displayed in the status bar.
type StatusBarItem struct {
	Label string
	Value string
}

// StatusBarStyles contains all styles for the status bar.
type StatusBarStyles struct {
	Box   lipgloss.Style
	Label lipgloss.Style
	Value lipgloss.Style
	Error lipgloss.Style
	Gap   lipgloss.Style
}

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
	case cfg.Message != "" && cfg.IsError:
		right = chrome.Error.Render(cfg.Message)
	case cfg.Message != "":
		right = chrome.Help.Render(cfg.Message)
	}
	return fitStatusLine(left, right, width, chrome.Help)
}

// RenderStatusBar keeps the existing call sites working while rendering the new one-line status.
func RenderStatusBar(styles Styles, items []StatusBarItem, status string, isErr bool, width int) string {
	actions := make([]StatusAction, 0, len(items))
	for _, item := range items {
		if item.Value == "" {
			continue
		}
		key := item.Label
		label := item.Value
		if key == "" {
			key = item.Value
			label = ""
		}
		actions = append(actions, StatusAction{Key: key, Label: label})
	}
	return RenderStatusLine(StatusLineConfig{Actions: actions, Message: status, IsError: isErr}, styles, width)
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
	line := left + fill.Render(strings.Repeat(" ", gap)) + right
	if ansi.StringWidth(line) > width {
		return ansi.Truncate(line, width, "")
	}
	return line
}
