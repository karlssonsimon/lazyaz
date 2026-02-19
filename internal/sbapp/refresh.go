package sbapp

import (
	"fmt"

	"azure-storage/internal/servicebus"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
)

func (m Model) refresh() (Model, tea.Cmd) {
	if m.focus == subscriptionsPane || !m.hasSubscription {
		m.loading = true
		m.lastErr = ""
		m.status = "Refreshing subscriptions..."
		return m, tea.Batch(spinner.Tick, loadSubscriptionsCmd(m.service))
	}

	if !m.hasNamespace || m.focus == namespacesPane {
		m.loading = true
		m.lastErr = ""
		m.status = fmt.Sprintf("Loading namespaces in %s", subscriptionDisplayName(m.currentSub))
		return m, tea.Batch(spinner.Tick, loadNamespacesCmd(m.service, m.currentSub.ID))
	}

	if m.focus == entitiesPane || !m.hasEntity {
		m.loading = true
		m.lastErr = ""
		m.status = fmt.Sprintf("Loading entities in %s", m.currentNS.Name)
		return m, tea.Batch(spinner.Tick, loadEntitiesCmd(m.service, m.currentNS))
	}

	return m.refreshDetail()
}

func (m Model) refreshDetail() (Model, tea.Cmd) {
	if m.currentEntity.Kind == servicebus.EntityQueue {
		m.loading = true
		m.lastErr = ""
		m.status = fmt.Sprintf("Peeking messages from queue %s", m.currentEntity.Name)
		return m, tea.Batch(spinner.Tick, peekQueueMessagesCmd(m.service, m.currentNS, m.currentEntity.Name, m.deadLetter))
	}

	if m.viewingTopicSub {
		m.loading = true
		m.lastErr = ""
		m.status = fmt.Sprintf("Peeking messages from %s/%s", m.currentEntity.Name, m.currentTopicSub.Name)
		return m, tea.Batch(spinner.Tick, peekSubscriptionMessagesCmd(m.service, m.currentNS, m.currentEntity.Name, m.currentTopicSub.Name, m.deadLetter))
	}

	m.loading = true
	m.lastErr = ""
	m.status = fmt.Sprintf("Loading subscriptions for topic %s", m.currentEntity.Name)
	return m, tea.Batch(spinner.Tick, loadTopicSubscriptionsCmd(m.service, m.currentNS, m.currentEntity.Name))
}

func (m Model) rePeekMessages() (Model, tea.Cmd) {
	m.loading = true
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
