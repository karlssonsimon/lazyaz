package blobapp

import (
	"fmt"
	"io"
	"strings"

	"github.com/karlssonsimon/lazyaz/internal/azure/blob"
	"github.com/karlssonsimon/lazyaz/internal/ui"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// blobDelegate wraps the default delegate and prepends a colored bar
// character to marked/visual blob items. Mark and visual state is
// looked up by blob name at render time so the item list doesn't need
// to be rebuilt when the selection changes. The bar replaces the
// leading padding of the rendered output so filter match underlines
// stay correctly aligned.
type blobDelegate struct {
	base      list.DefaultDelegate
	markedBar string
	visualBar string
	marked    map[string]blob.BlobEntry
	visual    map[string]struct{}
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
		if _, isMarked := d.marked[b.blob.Name]; isMarked {
			prefix = d.markedBar
		} else if _, isVisual := d.visual[b.blob.Name]; isVisual {
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
