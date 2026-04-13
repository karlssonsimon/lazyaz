package sbapp

import (
	"fmt"

	"github.com/karlssonsimon/lazyaz/internal/cache"
	"github.com/karlssonsimon/lazyaz/internal/ui"

	tea "charm.land/bubbletea/v2"
)

func (m Model) refresh() (Model, tea.Cmd) {
	if !m.HasSubscription {
		m.SubOverlay.Open()
		m.startLoading(-1, "Refreshing subscriptions...")
		return m, tea.Batch(m.Spinner.Tick, fetchSubscriptionsCmd(m.service, m.cache.subscriptions, m.Subscriptions))
	}

	if !m.hasNamespace || m.focus == namespacesPane {
		m.startLoading(m.focus, fmt.Sprintf("Loading namespaces in %s", ui.SubscriptionDisplayName(m.CurrentSub)))
		return m, tea.Batch(m.Spinner.Tick, fetchNamespacesCmd(m.service, m.cache.namespaces, m.CurrentSub.ID, m.namespaces))
	}

	if m.focus == entitiesPane || !m.hasPeekTarget {
		m.startLoading(m.focus, fmt.Sprintf("Loading entities in %s", m.currentNS.Name))
		entityCacheKey := cache.Key(m.CurrentSub.ID, m.currentNS.Name)
		return m, tea.Batch(m.Spinner.Tick, fetchEntitiesCmd(m.service, m.cache.entities, m.currentNS, entityCacheKey, m.entities))
	}

	if m.focus == queueTypePane {
		return m, refreshEntitiesCmd(m.service, m.currentNS)
	}

	if m.focus == messagesPane || m.focus == messagePreviewPane {
		if m.lockedMessages != nil {
			// Locked DLQ messages can't be re-peeked; refresh entity counts instead.
			return m, refreshEntitiesCmd(m.service, m.currentNS)
		}
		return m.rePeekMessages(true)
	}

	return m, nil
}

func (m Model) rePeekMessages(preserveCursor bool) (Model, tea.Cmd) {
	if !m.hasPeekTarget {
		return m, nil
	}
	dlqLabel := "active"
	if m.deadLetter {
		dlqLabel = "DLQ"
	}

	if m.currentSubName == "" {
		m.startLoading(m.focus, fmt.Sprintf("Peeking %s messages from queue %s", dlqLabel, m.currentEntity.Name))
		return m, tea.Batch(m.Spinner.Tick, peekQueueMessagesCmd(m.service, m.currentNS, m.currentEntity.Name, m.deadLetter, false, preserveCursor, 0))
	}

	m.startLoading(m.focus, fmt.Sprintf("Peeking %s messages from %s/%s", dlqLabel, m.currentEntity.Name, m.currentSubName))
	return m, tea.Batch(m.Spinner.Tick, peekSubscriptionMessagesCmd(m.service, m.currentNS, m.currentEntity.Name, m.currentSubName, m.deadLetter, false, preserveCursor, 0))
}
