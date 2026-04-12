package sbapp

import (
	"fmt"

	"github.com/karlssonsimon/lazyaz/internal/azure/servicebus"
	"github.com/karlssonsimon/lazyaz/internal/ui"
)

// startLoading dismisses any active spinner, marks the pane as loading,
// and pushes a new spinner notification. This prevents orphaned spinners
// when the user navigates away before a load finishes.
func (m *Model) startLoading(pane int, message string) {
	if m.Loading {
		m.ClearLoading()
		m.DismissSpinner(m.loadingSpinnerID)
	}
	m.SetLoading(pane)
	m.loadingSpinnerID = m.NotifySpinner(message)
}

func paneName(pane int) string {
	switch pane {
	case namespacesPane:
		return "namespaces"
	case entitiesPane:
		return "entities"
	case subscriptionsPane:
		return "subscriptions"
	case queueTypePane:
		return "queue type"
	case messagesPane:
		return "messages"
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
	if label := entitySortLabel(m.entitySortField, m.entitySortDesc, m.entityDLQFilter); label != "" {
		title = fmt.Sprintf("Entities [%s]", label)
	}
	if m.hasNamespace {
		title = fmt.Sprintf("%s · %s", title, m.currentNS.Name)
	}
	if m.entities != nil {
		n := len(sortAndFilterEntities(m.entities, m.entitySortField, m.entitySortDesc, m.entityDLQFilter))
		title = fmt.Sprintf("%s (%d)", title, n)
	}
	return title
}

func (m Model) subscriptionsPaneTitle() string {
	title := "Subscriptions"
	if m.currentEntity.Name != "" {
		title = fmt.Sprintf("Subscriptions · %s", m.currentEntity.Name)
	}
	if len(m.subscriptions) > 0 {
		title = fmt.Sprintf("%s (%d)", title, len(m.subscriptions))
	}
	return title
}

func (m Model) queueTypePaneTitle() string {
	if !m.hasPeekTarget {
		return "Queue Type"
	}
	target := m.currentEntity.Name
	if m.currentSubName != "" {
		target = m.currentEntity.Name + "/" + m.currentSubName
	}
	return target
}

func (m Model) messagesPaneTitle() string {
	label := "Messages"
	if m.deadLetter {
		label = "DLQ Messages"
	}
	if m.peekedMessages != nil {
		label = fmt.Sprintf("%s (%d)", label, len(m.peekedMessages))
	}
	return label
}

func (m *Model) rebuildEntitiesItems() {
	items := entitiesToItems(m.entities, m.entitySortField, m.entitySortDesc, m.entityDLQFilter)
	ui.SetItemsPreserveKey(&m.entitiesList, items, entityItemKey)
}

func (m *Model) applyEntitySort() {
	m.entitiesList.ResetFilter()
	items := entitiesToItems(m.entities, m.entitySortField, m.entitySortDesc, m.entityDLQFilter)
	m.entitiesList.SetItems(items)
	m.entitiesList.Title = m.entitiesPaneTitle()
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
