package dashapp

import (
	"fmt"
	"sort"
	"strings"

	"github.com/karlssonsimon/lazyaz/internal/azure/servicebus"

	"charm.land/lipgloss/v2"
)

// dlqAlertsWidget lists every queue or topic-subscription with at least
// one dead-lettered message.
type dlqAlertsWidget struct{}

func (dlqAlertsWidget) Title() string        { return "DLQ alerts" }
func (dlqAlertsWidget) Position() (int, int) { return 1, 0 }

func (dlqAlertsWidget) RowCount(m *Model, view widgetViewState) int {
	alerts := m.dlqAlerts()
	if view.filter == "" {
		return len(alerts)
	}
	n := 0
	for _, a := range alerts {
		if a.matchesFilter(view.filter) {
			n++
		}
	}
	return n
}

const (
	dlqSortByCount = iota // default — biggest problems first
	dlqSortByNamespace
	dlqSortByEntity
)

func (dlqAlertsWidget) SortFields() []SortField {
	return []SortField{
		{Label: "DLQ count"},
		{Label: "Namespace"},
		{Label: "Entity"},
	}
}

func (w dlqAlertsWidget) Actions(m *Model, cursorRow int) []Action {
	var actions []Action
	alerts := m.dlqAlerts()
	if cursorRow >= 0 && cursorRow < len(alerts) {
		a := alerts[cursorRow]
		actions = append(actions, Action{
			Label: "Open DLQ in Service Bus tab",
			Key:   "o",
			Cmd:   openSBEntityCmd(m.CurrentSub, a.namespace, a.entityName, a.subName, true),
		})
	}
	actions = append(actions, sortAction())
	return actions
}

func (w dlqAlertsWidget) Render(m *Model, width, innerHeight, offset, cursor int, view widgetViewState) string {
	if !m.HasSubscription {
		return "Pick a subscription with " + m.Keymap.SubscriptionPicker.Short() + "."
	}
	alerts := m.dlqAlerts()
	if view.filter != "" {
		filtered := alerts[:0]
		for _, a := range alerts {
			if a.matchesFilter(view.filter) {
				filtered = append(filtered, a)
			}
		}
		alerts = filtered
	}
	if view.hasSort {
		sortDLQAlerts(alerts, view.sortField, view.sortDesc)
	}
	if len(alerts) == 0 {
		if view.filter != "" {
			return "No matches for filter: " + view.filter
		}
		return m.loadingOrEmpty("No dead-lettered messages.")
	}

	cells := [][]string{{"Namespace", "Entity", "DLQ"}}
	for _, a := range alerts {
		cells = append(cells, []string{a.namespace.Name, a.displayEntity(), fmt.Sprintf("%d", a.count)})
	}
	aligns := []lipgloss.Position{lipgloss.Left, lipgloss.Left, lipgloss.Right}
	return renderScrollableTable(cells, aligns, m.Styles, offset, innerHeightToVisibleData(innerHeight), cursor)
}

// dlqAlert is a single row in the DLQ alerts widget.
type dlqAlert struct {
	namespace  servicebus.Namespace
	entityName string // queue name or topic name
	subName    string // empty for queues
	count      int64
}

// displayEntity formats the "Entity" column — queue name as-is, or
// "topic/sub" for topic subscriptions.
func (a dlqAlert) displayEntity() string {
	if a.subName == "" {
		return a.entityName
	}
	return a.entityName + "/" + a.subName
}

// matchesFilter checks the filter against the namespace name and the
// formatted entity column. Either match keeps the row.
func (a dlqAlert) matchesFilter(filter string) bool {
	return matchesFilter(a.namespace.Name, filter) || matchesFilter(a.displayEntity(), filter)
}

// dlqAlerts builds the unsorted list. Render applies sort/filter on
// top so the sort order isn't baked into the data layer.
func (m Model) dlqAlerts() []dlqAlert {
	var alerts []dlqAlert
	for _, ns := range m.namespaces {
		for _, e := range m.entitiesByNS[ns.Name] {
			switch e.Kind {
			case servicebus.EntityQueue:
				if e.DeadLetterCount > 0 {
					alerts = append(alerts, dlqAlert{ns, e.Name, "", e.DeadLetterCount})
				}
			case servicebus.EntityTopic:
				key := ns.Name + "/" + e.Name
				for _, s := range m.topicSubsByKey[key] {
					if s.DeadLetterCount > 0 {
						alerts = append(alerts, dlqAlert{ns, e.Name, s.Name, s.DeadLetterCount})
					}
				}
			}
		}
	}
	// Default: count descending — keeps the historical "biggest first"
	// behavior for unsorted views.
	sort.SliceStable(alerts, func(i, j int) bool { return alerts[i].count > alerts[j].count })
	return alerts
}

// sortDLQAlerts sorts in place. Direction is baked into the comparator
// for the primary field (rather than wrapping with a swap-args adapter)
// so tiebreakers stay in their canonical order regardless of `desc`.
func sortDLQAlerts(alerts []dlqAlert, field int, desc bool) {
	less := func(i, j int) bool {
		a, b := alerts[i], alerts[j]
		switch field {
		case dlqSortByNamespace:
			an, bn := strings.ToLower(a.namespace.Name), strings.ToLower(b.namespace.Name)
			if an != bn {
				if desc {
					return an > bn
				}
				return an < bn
			}
			return a.count > b.count // stable tiebreak: worst first
		case dlqSortByEntity:
			ad, bd := strings.ToLower(a.displayEntity()), strings.ToLower(b.displayEntity())
			if ad != bd {
				if desc {
					return ad > bd
				}
				return ad < bd
			}
			return a.count > b.count
		}
		// dlqSortByCount (and default).
		if a.count != b.count {
			if desc {
				return a.count > b.count
			}
			return a.count < b.count
		}
		return strings.ToLower(a.entityName) < strings.ToLower(b.entityName)
	}
	sort.SliceStable(alerts, less)
}
