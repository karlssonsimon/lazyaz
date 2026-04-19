package dashapp

import (
	"fmt"

	"github.com/karlssonsimon/lazyaz/internal/azure/servicebus"

	"charm.land/lipgloss/v2"
)

// namespaceCountsWidget renders a row per Service Bus namespace with
// queue / topic counts and aggregate active + DLQ message totals.
// Topics' messages are summed from their per-subscription counts.
type namespaceCountsWidget struct{}

func (namespaceCountsWidget) Title() string         { return "Namespace counts" }
func (namespaceCountsWidget) Position() (int, int)  { return 0, 0 }
func (namespaceCountsWidget) RowCount(m *Model) int { return len(m.namespaces) }

func (w namespaceCountsWidget) Render(m *Model, width, innerHeight, offset int) string {
	if !m.HasSubscription {
		return "Pick a subscription with " + m.Keymap.SubscriptionPicker.Short() + "."
	}
	if len(m.namespaces) == 0 {
		return m.loadingOrEmpty("No Service Bus namespaces in this subscription.")
	}

	cells := [][]string{{"Namespace", "Queues", "Topics", "Active", "DLQ"}}
	for _, ns := range m.namespaces {
		ents, have := m.entitiesByNS[ns.Name]
		if !have {
			cells = append(cells, []string{ns.Name, "…", "…", "…", "…"})
			continue
		}
		var queues, topics, active, dlq int64
		for _, e := range ents {
			switch e.Kind {
			case servicebus.EntityQueue:
				queues++
				active += e.ActiveMsgCount
				dlq += e.DeadLetterCount
			case servicebus.EntityTopic:
				topics++
				key := ns.Name + "/" + e.Name
				for _, s := range m.topicSubsByKey[key] {
					active += s.ActiveMsgCount
					dlq += s.DeadLetterCount
				}
			}
		}
		cells = append(cells, []string{
			ns.Name,
			fmt.Sprintf("%d", queues),
			fmt.Sprintf("%d", topics),
			fmt.Sprintf("%d", active),
			fmt.Sprintf("%d", dlq),
		})
	}

	aligns := []lipgloss.Position{lipgloss.Left, lipgloss.Right, lipgloss.Right, lipgloss.Right, lipgloss.Right}
	return renderScrollableTable(cells, aligns, m.Styles, offset, innerHeightToVisibleData(innerHeight))
}
