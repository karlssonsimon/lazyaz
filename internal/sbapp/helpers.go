package sbapp

import (
	"fmt"

	"azure-storage/internal/azure/servicebus"
	"azure-storage/internal/ui"
)

func paneName(pane int) string {
	switch pane {
	case namespacesPane:
		return "namespaces"
	case entitiesPane:
		return "entities"
	case detailPane:
		return "detail"
	default:
		return "items"
	}
}

func entityDisplayName(e servicebus.Entity) string {
	glyph := "☰"
	if e.Kind == servicebus.EntityTopic {
		glyph = "▶"
	}
	return fmt.Sprintf("%s %s", glyph, e.Name)
}

func (m Model) namespacesPaneTitle() string {
	title := "Namespaces"
	if m.HasSubscription {
		title = fmt.Sprintf("Namespaces · %s", ui.SubscriptionDisplayName(m.CurrentSub))
	}
	if len(m.namespaces) > 0 {
		title = fmt.Sprintf("%s (%d)", title, len(m.namespaces))
	}
	return title
}

func (m Model) entitiesPaneTitle() string {
	title := "Entities"
	if m.dlqSort {
		title = "Entities [DLQ-first]"
	}
	if m.hasNamespace {
		title = fmt.Sprintf("%s · %s", title, m.currentNS.Name)
	}
	if m.entities != nil {
		// Top-level entity count (queues + topics) — exclude expanded
		// children so the displayed number reflects the underlying
		// resource count, not whatever the user has unfolded.
		title = fmt.Sprintf("%s (%d)", title, len(m.entities))
	}
	return title
}

func (m Model) detailPaneTitle() string {
	if !m.hasPeekTarget {
		return "Detail"
	}

	// The active/DLQ mode is shown by the tab strip immediately below
	// the title — no need to repeat it in the title itself.
	target := m.currentEntity.Name
	if m.currentSubName != "" {
		target = m.currentEntity.Name + "/" + m.currentSubName
	}

	title := target
	if m.peekedMessages != nil {
		title = fmt.Sprintf("%s (%d)", title, len(m.peekedMessages))
	}
	return title
}

func (m *Model) rebuildEntitiesItems() {
	items := entitiesTreeToItems(m.entities, m.topicSubsByTopic, m.expandedTopics, m.entityFilter, m.dlqSort)
	ui.SetItemsPreserveKey(&m.entitiesList, items, entitiesTreeItemKey)
}

// applyDLQSort rebuilds the entities list under the current dlqSort
// flag. Used by the toggle handler. Resets the filter and cursor so the
// reordering is visible from the top.
func (m *Model) applyDLQSort() {
	m.entitiesList.ResetFilter()
	items := entitiesTreeToItems(m.entities, m.topicSubsByTopic, m.expandedTopics, m.entityFilter, m.dlqSort)
	m.entitiesList.SetItems(items)
	if len(items) > 0 {
		m.entitiesList.Select(0)
	}
}

func truncateForStatus(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
