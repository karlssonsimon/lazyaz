package app

import (
	"fmt"
	"strings"

	"azure-storage/internal/ui"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// command represents a single entry in the command palette.
type command struct {
	name     string
	hint     string // keyboard shortcut shown on the right
	action   func() commandAction
}

// commandAction is the result of executing a command.
type commandAction struct {
	msg tea.Msg // if non-nil, injected as a message
	cmd tea.Cmd // if non-nil, returned as a command
	quit bool   // shortcut for tea.Quit
}

// commandPalette manages the state of the command palette overlay.
type commandPalette struct {
	active   bool
	query    string
	cursor   int
	commands []command
	filtered []int // indices into commands
}

func (p *commandPalette) open(commands []command) {
	p.active = true
	p.query = ""
	p.cursor = 0
	p.commands = commands
	p.refilter()
}

func (p *commandPalette) close() {
	p.active = false
	p.query = ""
	p.cursor = 0
	p.filtered = nil
}

func (p *commandPalette) refilter() {
	if p.query == "" {
		p.filtered = make([]int, len(p.commands))
		for i := range p.commands {
			p.filtered[i] = i
		}
	} else {
		q := strings.ToLower(p.query)
		p.filtered = p.filtered[:0]
		for i, cmd := range p.commands {
			if strings.Contains(strings.ToLower(cmd.name), q) {
				p.filtered = append(p.filtered, i)
			}
		}
	}
	if p.cursor >= len(p.filtered) {
		p.cursor = max(0, len(p.filtered)-1)
	}
}

func (p *commandPalette) selectedCommand() (command, bool) {
	if len(p.filtered) == 0 || p.cursor >= len(p.filtered) {
		return command{}, false
	}
	return p.commands[p.filtered[p.cursor]], true
}

func (p *commandPalette) handleKey(key string) (cmd command, executed bool, closed bool) {
	switch key {
	case "esc", "ctrl+c", "ctrl+p":
		p.close()
		return command{}, false, true
	case "up", "ctrl+k":
		if p.cursor > 0 {
			p.cursor--
		}
	case "down", "ctrl+j":
		if p.cursor < len(p.filtered)-1 {
			p.cursor++
		}
	case "enter":
		if c, ok := p.selectedCommand(); ok {
			p.close()
			return c, true, false
		}
	case "backspace":
		if len(p.query) > 0 {
			p.query = p.query[:len(p.query)-1]
			p.refilter()
		}
	default:
		// Only accept printable single characters for the filter.
		if len(key) == 1 && key[0] >= 32 && key[0] < 127 {
			p.query += key
			p.refilter()
		}
	}
	return command{}, false, false
}

func renderCommandPalette(p *commandPalette, overlay ui.OverlayStyles, width, height int, base string) string {
	boxWidth := width / 2
	if boxWidth < 40 {
		boxWidth = 40
	}
	if boxWidth > width-4 {
		boxWidth = width - 4
	}
	innerWidth := boxWidth - 6

	normalStyle := overlay.Normal.Width(innerWidth)
	cursorStyle := overlay.Cursor.Width(innerWidth)

	var rows []string
	rows = append(rows, overlay.Title.Render("Command Palette"))

	// Input line.
	cursor := "█"
	inputLine := overlay.Prompt.Render("> ") + overlay.Input.Render(p.query+cursor)
	rows = append(rows, inputLine)
	rows = append(rows, "")

	if len(p.filtered) == 0 {
		rows = append(rows, overlay.NoMatch.Render("No matching commands"))
	} else {
		maxVisible := height/2 - 6
		if maxVisible < 5 {
			maxVisible = 5
		}
		if maxVisible > len(p.filtered) {
			maxVisible = len(p.filtered)
		}

		// Scroll window around cursor.
		start := 0
		if p.cursor >= maxVisible {
			start = p.cursor - maxVisible + 1
		}
		end := start + maxVisible
		if end > len(p.filtered) {
			end = len(p.filtered)
			start = max(0, end-maxVisible)
		}

		for _, idx := range p.filtered[start:end] {
			c := p.commands[idx]
			label := c.name
			hint := ""
			if c.hint != "" {
				hint = c.hint
			}

			// Pad name to fill width, right-align hint.
			nameWidth := innerWidth - lipgloss.Width(hint) - 2
			if nameWidth < 10 {
				nameWidth = 10
			}
			entry := fmt.Sprintf("%-*s", nameWidth, label)
			if hint != "" {
				entry += "  " + hint
			}

			realIdx := idx
			isCursor := false
			for ci, fi := range p.filtered {
				if fi == realIdx && ci == p.cursor {
					isCursor = true
					break
				}
			}

			if isCursor {
				rows = append(rows, cursorStyle.Render(entry))
			} else {
				rows = append(rows, normalStyle.Render(entry))
			}
		}
	}

	content := lipgloss.JoinVertical(lipgloss.Left, rows...)

	box := overlay.Box.
		Width(boxWidth).
		Render(content)

	// Place overlay near the top of the screen.
	return placeOverlayTop(width, height, box, base)
}

// placeOverlayTop places the overlay near the top (1/4 down) rather than centered.
func placeOverlayTop(width, height int, overlay, base string) string {
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

	startY := height / 5
	startX := (width - oW) / 2
	if startY < 1 {
		startY = 1
	}
	if startX < 0 {
		startX = 0
	}
	if startY+oH > height {
		startY = max(0, height-oH)
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
