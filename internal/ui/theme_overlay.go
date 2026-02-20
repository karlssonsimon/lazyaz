package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

type KeyMatcher interface {
	Matches(key string) bool
}

type ThemeKeyBindings struct {
	Up, Down, Apply, Cancel KeyMatcher
}

type ThemeOverlayState struct {
	Active         bool
	ActiveThemeIdx int
	CursorIdx      int
}

func (s *ThemeOverlayState) Open() {
	s.Active = true
	s.CursorIdx = s.ActiveThemeIdx
}

func (s *ThemeOverlayState) HandleKey(key string, bindings ThemeKeyBindings, themes []Theme) (applied bool) {
	if len(themes) == 0 {
		s.Active = false
		return false
	}

	switch {
	case bindings.Up.Matches(key):
		if s.CursorIdx > 0 {
			s.CursorIdx--
		}
	case bindings.Down.Matches(key):
		if s.CursorIdx < len(themes)-1 {
			s.CursorIdx++
		}
	case bindings.Apply.Matches(key):
		s.ActiveThemeIdx = s.CursorIdx
		s.Active = false
		return true
	case bindings.Cancel.Matches(key):
		s.Active = false
	}
	return false
}

func RenderThemeOverlay(state ThemeOverlayState, themes []Theme, palette Palette, width, height int, base string) string {
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(palette.Accent)).
		Padding(0, 1)

	normalStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(palette.Text)).
		Padding(0, 1)

	cursorStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(palette.SelectedText)).
		Background(lipgloss.Color(palette.SelectedBg)).
		Bold(true).
		Padding(0, 1)

	hintStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(palette.Muted)).
		Padding(0, 1)

	var rows []string
	rows = append(rows, titleStyle.Render("Select Theme"))
	rows = append(rows, "")

	for i, t := range themes {
		marker := "  "
		if i == state.ActiveThemeIdx {
			marker = "* "
		}
		label := marker + t.Name
		if i == state.CursorIdx {
			rows = append(rows, cursorStyle.Render(label))
		} else {
			rows = append(rows, normalStyle.Render(label))
		}
	}

	rows = append(rows, "")
	rows = append(rows, hintStyle.Render("j/k navigate | enter apply | esc cancel"))

	content := lipgloss.JoinVertical(lipgloss.Left, rows...)

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(palette.BorderFocused)).
		Padding(1, 2)

	box := boxStyle.Render(content)

	return PlaceOverlay(width, height, box, base)
}

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
