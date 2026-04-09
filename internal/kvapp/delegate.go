package kvapp

import (
	"io"

	"github.com/karlssonsimon/lazyaz/internal/ui"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

type secretDelegate struct {
	base      list.DefaultDelegate
	markedBar string
	visualBar string
}

func newSecretDelegate(base list.DefaultDelegate, styles ui.Styles) secretDelegate {
	bar := "┃ "
	markedBar := lipgloss.NewStyle().
		Foreground(styles.Accent2.GetForeground()).
		Render(bar)
	visualBar := lipgloss.NewStyle().
		Foreground(styles.Warning.GetForeground()).
		Render(bar)
	return secretDelegate{
		base:      base,
		markedBar: markedBar,
		visualBar: visualBar,
	}
}

func (d secretDelegate) Height() int  { return d.base.Height() }
func (d secretDelegate) Spacing() int { return d.base.Spacing() }
func (d secretDelegate) Update(msg tea.Msg, m *list.Model) tea.Cmd {
	return d.base.Update(msg, m)
}

func (d secretDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	if s, ok := item.(secretItem); ok {
		var prefix string
		switch {
		case s.marked:
			prefix = d.markedBar
		case s.visual:
			prefix = d.visualBar
		}
		if prefix != "" {
			item = prefixedItem{inner: s, prefix: prefix}
		}
	}
	d.base.Render(w, m, index, item)
}

type prefixedItem struct {
	inner  list.Item
	prefix string
}

func (p prefixedItem) Title() string {
	if t, ok := p.inner.(list.DefaultItem); ok {
		return p.prefix + t.Title()
	}
	return p.prefix
}

func (p prefixedItem) Description() string {
	if d, ok := p.inner.(list.DefaultItem); ok {
		return d.Description()
	}
	return ""
}

func (p prefixedItem) FilterValue() string {
	return p.inner.FilterValue()
}
