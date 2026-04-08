package sbapp

import (
	"fmt"

	"github.com/karlssonsimon/lazyaz/internal/azure/servicebus"

	"charm.land/lipgloss/v2"
)

// entityTabsHeight is the rendered height of the All/Queues/Topics
// tab strip at the top of the entities pane. One line for the tabs
// plus a blank spacer below for breathing room.
const entityTabsHeight = 2

// entityKindCounts returns the number of queues and topics among the
// currently loaded entities. Used to label the tab counts.
func (m Model) entityKindCounts() (queues, topics int) {
	for _, e := range m.entities {
		if e.Kind == servicebus.EntityTopic {
			topics++
		} else {
			queues++
		}
	}
	return queues, topics
}

// renderEntityTabs renders the All/Queues/Topics tab strip at the top
// of the entities pane. The selected tab gets the filled TabBar.Active
// styling; the others use TabBar.Inactive. Padded out to the full pane
// width so the tabs sit on a single visual band.
func (m Model) renderEntityTabs(width int) string {
	if !m.hasNamespace {
		return ""
	}
	queues, topics := m.entityKindCounts()
	all := queues + topics

	tb := m.Styles.TabBar
	active := tb.Active.Copy()
	inactive := tb.Inactive.Copy()

	render := func(label string, selected bool) string {
		if selected {
			return active.Render(label)
		}
		return inactive.Render(label)
	}

	allTab := render(fmt.Sprintf(" All (%d) ", all), m.entityFilter == entityFilterAll)
	queuesTab := render(fmt.Sprintf(" Queues (%d) ", queues), m.entityFilter == entityFilterQueues)
	topicsTab := render(fmt.Sprintf(" Topics (%d) ", topics), m.entityFilter == entityFilterTopics)

	sep := tb.Sep.Render("│")
	bar := allTab + sep + queuesTab + sep + topicsTab

	if w := lipgloss.Width(bar); w < width {
		bar += tb.Bar.Render(spaces(width - w))
	}

	return bar + "\n"
}

// cycleEntityFilter advances the entity filter by one step in the given
// direction (+1 forward, -1 back) through All → Queues → Topics → All.
// Resets the entities list cursor to top so the user always sees the
// first item under the new filter.
func (m *Model) cycleEntityFilter(direction int) {
	const n = 3
	cur := int(m.entityFilter)
	cur = ((cur+direction)%n + n) % n
	m.entityFilter = entityFilterMode(cur)
	m.rebuildEntitiesItems()
	if len(m.entitiesList.VisibleItems()) > 0 {
		m.entitiesList.Select(0)
	}
}
