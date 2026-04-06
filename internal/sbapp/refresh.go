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
	if !m.HasSubscription {
		// Can't refresh anything without a subscription; open the picker instead.
		m.SubOverlay.Open()
		m.SetLoading(-1)
		m.LastErr = ""
		m.Status = "Refreshing subscriptions..."
		return m, tea.Batch(spinner.Tick, fetchSubscriptionsCmd(m.service, m.cache.subscriptions, true))
	}

	if !m.hasNamespace || m.focus == namespacesPane {
		m.SetLoading(m.focus)
		m.LastErr = ""
		m.Status = fmt.Sprintf("Loading namespaces in %s", ui.SubscriptionDisplayName(m.CurrentSub))
		return m, tea.Batch(spinner.Tick, fetchNamespacesCmd(m.service, m.cache.namespaces, m.CurrentSub.ID))
	}

	if m.focus == entitiesPane || !m.hasEntity {
		m.SetLoading(m.focus)
		m.LastErr = ""
		m.Status = fmt.Sprintf("Loading entities in %s", m.currentNS.Name)
		entityKey := cache.Key(m.CurrentSub.ID, m.currentNS.Name)
		return m, tea.Batch(spinner.Tick, fetchEntitiesCmd(m.service, m.cache.entities, m.currentNS, entityKey))
	}

	return m.refreshDetail()
}

func (m Model) refreshDetail() (Model, tea.Cmd) {
	if m.currentEntity.Kind == servicebus.EntityQueue {
		m.SetLoading(m.focus)
		m.LastErr = ""
		m.Status = fmt.Sprintf("Peeking messages from queue %s", m.currentEntity.Name)
		return m, tea.Batch(spinner.Tick, peekQueueMessagesCmd(m.service, m.currentNS, m.currentEntity.Name, m.deadLetter))
	}

	if m.viewingTopicSub {
		m.SetLoading(m.focus)
		m.LastErr = ""
		m.Status = fmt.Sprintf("Peeking messages from %s/%s", m.currentEntity.Name, m.currentTopicSub.Name)
		return m, tea.Batch(spinner.Tick, peekSubscriptionMessagesCmd(m.service, m.currentNS, m.currentEntity.Name, m.currentTopicSub.Name, m.deadLetter))
	}

	m.SetLoading(m.focus)
	m.LastErr = ""
	m.Status = fmt.Sprintf("Loading subscriptions for topic %s", m.currentEntity.Name)
	topicKey := cache.Key(m.CurrentSub.ID, m.currentNS.Name, m.currentEntity.Name)
	return m, tea.Batch(spinner.Tick, fetchTopicSubscriptionsCmd(m.service, m.cache.topicSubs, m.currentNS, m.currentEntity.Name, topicKey))
}

func (m Model) rePeekMessages() (Model, tea.Cmd) {
	m.SetLoading(m.focus)
	m.LastErr = ""
	dlqLabel := "active"
	if m.deadLetter {
		dlqLabel = "DLQ"
	}

	if m.currentEntity.Kind == servicebus.EntityQueue {
		m.Status = fmt.Sprintf("Peeking %s messages from queue %s", dlqLabel, m.currentEntity.Name)
		return m, tea.Batch(spinner.Tick, peekQueueMessagesCmd(m.service, m.currentNS, m.currentEntity.Name, m.deadLetter))
	}

	if m.viewingTopicSub {
		m.Status = fmt.Sprintf("Peeking %s messages from %s/%s", dlqLabel, m.currentEntity.Name, m.currentTopicSub.Name)
		return m, tea.Batch(spinner.Tick, peekSubscriptionMessagesCmd(m.service, m.currentNS, m.currentEntity.Name, m.currentTopicSub.Name, m.deadLetter))
	}

	return m, nil
}
