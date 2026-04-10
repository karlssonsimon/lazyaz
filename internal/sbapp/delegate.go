package sbapp

import (
	"fmt"
	"io"
	"strings"

	"github.com/karlssonsimon/lazyaz/internal/ui"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

type messageDelegate struct {
	base      list.DefaultDelegate
	markedBar string
	visualBar string
	marked    map[string]struct{}
	visual    map[string]struct{}
}

func newMessageDelegate(base list.DefaultDelegate, styles ui.Styles) messageDelegate {
	bar := "┃ "
	markedBar := lipgloss.NewStyle().
		Foreground(styles.Accent2.GetForeground()).
		Render(bar)
	visualBar := lipgloss.NewStyle().
		Foreground(styles.Warning.GetForeground()).
		Render(bar)
	return messageDelegate{
		base:      base,
		markedBar: markedBar,
		visualBar: visualBar,
	}
}

func (d messageDelegate) Height() int  { return d.base.Height() }
func (d messageDelegate) Spacing() int { return d.base.Spacing() }
func (d messageDelegate) Update(msg tea.Msg, m *list.Model) tea.Cmd {
	return d.base.Update(msg, m)
}

func (d messageDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	if mi, ok := item.(messageItem); ok {
		var prefix string
		if _, isMarked := d.marked[mi.message.MessageID]; isMarked {
			prefix = d.markedBar
		} else if _, isVisual := d.visual[mi.message.MessageID]; isVisual {
			prefix = d.visualBar
		}
		if prefix != "" {
			var buf strings.Builder
			d.base.Render(&buf, m, index, item)
			trimmed := ansi.TruncateLeft(buf.String(), 2, "")
			fmt.Fprint(w, prefix+trimmed)
			return
		}
	}
	d.base.Render(w, m, index, item)
}
