package blobapp

import (
	"io"

	"azure-storage/internal/ui"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// blobDelegate wraps the default delegate and prepends a colored bar
// character to marked/visual blob items. The bar is rendered inline
// inside the title so it remains visible even when the selection
// border covers the leftmost column.
type blobDelegate struct {
	base      list.DefaultDelegate
	markedBar string
	visualBar string
}

func newBlobDelegate(base list.DefaultDelegate, styles ui.Styles) blobDelegate {
	bar := "┃ "
	markedBar := lipgloss.NewStyle().
		Foreground(styles.Accent2.GetForeground()).
		Render(bar)
	visualBar := lipgloss.NewStyle().
		Foreground(styles.Warning.GetForeground()).
		Render(bar)
	return blobDelegate{
		base:      base,
		markedBar: markedBar,
		visualBar: visualBar,
	}
}

func (d blobDelegate) Height() int  { return d.base.Height() }
func (d blobDelegate) Spacing() int { return d.base.Spacing() }
func (d blobDelegate) Update(msg tea.Msg, m *list.Model) tea.Cmd {
	return d.base.Update(msg, m)
}

func (d blobDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	if b, ok := item.(blobItem); ok {
		var prefix string
		switch {
		case b.marked:
			prefix = d.markedBar
		case b.visual:
			prefix = d.visualBar
		}
		if prefix != "" {
			item = prefixedItem{inner: b, prefix: prefix}
		}
	}
	d.base.Render(w, m, index, item)
}

// prefixedItem wraps a list item and prepends a rendered prefix to its title.
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
