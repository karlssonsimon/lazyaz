package sbapp

import (
	"fmt"

	"azure-storage/internal/servicebus"
	commonui "azure-storage/internal/ui"

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
			m.status = "Back to topic subscriptions"
			return m, nil
		}
		m.focus = entitiesPane
		m.status = "Focus: entities"
		return m, nil
	case entitiesPane:
		m.focus = namespacesPane
		m.status = "Focus: namespaces"
		return m, nil
	case namespacesPane:
		m.focus = subscriptionsPane
		m.status = "Focus: subscriptions"
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
			m.status = "Back to topic subscriptions"
			return m, nil
		}
		m.focus = entitiesPane
		m.status = "Focus: entities"
	}
	return m, nil
}

func (m Model) handleEnter() (Model, tea.Cmd) {
	if m.focus == subscriptionsPane {
		item, ok := m.subscriptionsList.SelectedItem().(subscriptionItem)
		if !ok {
			return m, nil
		}

		m.currentSub = item.subscription
		m.hasSubscription = true
		m.hasNamespace = false
		m.hasEntity = false
		m.currentNS = servicebus.Namespace{}
		m.currentEntity = servicebus.Entity{}
		m.clearDetailState()
		m.focus = namespacesPane

		m.namespaces = nil
		m.entities = nil
		m.namespacesList.ResetFilter()
		m.entitiesList.ResetFilter()
		m.detailList.ResetFilter()
		m.namespacesList.SetItems(nil)
		m.entitiesList.SetItems(nil)
		m.detailList.SetItems(nil)
		m.namespacesList.Title = "Namespaces"
		m.entitiesList.Title = "Entities"
		m.detailList.Title = "Detail"

		m.loading = true
		m.status = fmt.Sprintf("Loading namespaces in %s", subscriptionDisplayName(item.subscription))
		return m, tea.Batch(spinner.Tick, loadNamespacesCmd(m.service, item.subscription.ID))
	}

	if m.focus == namespacesPane {
		item, ok := m.namespacesList.SelectedItem().(namespaceItem)
		if !ok {
			return m, nil
		}

		m.currentNS = item.namespace
		m.hasNamespace = true
		m.hasEntity = false
		m.currentEntity = servicebus.Entity{}
		m.clearDetailState()
		m.focus = entitiesPane

		m.entities = nil
		m.entitiesList.ResetFilter()
		m.detailList.ResetFilter()
		m.entitiesList.SetItems(nil)
		m.detailList.SetItems(nil)
		m.entitiesList.Title = "Entities"
		m.detailList.Title = "Detail"

		m.loading = true
		m.status = fmt.Sprintf("Loading entities in %s", item.namespace.Name)
		return m, tea.Batch(spinner.Tick, loadEntitiesCmd(m.service, item.namespace))
	}

	if m.focus == entitiesPane {
		item, ok := m.entitiesList.SelectedItem().(entityItem)
		if !ok {
			return m, nil
		}

		m.currentEntity = item.entity
		m.hasEntity = true
		m.clearDetailState()
		m.focus = detailPane

		m.detailList.ResetFilter()
		m.detailList.SetItems(nil)
		m.detailList.Title = "Detail"

		if item.entity.Kind == servicebus.EntityQueue {
			m.loading = true
			m.status = fmt.Sprintf("Peeking messages from queue %s", item.entity.Name)
			return m, tea.Batch(spinner.Tick, peekQueueMessagesCmd(m.service, m.currentNS, item.entity.Name, m.deadLetter))
		}

		m.loading = true
		m.status = fmt.Sprintf("Loading subscriptions for topic %s", item.entity.Name)
		return m, tea.Batch(spinner.Tick, loadTopicSubscriptionsCmd(m.service, m.currentNS, item.entity.Name))
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

			m.loading = true
			m.status = fmt.Sprintf("Peeking messages from %s/%s", m.currentEntity.Name, item.sub.Name)
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
			m.messageViewport.SetContent(m.syntaxStyles.HighlightJSON(item.message.FullBody))
			m.messageViewport.GotoTop()
			m.status = fmt.Sprintf("Viewing message %s (Esc/h to go back)", commonui.EmptyToDash(item.message.MessageID))
			return m, nil
		}
	}

	return m, nil
}
