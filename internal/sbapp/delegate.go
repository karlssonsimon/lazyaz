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

// countStyles holds the three styles used to render the active/DLQ
// suffix on entity and subscription rows.
type countStyles struct {
	normal   lipgloss.Style
	danger   lipgloss.Style
	selected lipgloss.Style
}

func newCountStyles(base list.DefaultDelegate, styles ui.Styles) countStyles {
	sel := lipgloss.NewStyle().
		Foreground(base.Styles.SelectedTitle.GetForeground()).
		Background(base.Styles.SelectedTitle.GetBackground()).
		Bold(true)
	return countStyles{
		normal:   lipgloss.NewStyle().Foreground(styles.Muted.GetForeground()),
		danger:   lipgloss.NewStyle().Foreground(styles.Danger.GetForeground()),
		selected: sel,
	}
}

// countsGap is the minimum space between the widest title in the list
// and the active/DLQ counts column.
const countsGap = 4

// renderRowWithCounts delegates to base.Render, then aligns a
// "active / dlq" suffix to a column just past the widest title in
// the visible list. If the row cannot fit the counts, they are
// omitted so the name is never truncated further.
func renderRowWithCounts(w io.Writer, base list.DefaultDelegate, m list.Model, index int, item list.Item, active, dead int64, cs countStyles) {
	var buf strings.Builder
	base.Render(&buf, m, index, item)
	rendered := buf.String()

	rowWidth := m.Width()
	counts := fmt.Sprintf("%d / %d", active, dead)
	countsW := lipgloss.Width(counts)
	renderedW := lipgloss.Width(rendered)

	targetCol := titleColumnWidth(m) + countsGap
	if targetCol < renderedW+1 {
		targetCol = renderedW + 1
	}
	if rowWidth <= 0 || targetCol+countsW > rowWidth {
		fmt.Fprint(w, rendered)
		return
	}

	var style lipgloss.Style
	switch {
	case index == m.Index() && m.FilterState() != list.Filtering:
		style = cs.selected
	case dead > 0:
		style = cs.danger
	default:
		style = cs.normal
	}

	pad := targetCol - renderedW
	fmt.Fprint(w, rendered+strings.Repeat(" ", pad)+style.Render(counts))
}

// titleColumnWidth returns the rendered width of the widest title
// among the list's visible items, so counts can be aligned just past
// the names instead of being pushed to the row's right edge.
func titleColumnWidth(m list.Model) int {
	max := 0
	for _, it := range m.VisibleItems() {
		di, ok := it.(list.DefaultItem)
		if !ok {
			continue
		}
		if w := lipgloss.Width(di.Title()); w > max {
			max = w
		}
	}
	// Base.Render left-pads the title (2 cols for normal rows, 2 cols
	// of border+pad for selected). Account for that so the counts
	// column sits past the visible title text.
	return max + 2
}

type entityDelegate struct {
	base   list.DefaultDelegate
	counts countStyles
}

func newEntityDelegate(base list.DefaultDelegate, styles ui.Styles) entityDelegate {
	return entityDelegate{base: base, counts: newCountStyles(base, styles)}
}

func (d entityDelegate) Height() int  { return d.base.Height() }
func (d entityDelegate) Spacing() int { return d.base.Spacing() }
func (d entityDelegate) Update(msg tea.Msg, m *list.Model) tea.Cmd {
	return d.base.Update(msg, m)
}

func (d entityDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	ei, ok := item.(entityItem)
	if !ok {
		d.base.Render(w, m, index, item)
		return
	}
	renderRowWithCounts(w, d.base, m, index, item, ei.entity.ActiveMsgCount, ei.entity.DeadLetterCount, d.counts)
}

type subscriptionDelegate struct {
	base   list.DefaultDelegate
	counts countStyles
}

func newSubscriptionDelegate(base list.DefaultDelegate, styles ui.Styles) subscriptionDelegate {
	return subscriptionDelegate{base: base, counts: newCountStyles(base, styles)}
}

func (d subscriptionDelegate) Height() int  { return d.base.Height() }
func (d subscriptionDelegate) Spacing() int { return d.base.Spacing() }
func (d subscriptionDelegate) Update(msg tea.Msg, m *list.Model) tea.Cmd {
	return d.base.Update(msg, m)
}

func (d subscriptionDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	si, ok := item.(subscriptionItem)
	if !ok {
		d.base.Render(w, m, index, item)
		return
	}
	renderRowWithCounts(w, d.base, m, index, item, si.sub.ActiveMsgCount, si.sub.DeadLetterCount, d.counts)
}
