package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

type HelpOverlayState struct {
	Active bool
}

func (s *HelpOverlayState) Toggle() {
	s.Active = !s.Active
}

func (s *HelpOverlayState) Close() {
	s.Active = false
}

type HelpSection struct {
	Title string
	Items []string
}

func RenderHelpOverlay(title string, sections []HelpSection, palette Palette, width, height int, base string) string {
	boxWidth := width * 4 / 5
	boxHeight := height * 4 / 5
	if boxWidth < 48 {
		boxWidth = 48
	}
	if boxHeight < 12 {
		boxHeight = 12
	}
	if boxWidth > width {
		boxWidth = width
	}
	if boxHeight > height {
		boxHeight = height
	}

	innerWidth := boxWidth - 6
	innerHeight := boxHeight - 4
	if innerWidth < 1 {
		innerWidth = 1
	}
	if innerHeight < 1 {
		innerHeight = 1
	}

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(palette.Accent))

	sectionTitleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(palette.Accent))

	bodyStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(palette.Text)).
		Width(innerWidth)

	hintStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(palette.Muted)).
		Width(innerWidth)

	rows := []string{titleStyle.Render(title), ""}
	for i, section := range sections {
		if i > 0 {
			rows = append(rows, "")
		}
		rows = append(rows, sectionTitleStyle.Render(section.Title))
		for _, item := range section.Items {
			rows = append(rows, bodyStyle.Render(item))
		}
	}
	rows = append(rows, "", hintStyle.Render("?: close | esc close"))

	content := fitOverlayContent(rows, innerWidth, innerHeight)
	box := lipgloss.NewStyle().
		Width(boxWidth).
		Height(boxHeight).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(palette.BorderFocused)).
		Padding(1, 2).
		Render(content)

	return PlaceOverlay(width, height, box, base)
}

func fitOverlayContent(rows []string, width, height int) string {
	if height <= 0 {
		return ""
	}

	var lines []string
	for _, row := range rows {
		wrapped := lipgloss.NewStyle().Width(width).Render(row)
		lines = append(lines, strings.Split(wrapped, "\n")...)
	}

	if len(lines) > height {
		lines = append(lines[:height-1], fmt.Sprintf("... (%d more lines)", len(lines)-height+1))
	}
	for len(lines) < height {
		lines = append(lines, "")
	}

	return strings.Join(lines, "\n")
}
