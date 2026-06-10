package ui

import (
	"fmt"
	"io"
	"strings"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// MarkDelegate wraps the default delegate and prepends a colored bar
// character to marked/visual items. Mark and visual state is looked up
// by item key at render time so the item list doesn't need to be
// rebuilt when the selection changes. The bar replaces the leading
// padding of the rendered output so filter match underlines stay
// correctly aligned.
type MarkDelegate struct {
	base      list.DefaultDelegate
	markedBar string
	visualBar string
	keyOf     func(list.Item) (string, bool)

	// Marked and Visual are the key sets consulted at render time.
	// Callers set them after construction and re-set the delegate on
	// the list to trigger a re-render.
	Marked map[string]struct{}
	Visual map[string]struct{}
}

// NewMarkDelegate builds a MarkDelegate around base. keyOf extracts the
// mark/visual lookup key from a list item; it returns ok=false for item
// types the delegate should render unmodified.
func NewMarkDelegate(base list.DefaultDelegate, styles Styles, keyOf func(list.Item) (string, bool)) MarkDelegate {
	bar := "▌ "
	markedBar := lipgloss.NewStyle().
		Foreground(styles.Accent2.GetForeground()).
		Render(bar)
	visualBar := lipgloss.NewStyle().
		Foreground(styles.Warning.GetForeground()).
		Render(bar)
	return MarkDelegate{
		base:      base,
		markedBar: markedBar,
		visualBar: visualBar,
		keyOf:     keyOf,
	}
}

func (d MarkDelegate) Height() int  { return d.base.Height() }
func (d MarkDelegate) Spacing() int { return d.base.Spacing() }
func (d MarkDelegate) Update(msg tea.Msg, m *list.Model) tea.Cmd {
	return d.base.Update(msg, m)
}

func (d MarkDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	if key, ok := d.keyOf(item); ok {
		var prefix string
		if _, isMarked := d.Marked[key]; isMarked {
			prefix = d.markedBar
		} else if _, isVisual := d.Visual[key]; isVisual {
			prefix = d.visualBar
		}
		if prefix != "" {
			// Render the item normally so filter match underlines are
			// applied to the correct characters, then replace the
			// 2-char left padding/border with the colored bar.
			var buf strings.Builder
			d.base.Render(&buf, m, index, item)
			trimmed := ansi.TruncateLeft(buf.String(), 2, "")
			fmt.Fprint(w, prefix+trimmed)
			return
		}
	}
	d.base.Render(w, m, index, item)
}
