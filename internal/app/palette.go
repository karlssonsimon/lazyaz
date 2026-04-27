package app

import (
	"github.com/karlssonsimon/lazyaz/internal/fuzzy"
	"github.com/karlssonsimon/lazyaz/internal/keymap"
	"github.com/karlssonsimon/lazyaz/internal/ui"

	tea "charm.land/bubbletea/v2"
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

func (p *commandPalette) handleKey(key string, km keymap.Keymap) (cmd command, executed bool, closed bool) {
	switch {
	case km.Cancel.Matches(key), km.CommandPalette.Matches(key):
		if p.query != "" && !km.CommandPalette.Matches(key) {
			p.query = ""
			p.refilter()
			return command{}, false, false
		}
		p.close()
		return command{}, false, true
	case km.ThemeUp.Matches(key):
		if p.cursor > 0 {
			p.cursor--
		}
	case km.ThemeDown.Matches(key):
		if p.cursor < len(p.filtered)-1 {
			p.cursor++
		}
	case km.OpenFocused.Matches(key):
		if c, ok := p.selectedCommand(); ok {
			p.close()
			return c, true, false
		}
	case km.BackspaceUp.Matches(key):
		if len(p.query) > 0 {
			p.query = p.query[:len(p.query)-1]
			p.refilter()
		}
	case key == "ctrl+v":
		if text := ui.ReadClipboard(); text != "" {
			p.query += text
			p.refilter()
		}
	default:
		if len(key) == 1 && key[0] >= 32 && key[0] < 127 {
			p.query += key
			p.refilter()
		}
	}
	return command{}, false, false
}

func renderCommandPalette(p *commandPalette, closeHint, cursorView string, styles ui.Styles, km *keymap.Keymap, width, height int, base string) string {
	items := make([]ui.OverlayItem, len(p.filtered))
	for ci, idx := range p.filtered {
		items[ci] = ui.OverlayItem{
			Label: p.commands[idx].name,
			Hint:  p.commands[idx].hint,
		}
	}

	return ui.RenderOverlayList(ui.OverlayListConfig{Title: "Commands", Query: p.query, CursorView: cursorView, CloseHint: closeHint, Keymap: km}, items, p.cursor, styles, width, height, base)
}
