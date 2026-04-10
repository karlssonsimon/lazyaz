package kvapp

import (
	"fmt"
	"io"
	"strings"

	"github.com/karlssonsimon/lazyaz/internal/azure/keyvault"
	"github.com/karlssonsimon/lazyaz/internal/ui"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

type secretDelegate struct {
	base      list.DefaultDelegate
	markedBar string
	visualBar string
	marked    map[string]keyvault.Secret
	visual    map[string]struct{}
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
		if _, isMarked := d.marked[s.secret.Name]; isMarked {
			prefix = d.markedBar
		} else if _, isVisual := d.visual[s.secret.Name]; isVisual {
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
