package sbapp

import (
	"fmt"

	"azure-storage/internal/azure"
	"azure-storage/internal/azure/servicebus"
	"azure-storage/internal/ui"
)

func paneName(pane int) string {
	switch pane {
	case subscriptionsPane:
		return "subscriptions"
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

func subscriptionDisplayName(sub azure.Subscription) string {
	return ui.SubscriptionDisplayName(sub)
}

func entityDisplayName(e servicebus.Entity) string {
	tag := "[Q]"
	if e.Kind == servicebus.EntityTopic {
		tag = "[T]"
	}
	return fmt.Sprintf("%s %s", tag, e.Name)
}

func (m Model) subscriptionsPaneTitle() string {
	title := "Subscriptions"
	if len(m.subscriptions) > 0 {
		title = fmt.Sprintf("Subscriptions (%d)", len(m.subscriptions))
	}
	return title
}

func (m Model) namespacesPaneTitle() string {
	title := "Namespaces"
	if m.hasSubscription {
		title = fmt.Sprintf("Namespaces · %s", subscriptionDisplayName(m.currentSub))
	}
	if len(m.namespaces) > 0 {
		title = fmt.Sprintf("%s (%d)", title, len(m.namespaces))
	}
	return title
}

func (m Model) entitiesPaneTitle() string {
	title := "Entities"
	if m.dlqFilter {
		title = "Entities [DLQ]"
	}
	if m.hasNamespace {
		title = fmt.Sprintf("%s · %s", title, m.currentNS.Name)
	}
	if m.entities != nil {
		filtered := len(entitiesToFilteredItems(m.entities, m.dlqFilter))
		title = fmt.Sprintf("%s (%d)", title, filtered)
	}
	return title
}

func (m Model) detailPaneTitle() string {
	if !m.hasEntity {
		return "Detail"
	}

	queueLabel := "ACTIVE"
	if m.deadLetter {
		queueLabel = "DLQ"
	}

	if m.currentEntity.Kind == servicebus.EntityQueue {
		title := fmt.Sprintf("[%s] %s", queueLabel, m.currentEntity.Name)
		if m.peekedMessages != nil {
			title = fmt.Sprintf("%s (%d)", title, len(m.peekedMessages))
		}
		return title
	}

	if m.viewingTopicSub {
		title := fmt.Sprintf("[%s] %s/%s", queueLabel, m.currentEntity.Name, m.currentTopicSub.Name)
		if m.peekedMessages != nil {
			title = fmt.Sprintf("%s (%d)", title, len(m.peekedMessages))
		}
		return title
	}

	title := fmt.Sprintf("Topic Subs · %s", m.currentEntity.Name)
	if m.topicSubs != nil {
		title = fmt.Sprintf("%s (%d)", title, len(m.topicSubs))
	}
	return title
}

func (m *Model) applyEntityFilter() {
	items := entitiesToFilteredItems(m.entities, m.dlqFilter)
	m.entitiesList.ResetFilter()
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
