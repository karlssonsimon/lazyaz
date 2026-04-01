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
	filtered       []int
}

func (s *ThemeOverlayState) Open() {
	s.Active = true
	s.Query = ""
	s.filtered = nil
	s.CursorIdx = s.ActiveThemeIdx
}

func (s *ThemeOverlayState) refilter(schemes []Scheme) {
	if s.Query == "" {
		s.filtered = make([]int, len(schemes))
		for i := range schemes {
			s.filtered[i] = i
		}
	} else {
		q := strings.ToLower(s.Query)
		s.filtered = s.filtered[:0]
		for i, sc := range schemes {
			if strings.Contains(strings.ToLower(sc.Name), q) {
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

func (s *ThemeOverlayState) HandleKey(key string, bindings ThemeKeyBindings, schemes []Scheme) (applied bool) {
	if len(schemes) == 0 {
		s.Active = false
		return false
	}

	if s.filtered == nil {
		s.refilter(schemes)
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
			s.refilter(schemes)
		}
	default:
		if len(key) == 1 && key[0] >= 32 && key[0] < 127 {
			s.Query += key
			s.refilter(schemes)
		}
	}
	return false
}

func RenderThemeOverlay(state ThemeOverlayState, schemes []Scheme, styles Styles, width, height int, base string) string {
	o := styles.Overlay

	var rows []string
	rows = append(rows, o.Title.Render("Select Theme"))

	inputLine := o.Prompt.Render("> ") + o.Input.Render(state.Query+"█")
	rows = append(rows, inputLine)
	rows = append(rows, "")

	filtered := state.filtered
	if filtered == nil {
		filtered = make([]int, len(schemes))
		for i := range schemes {
			filtered[i] = i
		}
	}

	if len(filtered) == 0 {
		rows = append(rows, o.NoMatch.Render("No matching themes"))
	} else {
		for ci, ti := range filtered {
			marker := "  "
			if ti == state.ActiveThemeIdx {
				marker = "* "
			}
			label := marker + schemes[ti].Name
			if ci == state.CursorIdx {
				rows = append(rows, o.Cursor.Render(label))
			} else {
				rows = append(rows, o.Normal.Render(label))
			}
		}
	}

	content := lipgloss.JoinVertical(lipgloss.Left, rows...)
	box := o.Box.Render(content)

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
