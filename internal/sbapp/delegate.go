package sbapp

import (
	"fmt"
	"io"
	"strings"

	"github.com/karlssonsimon/lazyaz/internal/ui"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// messageMarkKey extracts the mark/visual lookup key for ui.MarkDelegate
// from a messages-list item.
func messageMarkKey(item list.Item) (string, bool) {
	mi, ok := item.(messageItem)
	if !ok {
		return "", false
	}
	return messageOperationKey(mi.message), true
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

const (
	// countsGap is the minimum space between the widest title in the
	// list and the active/DLQ counts column.
	countsGap = 4

	// minTitleWidth is the least a cropped name may keep. Below it the
	// pane is too narrow for both, and the counts are dropped instead
	// of leaving an unreadable name.
	minTitleWidth = 12
)

// renderRowWithCounts delegates to base.Render, then aligns a
// "active / dlq" suffix to a column just past the widest title in
// the visible list. When the names are too long for that, they are
// cropped with an ellipsis so the counts keep a column at the right
// edge — a queue's backlog matters more than the tail of its name.
// Only a pane too narrow for even a cropped name drops the counts.
func renderRowWithCounts(w io.Writer, base list.DefaultDelegate, m list.Model, index int, item list.Item, active, dead int64, cs countStyles) {
	rowWidth := m.Width()
	counts := fmt.Sprintf("%d / %d", active, dead)
	countsW := lipgloss.Width(counts)
	colW := max(countsColumnWidth(m), countsW)

	targetCol := titleColumnWidth(m) + countsGap
	if rowWidth > 0 && targetCol+colW > rowWidth {
		avail := rowWidth - countsGap - colW
		if avail < minTitleWidth {
			base.Render(w, m, index, item)
			return
		}
		// m is a copy; narrowing it makes base.Render truncate the
		// title to the column left of the counts.
		m.SetWidth(avail)
		targetCol = rowWidth - colW
	}

	var buf strings.Builder
	base.Render(&buf, m, index, item)
	rendered := buf.String()
	renderedW := lipgloss.Width(rendered)

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

// countsColumnWidth is the width of the widest "active / dlq" text
// among the visible items, so a cropped list still aligns its counts
// in one column.
func countsColumnWidth(m list.Model) int {
	widest := 0
	for _, it := range m.VisibleItems() {
		var active, dead int64
		switch v := it.(type) {
		case entityItem:
			active, dead = v.entity.ActiveMsgCount, v.entity.DeadLetterCount
		case subscriptionItem:
			active, dead = v.sub.ActiveMsgCount, v.sub.DeadLetterCount
		default:
			continue
		}
		if w := lipgloss.Width(fmt.Sprintf("%d / %d", active, dead)); w > widest {
			widest = w
		}
	}
	return widest
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
