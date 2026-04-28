package app

import (
	"strings"

	"github.com/karlssonsimon/lazyaz/internal/fuzzy"
	"github.com/karlssonsimon/lazyaz/internal/keymap"
	"github.com/karlssonsimon/lazyaz/internal/ui"

	tea "charm.land/bubbletea/v2"
)

// command represents a single entry in the command palette.
type command struct {
	name    string
	hint    string // keyboard shortcut shown on the right
	section string // grouping label shown as a section header
	action  func() commandAction
}

// commandAction is the result of executing a command.
type commandAction struct {
	msg  tea.Msg // if non-nil, injected as a message
	cmd  tea.Cmd // if non-nil, returned as a command
	quit bool    // shortcut for tea.Quit
}

// commandPalette manages the state of the command palette overlay.
//
// Cursor and rendering work on the `visible` list which interleaves
// section headers with selectable commands. cmdIndex is parallel to
// visible: header rows hold -1, item rows hold the index into commands.
type commandPalette struct {
	active   bool
	query    string
	cursor   int
	commands []command
	visible  []ui.OverlayItem
	cmdIndex []int
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
	p.visible = nil
	p.cmdIndex = nil
}

func (p *commandPalette) refilter() {
	matched := fuzzy.Filter(p.query, p.commands, func(c command) string {
		return c.name + " " + c.section
	})

	p.visible = p.visible[:0]
	p.cmdIndex = p.cmdIndex[:0]

	var lastSection string
	for _, idx := range matched {
		c := p.commands[idx]
		if c.section != lastSection {
			p.visible = append(p.visible, ui.OverlayItem{
				Label:    strings.ToUpper(c.section),
				IsHeader: true,
			})
			p.cmdIndex = append(p.cmdIndex, -1)
			lastSection = c.section
		}
		p.visible = append(p.visible, ui.OverlayItem{
			Label: c.name,
			Hint:  c.hint,
		})
		p.cmdIndex = append(p.cmdIndex, idx)
	}

	if p.cursor >= len(p.visible) {
		p.cursor = max(0, len(p.visible)-1)
	}
	if p.cursor < len(p.visible) && p.visible[p.cursor].IsHeader {
		p.moveCursor(1)
		if p.cursor < len(p.visible) && p.visible[p.cursor].IsHeader {
			p.moveCursor(-1)
		}
	}
}

// moveCursor advances by delta, skipping over header rows so the cursor
// always rests on a selectable command.
func (p *commandPalette) moveCursor(delta int) {
	if len(p.visible) == 0 {
		return
	}
	next := p.cursor + delta
	for next >= 0 && next < len(p.visible) {
		if !p.visible[next].IsHeader {
			p.cursor = next
			return
		}
		next += delta
	}
}

func (p *commandPalette) selectedCommand() (command, bool) {
	if p.cursor < 0 || p.cursor >= len(p.cmdIndex) {
		return command{}, false
	}
	idx := p.cmdIndex[p.cursor]
	if idx < 0 {
		return command{}, false
	}
	return p.commands[idx], true
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
		p.moveCursor(-1)
	case km.ThemeDown.Matches(key):
		p.moveCursor(1)
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
	cfg := ui.OverlayListConfig{
		Title:          "Commands",
		Query:          p.query,
		CursorView:     cursorView,
		CloseHint:      closeHint,
		NoActiveMarker: true,
		Bindings: &ui.OverlayBindings{
			MoveUp:   km.ThemeUp,
			MoveDown: km.ThemeDown,
			Apply:    km.OpenFocused,
			Cancel:   km.Cancel,
			Erase:    km.BackspaceUp,
		},
	}
	return ui.RenderOverlayList(cfg, p.visible, p.cursor, styles, width, height, base)
}
