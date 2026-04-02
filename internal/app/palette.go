package app

import (
	"azure-storage/internal/fuzzy"
	"azure-storage/internal/ui"

	tea "github.com/charmbracelet/bubbletea"
)

// command represents a single entry in the command palette.
type command struct {
	name   string
	hint   string // keyboard shortcut shown on the right
	action func() commandAction
}

// commandAction is the result of executing a command.
type commandAction struct {
	msg  tea.Msg // if non-nil, injected as a message
	cmd  tea.Cmd // if non-nil, returned as a command
	quit bool    // shortcut for tea.Quit
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
	p.filtered = fuzzy.Filter(p.query, p.commands, func(c command) string { return c.name })
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
	items := make([]ui.OverlayItem, len(p.filtered))
	for ci, idx := range p.filtered {
		items[ci] = ui.OverlayItem{
			Label: p.commands[idx].name,
			Hint:  p.commands[idx].hint,
		}
	}

	return ui.RenderOverlayList(ui.OverlayListConfig{Title: "Commands", Query: p.query}, items, p.cursor, overlay, width, height, base)
}
