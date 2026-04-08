package sbapp

import (
	"fmt"

	"github.com/karlssonsimon/lazyaz/internal/azure"
	"github.com/karlssonsimon/lazyaz/internal/azure/servicebus"
	"github.com/karlssonsimon/lazyaz/internal/cache"
	"github.com/karlssonsimon/lazyaz/internal/ui"

	tea "charm.land/bubbletea/v2"
)

func (m Model) navigateLeft() (Model, tea.Cmd) {
	switch m.focus {
	case messagePreviewPane:
		// Move focus to the message list. The preview pane stays
		// mounted and follows the cursor — it's only torn down when
		// the user actually leaves the entity (one more 'left').
		m.focus = detailPane
		return m, nil
	case detailPane:
		// Leaving the entity entirely → close the preview pane.
		m.closePreview()
		m.focus = entitiesPane
		return m, nil
	case entitiesPane:
		// If the cursor is on a topic-sub child, collapse the parent
		// topic and move the cursor onto it. Otherwise, if the cursor
		// is on an expanded topic, just collapse it. Otherwise, go to
		// the namespaces pane.
		if collapsed, ok := m.collapseFocusedTopic(); ok {
			return collapsed, nil
		}
		m.focus = namespacesPane
		return m, nil
	case namespacesPane:
		return m, nil
	default:
		return m, nil
	}
}

func (m Model) handleBackspace() (Model, tea.Cmd) {
	if m.focus == messagePreviewPane {
		m.focus = detailPane
		return m, nil
	}
	if m.focus == detailPane {
		m.closePreview()
		m.focus = entitiesPane
		return m, nil
	}
	if m.focus == entitiesPane {
		if collapsed, ok := m.collapseFocusedTopic(); ok {
			return collapsed, nil
		}
	}
	return m, nil
}

// closePreview tears down the message preview pane state. Called when
// the user navigates away from the current entity (back to entities
// pane, into a different queue/sub, or namespace/sub change).
func (m *Model) closePreview() {
	m.viewingMessage = false
	m.selectedMessage = servicebus.PeekedMessage{}
}

// collapseFocusedTopic handles the "h / backspace" semantics for the
// entities tree: if the cursor is on a topic-sub child, collapse the
// parent and move the cursor onto it. If the cursor is on an expanded
// topic, just collapse. Returns (m, true) if anything was collapsed.
func (m Model) collapseFocusedTopic() (Model, bool) {
	switch sel := m.entitiesList.SelectedItem().(type) {
	case topicSubChildItem:
		delete(m.expandedTopics, sel.parentTopic)
		m.rebuildEntitiesItems()
		// Move cursor to the parent topic.
		for i, it := range m.entitiesList.VisibleItems() {
			if ei, ok := it.(entityItem); ok && ei.entity.Name == sel.parentTopic {
				m.entitiesList.Select(i)
				break
			}
		}
		return m, true
	case entityItem:
		if sel.entity.Kind == servicebus.EntityTopic && m.expandedTopics[sel.entity.Name] {
			delete(m.expandedTopics, sel.entity.Name)
			m.rebuildEntitiesItems()
			return m, true
		}
	}
	return m, false
}

func (m Model) selectSubscription(sub azure.Subscription) (Model, tea.Cmd) {
	// Re-selecting the same subscription: no-op.
	if m.HasSubscription && m.CurrentSub.ID == sub.ID {
		return m, nil
	}

	// Snapshot the current namespaces list under the outgoing sub.
	if m.HasSubscription {
		m.namespacesHistory[m.CurrentSub.ID] = ui.SnapshotListState(&m.namespacesList, namespaceItemKey)
	}

	m.CurrentSub = sub
	m.HasSubscription = true
	m.hasNamespace = false
	m.currentNS = servicebus.Namespace{}
	m.expandedTopics = make(map[string]bool)
	m.topicSubsByTopic = make(map[string][]servicebus.TopicSubscription)
	m.clearPeekState()
	m.clearAllMarks()
	m.focus = namespacesPane

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
	m.detailList.ResetFilter()
	m.entitiesList.SetItems(nil)
	m.detailList.SetItems(nil)
	m.entitiesList.Title = "Entities"
	m.detailList.Title = "Detail"
	m.resize() // peek state cleared → reclaim DLQ tab strip space

	m.fetchGen++
	m.namespacesSession = cache.NewFetchSession(m.namespaces, m.fetchGen, namespaceKey)
	m.SetLoading(m.focus)
	m.Status = fmt.Sprintf("Loading namespaces in %s", ui.SubscriptionDisplayName(sub))
	return m, tea.Batch(m.Spinner.Tick, fetchNamespacesCmd(m.service, m.cache.namespaces, sub.ID, m.fetchGen))
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

		// Snapshot current entities list under the outgoing namespace.
		if m.hasNamespace {
			oldKey := cache.Key(m.CurrentSub.ID, m.currentNS.Name)
			m.entitiesHistory[oldKey] = ui.SnapshotListState(&m.entitiesList, entitiesTreeItemKey)
		}

		m.currentNS = item.namespace
		m.hasNamespace = true
		m.expandedTopics = make(map[string]bool)
		m.topicSubsByTopic = make(map[string][]servicebus.TopicSubscription)
		m.clearPeekState()
		m.clearAllMarks()
		m.focus = entitiesPane

		entityCacheKey := cache.Key(m.CurrentSub.ID, item.namespace.Name)
		if cached, ok := m.cache.entities.Get(entityCacheKey); ok {
			m.entities = cached
			m.entitiesList.SetItems(entitiesTreeToItems(cached, m.topicSubsByTopic, m.expandedTopics, m.entityFilter, m.dlqSort))
			m.entitiesList.Title = m.entitiesPaneTitle()
		} else {
			m.entities = nil
			m.entitiesList.SetItems(nil)
			m.entitiesList.Title = "Entities"
		}
		ui.RestoreListState(&m.entitiesList, m.entitiesHistory[entityCacheKey], entitiesTreeItemKey)

		m.detailList.ResetFilter()
		m.detailList.SetItems(nil)
		m.detailList.Title = "Detail"
		m.resize() // peek state cleared → reclaim DLQ tab strip space

		m.fetchGen++
		m.entitiesSession = cache.NewFetchSession(m.entities, m.fetchGen, entityKey)
		m.SetLoading(m.focus)
		m.Status = fmt.Sprintf("Loading entities in %s", item.namespace.Name)
		return m, tea.Batch(m.Spinner.Tick, fetchEntitiesCmd(m.service, m.cache.entities, item.namespace, entityCacheKey, m.fetchGen))
	}

	if m.focus == entitiesPane {
		switch sel := m.entitiesList.SelectedItem().(type) {
		case entityItem:
			if sel.entity.Kind == servicebus.EntityQueue {
				return m.peekQueue(sel.entity)
			}
			// Topic — toggle expansion.
			return m.toggleTopicExpansion(sel.entity)
		case topicSubChildItem:
			return m.peekTopicSub(sel.parentTopic, sel.sub)
		default:
			return m, nil
		}
	}

	if m.focus == detailPane {
		item, ok := m.detailList.SelectedItem().(messageItem)
		if !ok {
			return m, nil
		}
		m.selectedMessage = item.message
		m.viewingMessage = true
		m.focus = messagePreviewPane
		m.resize()
		m.messageViewport.SetContent(m.Styles.Syntax.HighlightJSON(item.message.FullBody))
		m.messageViewport.GotoTop()
		m.Status = fmt.Sprintf("Viewing message %s (%s to back to list)", ui.EmptyToDash(item.message.MessageID), m.Keymap.PreviousFocus.Short())
		return m, nil
	}

	if m.focus == messagePreviewPane {
		// Pressing Enter while focused on the preview pane is a no-op —
		// the message is already selected. (Could re-yank to clipboard
		// or similar in the future.)
		return m, nil
	}

	return m, nil
}

// peekQueue starts a fresh peek of a queue's messages, binding the
// detail pane to it.
func (m Model) peekQueue(entity servicebus.Entity) (Model, tea.Cmd) {
	// Re-selecting the same queue: just move focus.
	if m.hasPeekTarget && m.currentSubName == "" && m.currentEntity.Name == entity.Name {
		m.focus = detailPane
		return m, nil
	}

	m.closePreview()
	m.currentEntity = entity
	m.currentSubName = ""
	m.hasPeekTarget = true
	m.peekedMessages = nil
	m.deadLetter = false
	m.focus = detailPane

	m.detailList.ResetFilter()
	m.detailList.SetItems(nil)
	m.detailList.Title = "Detail"
	m.resize() // make room for the DLQ tab strip

	m.SetLoading(m.focus)
	m.Status = fmt.Sprintf("Peeking messages from queue %s", entity.Name)
	return m, tea.Batch(m.Spinner.Tick, peekQueueMessagesCmd(m.service, m.currentNS, entity.Name, m.deadLetter, false))
}

// peekTopicSub starts a fresh peek of a topic subscription's messages,
// binding the detail pane to it.
func (m Model) peekTopicSub(topicName string, sub servicebus.TopicSubscription) (Model, tea.Cmd) {
	// Re-selecting the same sub: just move focus.
	if m.hasPeekTarget && m.currentSubName == sub.Name && m.currentEntity.Name == topicName {
		m.focus = detailPane
		return m, nil
	}

	// Find the parent topic entity in m.entities so we have its full
	// metadata (kind etc.) available for refresh paths.
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
	m.focus = detailPane

	m.detailList.ResetFilter()
	m.detailList.SetItems(nil)
	m.detailList.Title = "Detail"
	m.resize() // make room for the DLQ tab strip

	m.SetLoading(m.focus)
	m.Status = fmt.Sprintf("Peeking messages from %s/%s", topicName, sub.Name)
	return m, tea.Batch(m.Spinner.Tick, peekSubscriptionMessagesCmd(m.service, m.currentNS, topicName, sub.Name, m.deadLetter, false))
}

// toggleTopicExpansion expands or collapses a topic in the entities
// tree. On first expand, kicks off a fetch of the topic's subscriptions.
// On collapse, just hides the children.
func (m Model) toggleTopicExpansion(topic servicebus.Entity) (Model, tea.Cmd) {
	if m.expandedTopics[topic.Name] {
		// Collapse.
		delete(m.expandedTopics, topic.Name)
		m.rebuildEntitiesItems()
		return m, nil
	}

	m.expandedTopics[topic.Name] = true

	// If we already have the subs cached (in-memory or persistent), show
	// them immediately and re-fetch in the background to refresh.
	cacheKey := cache.Key(m.CurrentSub.ID, m.currentNS.Name, topic.Name)
	if cached, ok := m.cache.topicSubs.Get(cacheKey); ok {
		m.topicSubsByTopic[topic.Name] = cached
	}
	m.rebuildEntitiesItems()

	// Fire a fetch (always — to refresh).
	m.fetchGen++
	m.topicSubsFetching = topic.Name
	m.topicSubsSession = cache.NewFetchSession(m.topicSubsByTopic[topic.Name], m.fetchGen, topicSubKey)
	m.SetLoading(m.focus)
	m.Status = fmt.Sprintf("Loading subscriptions for topic %s", topic.Name)
	return m, tea.Batch(m.Spinner.Tick, fetchTopicSubscriptionsCmd(m.service, m.cache.topicSubs, m.currentNS, topic.Name, cacheKey, m.fetchGen))
}

