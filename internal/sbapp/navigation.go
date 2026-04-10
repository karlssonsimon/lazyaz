package sbapp

import (
	"fmt"

	"github.com/karlssonsimon/lazyaz/internal/appshell"
	"github.com/karlssonsimon/lazyaz/internal/azure"
	"github.com/karlssonsimon/lazyaz/internal/azure/servicebus"
	"github.com/karlssonsimon/lazyaz/internal/cache"
	"github.com/karlssonsimon/lazyaz/internal/ui"

	tea "charm.land/bubbletea/v2"
)

func (m Model) navigateLeft() (Model, tea.Cmd) {
	switch m.focus {
	case messagePreviewPane:
		m.setFocus(messagesPane)
		return m, nil
	case messagesPane:
		m.closePreview()
		m.setFocus(queueTypePane)
		return m, nil
	case queueTypePane:
		if m.isTopicSelected() {
			m.setFocus(subscriptionsPane)
		} else {
			m.setFocus(entitiesPane)
		}
		return m, nil
	case subscriptionsPane:
		m.setFocus(entitiesPane)
		return m, nil
	case entitiesPane:
		m.setFocus(namespacesPane)
		return m, nil
	default:
		return m, nil
	}
}

func (m Model) handleBackspace() (Model, tea.Cmd) {
	if m.focus > namespacesPane {
		return m.navigateLeft()
	}
	return m, nil
}

func (m *Model) closePreview() {
	m.viewingMessage = false
	m.selectedMessage = servicebus.PeekedMessage{}
	m.textSelection.Reset()
}

func (m Model) messageViewportRegion() ui.ViewportRegion {
	pane := m.Styles.Chrome.Pane

	previewX := 0
	for i := 0; i < messagePreviewPane; i++ {
		previewX += m.paneWidths[i]
	}

	hFrame := pane.GetHorizontalFrameSize()
	innerX := previewX + hFrame/2

	vFrameTop := pane.GetBorderTopSize() + pane.GetPaddingTop()
	innerY := ui.SubscriptionBarHeight + vFrameTop + ui.PaneTitleHeight + 1

	return ui.ViewportRegion{
		X:      innerX,
		Y:      innerY,
		Width:  m.messageViewport.Width(),
		Height: m.messageViewport.Height(),
	}
}

func (m Model) selectSubscription(sub azure.Subscription) (Model, tea.Cmd) {
	if m.HasSubscription && m.CurrentSub.ID == sub.ID {
		return m, nil
	}

	if m.HasSubscription {
		m.namespacesHistory[m.CurrentSub.ID] = ui.SnapshotListState(&m.namespacesList, namespaceItemKey)
	}

	m.CurrentSub = sub
	m.HasSubscription = true
	m.hasNamespace = false
	m.currentNS = servicebus.Namespace{}
	m.clearPeekState()
	m.clearAllMarks()
	m.subscriptions = nil
	m.setFocus(namespacesPane)

	if cached, ok := m.cache.namespaces.Get(sub.ID); ok {
		m.namespaces = cached
		m.namespacesList.SetItems(namespacesToItems(cached))
		m.namespacesList.Title = fmt.Sprintf("Namespaces (%d)", len(cached))
	} else {
		m.namespaces = nil
		m.namespacesList.SetItems(nil)
		m.namespacesList.Title = "Namespaces"
	}
	ui.RestoreListState(&m.namespacesList, m.namespacesHistory[sub.ID], namespaceItemKey)

	m.entities = nil
	m.entitiesList.ResetFilter()
	m.entitiesList.SetItems(nil)
	m.entitiesList.Title = "Entities"
	m.subscriptionsList.SetItems(nil)
	m.queueTypeList.SetItems(nil)
	m.messageList.SetItems(nil)
	m.resize()

	m.SetLoading(m.focus)
	m.loadingSpinnerID = m.NotifySpinner(fmt.Sprintf("Loading namespaces in %s", ui.SubscriptionDisplayName(sub)))
	return m, tea.Batch(m.Spinner.Tick, fetchNamespacesCmd(m.service, m.cache.namespaces, sub.ID, m.namespaces))
}

func (m Model) handleEnter() (Model, tea.Cmd) {
	if m.focus == namespacesPane {
		item, ok := m.namespacesList.SelectedItem().(namespaceItem)
		if !ok {
			return m, nil
		}

		if m.hasNamespace && m.currentNS.Name == item.namespace.Name {
			m.setFocus(entitiesPane)
			return m, nil
		}

		if m.hasNamespace {
			oldKey := cache.Key(m.CurrentSub.ID, m.currentNS.Name)
			m.entitiesHistory[oldKey] = ui.SnapshotListState(&m.entitiesList, entityItemKey)
		}

		m.currentNS = item.namespace
		m.hasNamespace = true
		m.clearPeekState()
		m.clearAllMarks()
		m.subscriptions = nil
		m.setFocus(entitiesPane)

		entityCacheKey := cache.Key(m.CurrentSub.ID, item.namespace.Name)
		if cached, ok := m.cache.entities.Get(entityCacheKey); ok {
			m.entities = cached
			m.entitiesList.SetItems(entitiesToItems(cached, m.dlqSort))
			m.entitiesList.Title = m.entitiesPaneTitle()
		} else {
			m.entities = nil
			m.entitiesList.SetItems(nil)
			m.entitiesList.Title = "Entities"
		}
		ui.RestoreListState(&m.entitiesList, m.entitiesHistory[entityCacheKey], entityItemKey)

		m.subscriptionsList.SetItems(nil)
		m.queueTypeList.SetItems(nil)
		m.messageList.SetItems(nil)
		m.resize()

		m.SetLoading(m.focus)
		m.loadingSpinnerID = m.NotifySpinner(fmt.Sprintf("Loading entities in %s", item.namespace.Name))
		return m, tea.Batch(m.Spinner.Tick, fetchEntitiesCmd(m.service, m.cache.entities, item.namespace, entityCacheKey, m.entities))
	}

	if m.focus == entitiesPane {
		item, ok := m.entitiesList.SelectedItem().(entityItem)
		if !ok {
			return m, nil
		}
		if item.entity.Kind == servicebus.EntityQueue {
			return m.selectQueue(item.entity)
		}
		return m.selectTopic(item.entity)
	}

	if m.focus == subscriptionsPane {
		item, ok := m.subscriptionsList.SelectedItem().(subscriptionItem)
		if !ok {
			return m, nil
		}
		return m.selectSubscriptionSub(m.currentEntity.Name, item.sub)
	}

	if m.focus == queueTypePane {
		item, ok := m.queueTypeList.SelectedItem().(queueTypeItem)
		if !ok {
			return m, nil
		}
		return m.peekMessages(item.deadLetter)
	}

	if m.focus == messagesPane {
		item, ok := m.messageList.SelectedItem().(messageItem)
		if !ok {
			return m, nil
		}
		m.selectedMessage = item.message
		m.viewingMessage = true
		m.setFocus(messagePreviewPane)
		m.messageViewport.SetContent(m.Styles.Syntax.HighlightJSON(item.message.FullBody))
		m.messageViewport.GotoTop()
		m.Notify(appshell.LevelInfo, fmt.Sprintf("Viewing message %s", ui.EmptyToDash(item.message.MessageID)))
		return m, nil
	}

	return m, nil
}

// selectQueue binds the queue type picker to a queue.
func (m Model) selectQueue(entity servicebus.Entity) (Model, tea.Cmd) {
	if m.hasPeekTarget && m.currentSubName == "" && m.currentEntity.Name == entity.Name {
		m.setFocus(queueTypePane)
		return m, nil
	}

	m.closePreview()
	m.currentEntity = entity
	m.currentSubName = ""
	m.hasPeekTarget = true
	m.peekedMessages = nil
	m.deadLetter = false
	m.subscriptions = nil
	m.buildQueueTypeItems()
	m.setFocus(queueTypePane)

	m.messageList.ResetFilter()
	m.messageList.SetItems(nil)
	m.resize()

	return m, nil
}

// selectTopic loads a topic's subscriptions.
func (m Model) selectTopic(entity servicebus.Entity) (Model, tea.Cmd) {
	if m.currentEntity.Name == entity.Name && m.isTopicSelected() {
		m.setFocus(subscriptionsPane)
		return m, nil
	}

	m.closePreview()
	m.currentEntity = entity
	m.currentSubName = ""
	m.hasPeekTarget = false
	m.peekedMessages = nil
	m.deadLetter = false
	m.setFocus(subscriptionsPane)

	cacheKey := cache.Key(m.CurrentSub.ID, m.currentNS.Name, entity.Name)
	if cached, ok := m.cache.topicSubs.Get(cacheKey); ok {
		m.subscriptions = cached
		m.subscriptionsList.SetItems(subscriptionsToItems(cached))
		m.subscriptionsList.Title = fmt.Sprintf("Subscriptions (%d)", len(cached))
	} else {
		m.subscriptions = nil
		m.subscriptionsList.SetItems(nil)
		m.subscriptionsList.Title = "Subscriptions"
	}
	ui.RestoreListState(&m.subscriptionsList, m.subscriptionsHistory[cacheKey], subscriptionItemKey)

	m.queueTypeList.SetItems(nil)
	m.messageList.SetItems(nil)
	m.resize()

	m.SetLoading(m.focus)
	m.loadingSpinnerID = m.NotifySpinner(fmt.Sprintf("Loading subscriptions for topic %s", entity.Name))
	return m, tea.Batch(m.Spinner.Tick, fetchTopicSubscriptionsCmd(m.service, m.cache.topicSubs, m.currentNS, entity.Name, cacheKey, m.subscriptions))
}

// selectSubscriptionSub binds the queue type picker to a topic subscription.
func (m Model) selectSubscriptionSub(topicName string, sub servicebus.TopicSubscription) (Model, tea.Cmd) {
	if m.hasPeekTarget && m.currentSubName == sub.Name && m.currentEntity.Name == topicName {
		m.setFocus(queueTypePane)
		return m, nil
	}

	var parent servicebus.Entity
	for _, e := range m.entities {
		if e.Name == topicName {
			parent = e
			break
		}
	}

	m.closePreview()
	m.currentEntity = parent
	m.currentSubName = sub.Name
	m.hasPeekTarget = true
	m.peekedMessages = nil
	m.deadLetter = false
	m.buildQueueTypeItems()
	m.setFocus(queueTypePane)

	m.messageList.ResetFilter()
	m.messageList.SetItems(nil)
	m.resize()

	return m, nil
}

// peekMessages navigates to the messages pane for the given queue type.
// If the same scope is already loaded, just re-focuses. Messages are
// NOT peeked automatically — the user opens the action menu to peek.
func (m Model) peekMessages(deadLetter bool) (Model, tea.Cmd) {
	if m.deadLetter == deadLetter && len(m.peekedMessages) > 0 {
		m.setFocus(messagesPane)
		return m, nil
	}

	m.deadLetter = deadLetter
	m.peekedMessages = nil
	m.setFocus(messagesPane)

	m.messageList.ResetFilter()
	m.messageList.SetItems(nil)
	m.messageList.Title = m.messagesPaneTitle()
	m.resize()

	return m, nil
}
