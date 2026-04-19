package dashapp

import (
	"fmt"
	"sort"

	"github.com/karlssonsimon/lazyaz/internal/azure/servicebus"

	"charm.land/lipgloss/v2"
)

// dlqAlertsWidget lists every queue or topic-subscription with at least
// one dead-lettered message, sorted descending by count.
type dlqAlertsWidget struct{}

func (dlqAlertsWidget) Title() string        { return "DLQ alerts" }
func (dlqAlertsWidget) Position() (int, int) { return 1, 0 }

func (dlqAlertsWidget) RowCount(m *Model) int { return len(m.dlqAlerts()) }

func (w dlqAlertsWidget) Render(m *Model, width, innerHeight, offset int) string {
	if !m.HasSubscription {
		return "Pick a subscription with " + m.Keymap.SubscriptionPicker.Short() + "."
	}
	alerts := m.dlqAlerts()
	if len(alerts) == 0 {
		return m.loadingOrEmpty("No dead-lettered messages.")
	}

	cells := [][]string{{"Namespace", "Entity", "DLQ"}}
	for _, a := range alerts {
		cells = append(cells, []string{a.namespace, a.entity, fmt.Sprintf("%d", a.count)})
	}
	aligns := []lipgloss.Position{lipgloss.Left, lipgloss.Left, lipgloss.Right}
	return renderScrollableTable(cells, aligns, m.Styles, offset, innerHeightToVisibleData(innerHeight))
}

// dlqAlert / dlqAlerts moved here from view.go since they're now only
// needed by the DLQ widget and the scroll math (which calls dlqAlerts
// indirectly via a Widget's RowCount).
type dlqAlert struct {
	namespace string
	entity    string
	count     int64
}

// dlqAlerts builds the sorted alert list. Shared between widget render
// and scroll math.
func (m Model) dlqAlerts() []dlqAlert {
	var alerts []dlqAlert
	for _, ns := range m.namespaces {
		for _, e := range m.entitiesByNS[ns.Name] {
			switch e.Kind {
			case servicebus.EntityQueue:
				if e.DeadLetterCount > 0 {
					alerts = append(alerts, dlqAlert{ns.Name, e.Name, e.DeadLetterCount})
				}
			case servicebus.EntityTopic:
				key := ns.Name + "/" + e.Name
				for _, s := range m.topicSubsByKey[key] {
					if s.DeadLetterCount > 0 {
						alerts = append(alerts, dlqAlert{ns.Name, e.Name + "/" + s.Name, s.DeadLetterCount})
					}
				}
			}
		}
	}
	sort.Slice(alerts, func(i, j int) bool { return alerts[i].count > alerts[j].count })
	return alerts
}
