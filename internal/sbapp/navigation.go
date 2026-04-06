package sbapp

import (
	"fmt"

	"azure-storage/internal/azure"
	"azure-storage/internal/cache"
	"azure-storage/internal/azure/servicebus"
	"azure-storage/internal/ui"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
)

func (m Model) navigateLeft() (Model, tea.Cmd) {
	switch m.focus {
	case detailPane:
		if m.viewingTopicSub {
			m.viewingTopicSub = false
			m.currentTopicSub = servicebus.TopicSubscription{}
			m.peekedMessages = nil
			m.detailMode = detailTopicSubscriptions
			m.detailList.ResetFilter()
			m.detailList.SetItems(topicSubsToItems(m.topicSubs))
			m.detailList.Title = fmt.Sprintf("Topic Subscriptions (%d)", len(m.topicSubs))
			m.Status = "Back to topic subscriptions"
			return m, nil
		}
		m.focus = entitiesPane
		return m, nil
	case entitiesPane:
		m.focus = namespacesPane
		return m, nil
	case namespacesPane:
		return m, nil
	default:
		return m, nil
	}
}

func (m Model) handleBackspace() (Model, tea.Cmd) {
	if m.focus == detailPane {
		if m.viewingTopicSub {
			m.viewingTopicSub = false
			m.currentTopicSub = servicebus.TopicSubscription{}
			m.peekedMessages = nil
			m.detailMode = detailTopicSubscriptions
			m.detailList.ResetFilter()
			m.detailList.SetItems(topicSubsToItems(m.topicSubs))
			m.detailList.Title = fmt.Sprintf("Topic Subscriptions (%d)", len(m.topicSubs))
			m.Status = "Back to topic subscriptions"
			return m, nil
		}
		m.focus = entitiesPane
	}
	return m, nil
}

func (m Model) selectSubscription(sub azure.Subscription) (Model, tea.Cmd) {
	// Re-selecting the same subscription: no-op.
	if m.HasSubscription && m.CurrentSub.ID == sub.ID {
		return m, nil
	}

	m.CurrentSub = sub
	m.HasSubscription = true
	m.hasNamespace = false
	m.hasEntity = false
	m.currentNS = servicebus.Namespace{}
	m.currentEntity = servicebus.Entity{}
	m.clearDetailState()
	m.focus = namespacesPane

	if cached, ok := m.cache.namespaces.Get(sub.ID); ok {
		m.namespaces = cached
		m.namespacesList.ResetFilter()
		ui.SetItemsPreserveIndex(&m.namespacesList, namespacesToItems(cached))
		m.namespacesList.Title = fmt.Sprintf("Namespaces (%d)", len(cached))
	} else {
		m.namespaces = nil
		m.namespacesList.ResetFilter()
		m.namespacesList.SetItems(nil)
		m.namespacesList.Title = "Namespaces"
	}

	m.entities = nil
	m.entitiesList.ResetFilter()
	m.detailList.ResetFilter()
	m.entitiesList.SetItems(nil)
	m.detailList.SetItems(nil)
	m.entitiesList.Title = "Entities"
	m.detailList.Title = "Detail"

	m.fetchGen++
	m.namespacesSession = cache.NewFetchSession(m.namespaces, m.fetchGen, namespaceKey)
	m.SetLoading(m.focus)
	m.Status = fmt.Sprintf("Loading namespaces in %s", ui.SubscriptionDisplayName(sub))
	return m, tea.Batch(spinner.Tick, fetchNamespacesCmd(m.service, m.cache.namespaces, sub.ID, m.fetchGen))
}

func (m Model) handleEnter() (Model, tea.Cmd) {
	if m.focus == namespacesPane {
		item, ok := m.namespacesList.SelectedItem().(namespaceItem)
		if !ok {
			return m, nil
		}

		// Re-selecting the same namespace: just move focus.
		if m.hasNamespace && m.currentNS.Name == item.namespace.Name {
			m.focus = entitiesPane
			return m, nil
		}

		m.currentNS = item.namespace
		m.hasNamespace = true
		m.hasEntity = false
		m.currentEntity = servicebus.Entity{}
		m.clearDetailState()
		m.focus = entitiesPane

		entityCacheKey := cache.Key(m.CurrentSub.ID, item.namespace.Name)
		if cached, ok := m.cache.entities.Get(entityCacheKey); ok {
			m.entities = cached
			m.entitiesList.ResetFilter()
			ui.SetItemsPreserveIndex(&m.entitiesList, entitiesToFilteredItems(cached, m.dlqFilter))
			m.entitiesList.Title = m.entitiesPaneTitle()
		} else {
			m.entities = nil
			m.entitiesList.ResetFilter()
			m.entitiesList.SetItems(nil)
			m.entitiesList.Title = "Entities"
		}

		m.detailList.ResetFilter()
		m.detailList.SetItems(nil)
		m.detailList.Title = "Detail"

		m.fetchGen++
		m.entitiesSession = cache.NewFetchSession(m.entities, m.fetchGen, entityKey)
		m.SetLoading(m.focus)
		m.Status = fmt.Sprintf("Loading entities in %s", item.namespace.Name)
		return m, tea.Batch(spinner.Tick, fetchEntitiesCmd(m.service, m.cache.entities, item.namespace, entityCacheKey, m.fetchGen))
	}

	if m.focus == entitiesPane {
		item, ok := m.entitiesList.SelectedItem().(entityItem)
		if !ok {
			return m, nil
		}

		// Re-selecting the same entity: just move focus.
		if m.hasEntity && m.currentEntity.Name == item.entity.Name && m.currentEntity.Kind == item.entity.Kind {
			m.focus = detailPane
			return m, nil
		}

		m.currentEntity = item.entity
		m.hasEntity = true
		m.clearDetailState()
		m.focus = detailPane

		if item.entity.Kind == servicebus.EntityTopic {
			topicCacheKey := cache.Key(m.CurrentSub.ID, m.currentNS.Name, item.entity.Name)
			if cached, ok := m.cache.topicSubs.Get(topicCacheKey); ok {
				m.topicSubs = cached
				m.detailMode = detailTopicSubscriptions
				m.detailList.ResetFilter()
				ui.SetItemsPreserveIndex(&m.detailList, topicSubsToItems(cached))
				m.detailList.Title = fmt.Sprintf("Topic Subscriptions (%d)", len(cached))
			} else {
				m.detailList.ResetFilter()
				m.detailList.SetItems(nil)
				m.detailList.Title = "Detail"
			}

			m.fetchGen++
			m.topicSubsSession = cache.NewFetchSession(m.topicSubs, m.fetchGen, topicSubKey)
			m.SetLoading(m.focus)
			m.Status = fmt.Sprintf("Loading subscriptions for topic %s", item.entity.Name)
			return m, tea.Batch(spinner.Tick, fetchTopicSubscriptionsCmd(m.service, m.cache.topicSubs, m.currentNS, item.entity.Name, topicCacheKey, m.fetchGen))
		}

		// Queue — messages are not cached (ephemeral)
		m.detailList.ResetFilter()
		m.detailList.SetItems(nil)
		m.detailList.Title = "Detail"

		m.SetLoading(m.focus)
		m.Status = fmt.Sprintf("Peeking messages from queue %s", item.entity.Name)
		return m, tea.Batch(spinner.Tick, peekQueueMessagesCmd(m.service, m.currentNS, item.entity.Name, m.deadLetter))
	}

	if m.focus == detailPane {
		if m.detailMode == detailTopicSubscriptions && !m.viewingTopicSub {
			item, ok := m.detailList.SelectedItem().(topicSubItem)
			if !ok {
				return m, nil
			}

			m.currentTopicSub = item.sub
			m.viewingTopicSub = true
			m.peekedMessages = nil
			m.detailList.ResetFilter()
			m.detailList.SetItems(nil)

			m.SetLoading(m.focus)
			m.Status = fmt.Sprintf("Peeking messages from %s/%s", m.currentEntity.Name, item.sub.Name)
			return m, tea.Batch(spinner.Tick, peekSubscriptionMessagesCmd(m.service, m.currentNS, m.currentEntity.Name, item.sub.Name, m.deadLetter))
		}

		if m.detailMode == detailMessages {
			item, ok := m.detailList.SelectedItem().(messageItem)
			if !ok {
				return m, nil
			}
			m.selectedMessage = item.message
			m.viewingMessage = true
			m.resize()
			m.messageViewport.SetContent(m.Styles.Syntax.HighlightJSON(item.message.FullBody))
			m.messageViewport.GotoTop()
			m.Status = fmt.Sprintf("Viewing message %s (Esc/h to go back)", ui.EmptyToDash(item.message.MessageID))
			return m, nil
		}
	}

	return m, nil
}
