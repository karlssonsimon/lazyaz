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
	Query          string
	filtered       []int // indices into the themes slice
}

func (s *ThemeOverlayState) Open() {
	s.Active = true
	s.Query = ""
	s.filtered = nil
	s.CursorIdx = s.ActiveThemeIdx
}

func (s *ThemeOverlayState) refilter(themes []Theme) {
	if s.Query == "" {
		s.filtered = make([]int, len(themes))
		for i := range themes {
			s.filtered[i] = i
		}
	} else {
		q := strings.ToLower(s.Query)
		s.filtered = s.filtered[:0]
		for i, t := range themes {
			if strings.Contains(strings.ToLower(t.Name), q) {
				s.filtered = append(s.filtered, i)
			}
		}
	}
	if s.CursorIdx >= len(s.filtered) {
		s.CursorIdx = max(0, len(s.filtered)-1)
	}
}

func (s *ThemeOverlayState) selectedThemeIdx() (int, bool) {
	if len(s.filtered) == 0 || s.CursorIdx >= len(s.filtered) {
		return 0, false
	}
	return s.filtered[s.CursorIdx], true
}

func (s *ThemeOverlayState) HandleKey(key string, bindings ThemeKeyBindings, themes []Theme) (applied bool) {
	if len(themes) == 0 {
		s.Active = false
		return false
	}

	// Ensure filtered list is initialized.
	if s.filtered == nil {
		s.refilter(themes)
	}

	switch {
	case bindings.Up.Matches(key):
		if s.CursorIdx > 0 {
			s.CursorIdx--
		}
	case bindings.Down.Matches(key):
		if s.CursorIdx < len(s.filtered)-1 {
			s.CursorIdx++
		}
	case bindings.Apply.Matches(key):
		if idx, ok := s.selectedThemeIdx(); ok {
			s.ActiveThemeIdx = idx
			s.Active = false
			return true
		}
	case bindings.Cancel.Matches(key):
		s.Active = false
	case key == "backspace":
		if len(s.Query) > 0 {
			s.Query = s.Query[:len(s.Query)-1]
			s.refilter(themes)
		}
	default:
		if len(key) == 1 && key[0] >= 32 && key[0] < 127 {
			s.Query += key
			s.refilter(themes)
		}
	}
	return false
}

func RenderThemeOverlay(state ThemeOverlayState, themes []Theme, palette Palette, width, height int, base string) string {
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(palette.Accent))

	promptStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(palette.AccentStrong))

	inputStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(palette.Text))

	normalStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(palette.Text)).
		Padding(0, 1)

	cursorStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(palette.SelectedText)).
		Background(lipgloss.Color(palette.SelectedBg)).
		Bold(true).
		Padding(0, 1)

	noMatchStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(palette.Muted)).
		Italic(true)

	var rows []string
	rows = append(rows, titleStyle.Render("Select Theme"))

	inputLine := promptStyle.Render("> ") + inputStyle.Render(state.Query+"█")
	rows = append(rows, inputLine)
	rows = append(rows, "")

	filtered := state.filtered
	if filtered == nil {
		// Fallback: show all themes unfiltered.
		filtered = make([]int, len(themes))
		for i := range themes {
			filtered[i] = i
		}
	}

	if len(filtered) == 0 {
		rows = append(rows, noMatchStyle.Render("No matching themes"))
	} else {
		for ci, ti := range filtered {
			marker := "  "
			if ti == state.ActiveThemeIdx {
				marker = "* "
			}
			label := marker + themes[ti].Name
			if ci == state.CursorIdx {
				rows = append(rows, cursorStyle.Render(label))
			} else {
				rows = append(rows, normalStyle.Render(label))
			}
		}
	}

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
