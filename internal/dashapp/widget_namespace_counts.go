package dashapp

import (
	"fmt"
	"sort"
	"strings"

	"github.com/karlssonsimon/lazyaz/internal/azure/servicebus"

	"charm.land/lipgloss/v2"
)

// namespaceCountsWidget renders a row per Service Bus namespace with
// queue / topic counts and aggregate active + DLQ message totals.
// Topics' messages are summed from their per-subscription counts.
type namespaceCountsWidget struct{}

func (namespaceCountsWidget) Title() string        { return "Namespace counts" }
func (namespaceCountsWidget) Position() (int, int) { return 0, 0 }

func (w namespaceCountsWidget) Context(m *Model, view widgetViewState) string {
	total := len(m.namespaces)
	if total == 0 {
		return ""
	}
	visible := w.RowCount(m, view)
	if visible == total {
		return fmt.Sprintf("%s total", formatThousands(int64(total)))
	}
	return fmt.Sprintf("%s visible · %s total", formatThousands(int64(visible)), formatThousands(int64(total)))
}

func (namespaceCountsWidget) RowCount(m *Model, view widgetViewState) int {
	if view.filter == "" {
		return len(m.namespaces)
	}
	n := 0
	for _, ns := range m.namespaces {
		if matchesFilter(ns.Name, view.filter) {
			n++
		}
	}
	return n
}

const (
	nsSortByName = iota
	nsSortByQueues
	nsSortByTopics
	nsSortByActive
	nsSortByDLQ
)

func (namespaceCountsWidget) SortFields() []SortField {
	return []SortField{
		{Label: "Namespace"},
		{Label: "Queue count"},
		{Label: "Topic count"},
		{Label: "Active messages"},
		{Label: "DLQ messages"},
	}
}

func (w namespaceCountsWidget) Actions(m *Model, cursorRow int) []Action {
	var actions []Action
	stats := w.rows(m, focusedWidgetView(m))
	if cursorRow >= 0 && cursorRow < len(stats) {
		ns := stats[cursorRow].namespace
		actions = append(actions, Action{
			Label: "Open in Service Bus tab",
			Key:   "o",
			Cmd:   openSBNamespaceCmd(m.CurrentSub, ns),
		})
	}
	actions = append(actions, sortAction())
	return actions
}

// nsStats is the per-namespace aggregate the widget computes once and
// then sorts. Computing in a single pass means sort comparators don't
// re-walk entitiesByNS for every comparison.
type nsStats struct {
	namespace                   servicebus.Namespace
	queues, topics, active, dlq int64
	entitiesLoaded              bool // false → render placeholder dots
}

func (w namespaceCountsWidget) rows(m *Model, view widgetViewState) []nsStats {
	stats := make([]nsStats, 0, len(m.namespaces))
	for _, ns := range m.namespaces {
		if !matchesFilter(ns.Name, view.filter) {
			continue
		}
		s := nsStats{namespace: ns}
		ents, have := m.entitiesByNS[ns.Name]
		if have {
			s.entitiesLoaded = true
			for _, e := range ents {
				switch e.Kind {
				case servicebus.EntityQueue:
					s.queues++
					s.active += e.ActiveMsgCount
					s.dlq += e.DeadLetterCount
				case servicebus.EntityTopic:
					s.topics++
					key := ns.Name + "/" + e.Name
					for _, sub := range m.topicSubsByKey[key] {
						s.active += sub.ActiveMsgCount
						s.dlq += sub.DeadLetterCount
					}
				}
			}
		}
		stats = append(stats, s)
	}

	if view.hasSort {
		sortNamespaceStats(stats, view.sortField, view.sortDesc)
	}
	return stats
}

func (w namespaceCountsWidget) Render(m *Model, width, innerHeight, offset, cursor int, view widgetViewState) string {
	if !m.HasSubscription {
		return "Pick a subscription with " + m.Keymap.SubscriptionPicker.Short() + "."
	}
	if len(m.namespaces) == 0 {
		return m.loadingOrEmpty("No Service Bus namespaces in this subscription.")
	}

	stats := w.rows(m, view)

	cells := [][]string{{"Namespace", "Queues", "Topics", "Active", "DLQ"}}
	muted := m.Styles.Muted
	for _, s := range stats {
		if !s.entitiesLoaded {
			placeholder := muted.Render("…")
			cells = append(cells, []string{s.namespace.Name, placeholder, placeholder, placeholder, placeholder})
			continue
		}
		cells = append(cells, []string{
			s.namespace.Name,
			countCell(s.queues, m.Styles, severityCounts),
			countCell(s.topics, m.Styles, severityCounts),
			countCell(s.active, m.Styles, severityMessages),
			countCell(s.dlq, m.Styles, severityDLQ),
		})
	}

	aligns := []lipgloss.Position{lipgloss.Left, lipgloss.Right, lipgloss.Right, lipgloss.Right, lipgloss.Right}
	return renderScrollableTable(cells, aligns, m.Styles, offset, innerHeightToVisibleData(innerHeight), cursor)
}

// sortNamespaceStats sorts in place. Direction is baked into the
// primary comparator so the name-asc tiebreaker stays canonical
// regardless of `desc`.
func sortNamespaceStats(stats []nsStats, field int, desc bool) {
	cmpInt := func(x, y int64) bool {
		if desc {
			return x > y
		}
		return x < y
	}
	less := func(i, j int) bool {
		a, b := stats[i], stats[j]
		switch field {
		case nsSortByQueues:
			if a.queues != b.queues {
				return cmpInt(a.queues, b.queues)
			}
		case nsSortByTopics:
			if a.topics != b.topics {
				return cmpInt(a.topics, b.topics)
			}
		case nsSortByActive:
			if a.active != b.active {
				return cmpInt(a.active, b.active)
			}
		case nsSortByDLQ:
			if a.dlq != b.dlq {
				return cmpInt(a.dlq, b.dlq)
			}
		case nsSortByName:
			an, bn := strings.ToLower(a.namespace.Name), strings.ToLower(b.namespace.Name)
			if an != bn {
				if desc {
					return an > bn
				}
				return an < bn
			}
		}
		// Tiebreaker: name asc, never flips with direction.
		return strings.ToLower(a.namespace.Name) < strings.ToLower(b.namespace.Name)
	}
	sort.SliceStable(stats, less)
}
