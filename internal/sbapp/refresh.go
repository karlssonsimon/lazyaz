package sbapp

import (
	"fmt"

	"azure-storage/internal/azure/servicebus"
	"azure-storage/internal/cache"
	"azure-storage/internal/ui"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
)

func (m Model) refresh() (Model, tea.Cmd) {
	if !m.hasSubscription {
		// Can't refresh anything without a subscription; open the picker instead.
		m.subOverlay.Open()
		m.setLoading(-1)
		m.lastErr = ""
		m.status = "Refreshing subscriptions..."
		return m, tea.Batch(spinner.Tick, fetchSubscriptionsCmd(m.service, m.cache.subscriptions, true))
	}

	if !m.hasNamespace || m.focus == namespacesPane {
		m.setLoading(m.focus)
		m.lastErr = ""
		m.status = fmt.Sprintf("Loading namespaces in %s", ui.SubscriptionDisplayName(m.currentSub))
		return m, tea.Batch(spinner.Tick, fetchNamespacesCmd(m.service, m.cache.namespaces, m.currentSub.ID))
	}

	if m.focus == entitiesPane || !m.hasEntity {
		m.setLoading(m.focus)
		m.lastErr = ""
		m.status = fmt.Sprintf("Loading entities in %s", m.currentNS.Name)
		entityKey := cache.Key(m.currentSub.ID, m.currentNS.Name)
		return m, tea.Batch(spinner.Tick, fetchEntitiesCmd(m.service, m.cache.entities, m.currentNS, entityKey))
	}

	return m.refreshDetail()
}

func (m Model) refreshDetail() (Model, tea.Cmd) {
	if m.currentEntity.Kind == servicebus.EntityQueue {
		m.setLoading(m.focus)
		m.lastErr = ""
		m.status = fmt.Sprintf("Peeking messages from queue %s", m.currentEntity.Name)
		return m, tea.Batch(spinner.Tick, peekQueueMessagesCmd(m.service, m.currentNS, m.currentEntity.Name, m.deadLetter))
	}

	if m.viewingTopicSub {
		m.setLoading(m.focus)
		m.lastErr = ""
		m.status = fmt.Sprintf("Peeking messages from %s/%s", m.currentEntity.Name, m.currentTopicSub.Name)
		return m, tea.Batch(spinner.Tick, peekSubscriptionMessagesCmd(m.service, m.currentNS, m.currentEntity.Name, m.currentTopicSub.Name, m.deadLetter))
	}

	m.setLoading(m.focus)
	m.lastErr = ""
	m.status = fmt.Sprintf("Loading subscriptions for topic %s", m.currentEntity.Name)
	topicKey := cache.Key(m.currentSub.ID, m.currentNS.Name, m.currentEntity.Name)
	return m, tea.Batch(spinner.Tick, fetchTopicSubscriptionsCmd(m.service, m.cache.topicSubs, m.currentNS, m.currentEntity.Name, topicKey))
}

func (m Model) rePeekMessages() (Model, tea.Cmd) {
	m.setLoading(m.focus)
	m.lastErr = ""
	dlqLabel := "active"
	if m.deadLetter {
		dlqLabel = "DLQ"
	}

	if m.currentEntity.Kind == servicebus.EntityQueue {
		m.status = fmt.Sprintf("Peeking %s messages from queue %s", dlqLabel, m.currentEntity.Name)
		return m, tea.Batch(spinner.Tick, peekQueueMessagesCmd(m.service, m.currentNS, m.currentEntity.Name, m.deadLetter))
	}

	if m.viewingTopicSub {
		m.status = fmt.Sprintf("Peeking %s messages from %s/%s", dlqLabel, m.currentEntity.Name, m.currentTopicSub.Name)
		return m, tea.Batch(spinner.Tick, peekSubscriptionMessagesCmd(m.service, m.currentNS, m.currentEntity.Name, m.currentTopicSub.Name, m.deadLetter))
	}

	return m, nil
}
