package sbapp

import (
	"fmt"

	"github.com/karlssonsimon/lazyaz/internal/cache"
	"github.com/karlssonsimon/lazyaz/internal/ui"

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
		m.fetchGen++
		m.namespacesSession = cache.NewFetchSession(m.namespaces, m.fetchGen, namespaceKey)
		m.SetLoading(m.focus)
		m.LastErr = ""
		m.Status = fmt.Sprintf("Loading namespaces in %s", ui.SubscriptionDisplayName(m.CurrentSub))
		return m, tea.Batch(spinner.Tick, fetchNamespacesCmd(m.service, m.cache.namespaces, m.CurrentSub.ID, m.fetchGen))
	}

	if m.focus == entitiesPane || !m.hasPeekTarget {
		m.fetchGen++
		m.entitiesSession = cache.NewFetchSession(m.entities, m.fetchGen, entityKey)
		m.SetLoading(m.focus)
		m.LastErr = ""
		m.Status = fmt.Sprintf("Loading entities in %s", m.currentNS.Name)
		entityCacheKey := cache.Key(m.CurrentSub.ID, m.currentNS.Name)
		return m, tea.Batch(spinner.Tick, fetchEntitiesCmd(m.service, m.cache.entities, m.currentNS, entityCacheKey, m.fetchGen))
	}

	return m.rePeekMessages(true)
}

// rePeekMessages re-fetches the current message list. preserveCursor
// should be true when the user is browsing the same scope (after
// requeue/delete-duplicate, R-key refresh) so we keep their position,
// and false when the scope itself just changed (active↔DLQ toggle)
// since the new message IDs won't match the old ones anyway.
func (m Model) rePeekMessages(preserveCursor bool) (Model, tea.Cmd) {
	if !m.hasPeekTarget {
		return m, nil
	}
	m.SetLoading(m.focus)
	m.LastErr = ""
	dlqLabel := "active"
	if m.deadLetter {
		dlqLabel = "DLQ"
	}

	if m.currentSubName == "" {
		m.Status = fmt.Sprintf("Peeking %s messages from queue %s", dlqLabel, m.currentEntity.Name)
		return m, tea.Batch(spinner.Tick, peekQueueMessagesCmd(m.service, m.currentNS, m.currentEntity.Name, m.deadLetter, preserveCursor))
	}

	m.Status = fmt.Sprintf("Peeking %s messages from %s/%s", dlqLabel, m.currentEntity.Name, m.currentSubName)
	return m, tea.Batch(spinner.Tick, peekSubscriptionMessagesCmd(m.service, m.currentNS, m.currentEntity.Name, m.currentSubName, m.deadLetter, preserveCursor))
}
