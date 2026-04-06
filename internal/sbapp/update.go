package sbapp

import (
	"errors"
	"fmt"
	"time"

	"azure-storage/internal/appshell"
	"azure-storage/internal/azure/servicebus"
	"azure-storage/internal/cache"
	"azure-storage/internal/ui"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
)

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.Width = msg.Width
		m.Height = msg.Height
		m.resize()
		return m, nil

	case spinner.TickMsg:
		if !m.Loading {
			return m, nil
		}
		var cmd tea.Cmd
		m.Spinner, cmd = m.Spinner.Update(msg)
		return m, cmd

	case appshell.LoadingHoldExpiredMsg:
		m.ClearLoading()
		m.Status = msg.Status
		return m, nil

	case appshell.SubscriptionsLoadedMsg:
		return m.handleSubscriptionsLoaded(msg)

	case namespacesLoadedMsg:
		return m.handleNamespacesLoaded(msg)

	case entitiesLoadedMsg:
		return m.handleEntitiesLoaded(msg)

	case topicSubscriptionsLoadedMsg:
		return m.handleTopicSubscriptionsLoaded(msg)

	case messagesLoadedMsg:
		return m.handleMessagesLoaded(msg)

	case requeueDoneMsg:
		return m.handleRequeueDone(msg)

	case deleteDuplicateDoneMsg:
		return m.handleDeleteDuplicateDone(msg)

	case entitiesRefreshedMsg:
		return m.handleEntitiesRefreshed(msg)

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	// Fallthrough: propagate to the focused list.
	var cmd tea.Cmd
	switch m.focus {
	case namespacesPane:
		m.namespacesList, cmd = m.namespacesList.Update(msg)
	case entitiesPane:
		m.entitiesList, cmd = m.entitiesList.Update(msg)
	case detailPane:
		m.detailList, cmd = m.detailList.Update(msg)
	}
	return m, cmd
}

func (m Model) handleSubscriptionsLoaded(msg appshell.SubscriptionsLoadedMsg) (Model, tea.Cmd) {
	if msg.Err != nil {
		m.ClearLoading()
		m.LastErr = msg.Err.Error()
		m.Status = "Failed to load subscriptions"
		return m, nil
	}

	m.LastErr = ""
	m.Subscriptions = msg.Subscriptions

	if msg.Done {
		m.cache.subscriptions.Set("", msg.Subscriptions)
		if !m.HasSubscription {
			m.SubOverlay.Open()
		}
		status := fmt.Sprintf("Loaded %d subscriptions in %s", len(msg.Subscriptions), time.Since(m.LoadingStartedAt).Round(time.Millisecond))
		return m, m.FinishLoading(status)
	}

	return m, msg.Next
}

func (m Model) handleNamespacesLoaded(msg namespacesLoadedMsg) (Model, tea.Cmd) {
	if !m.HasSubscription || m.CurrentSub.ID != msg.subscriptionID {
		return m, nil
	}

	if msg.err != nil {
		m.ClearLoading()
		m.LastErr = msg.err.Error()
		m.Status = fmt.Sprintf("Failed to load namespaces in %s", ui.SubscriptionDisplayName(m.CurrentSub))
		return m, nil
	}

	m.LastErr = ""
	m.namespaces = msg.namespaces
	m.namespacesList.Title = fmt.Sprintf("Namespaces (%d)", len(msg.namespaces))
	ui.SetItemsPreserveIndex(&m.namespacesList, namespacesToItems(msg.namespaces))

	if msg.done {
		m.cache.namespaces.Set(msg.subscriptionID, msg.namespaces)
		status := fmt.Sprintf("Loaded %d namespaces from %s in %s", len(msg.namespaces), ui.SubscriptionDisplayName(m.CurrentSub), time.Since(m.LoadingStartedAt).Round(time.Millisecond))
		return m, m.FinishLoading(status)
	}

	return m, msg.next
}

func (m Model) handleEntitiesLoaded(msg entitiesLoadedMsg) (Model, tea.Cmd) {
	if !m.hasNamespace || m.currentNS.Name != msg.namespace.Name {
		return m, nil
	}

	if msg.err != nil {
		m.ClearLoading()
		m.LastErr = msg.err.Error()
		m.Status = fmt.Sprintf("Failed to load entities in %s", msg.namespace.Name)
		return m, nil
	}

	m.LastErr = ""
	m.entities = msg.entities
	items := entitiesToFilteredItems(m.entities, m.dlqFilter)
	ui.SetItemsPreserveIndex(&m.entitiesList, items)
	m.entitiesList.Title = m.entitiesPaneTitle()

	if msg.done {
		m.cache.entities.Set(cache.Key(m.CurrentSub.ID, msg.namespace.Name), msg.entities)
		status := fmt.Sprintf("Loaded %d entities from %s in %s", len(msg.entities), msg.namespace.Name, time.Since(m.LoadingStartedAt).Round(time.Millisecond))
		return m, m.FinishLoading(status)
	}

	return m, msg.next
}

func (m Model) handleTopicSubscriptionsLoaded(msg topicSubscriptionsLoadedMsg) (Model, tea.Cmd) {
	if !m.hasEntity || m.currentEntity.Kind != servicebus.EntityTopic {
		return m, nil
	}
	if m.currentNS.Name != msg.namespace.Name || m.currentEntity.Name != msg.topicName {
		return m, nil
	}

	if msg.err != nil {
		m.ClearLoading()
		m.LastErr = msg.err.Error()
		m.Status = fmt.Sprintf("Failed to load subscriptions for topic %s", msg.topicName)
		return m, nil
	}

	m.LastErr = ""
	m.topicSubs = msg.subs
	m.detailMode = detailTopicSubscriptions
	m.detailList.Title = fmt.Sprintf("Topic Subscriptions (%d)", len(msg.subs))
	ui.SetItemsPreserveIndex(&m.detailList, topicSubsToItems(msg.subs))

	if msg.done {
		m.cache.topicSubs.Set(cache.Key(m.CurrentSub.ID, msg.namespace.Name, msg.topicName), msg.subs)
		status := fmt.Sprintf("Loaded %d subscriptions for topic %s in %s", len(msg.subs), msg.topicName, time.Since(m.LoadingStartedAt).Round(time.Millisecond))
		return m, m.FinishLoading(status)
	}

	return m, msg.next
}

func (m Model) handleMessagesLoaded(msg messagesLoadedMsg) (Model, tea.Cmd) {
	// Messages are ephemeral peek results — not cached.
	m.ClearLoading()
	if msg.err != nil {
		m.LastErr = msg.err.Error()
		m.Status = fmt.Sprintf("Failed to peek messages from %s", msg.source)
		return m, nil
	}

	m.LastErr = ""
	m.peekedMessages = msg.messages
	m.detailMode = detailMessages
	m.viewingMessage = false
	m.selectedMessage = servicebus.PeekedMessage{}
	m.detailList.ResetFilter()
	m.detailList.SetItems(messagesToItems(msg.messages, m.markedMessages, m.duplicateMessages))
	m.detailList.Title = fmt.Sprintf("Messages (%d)", len(msg.messages))
	if len(msg.messages) > 0 {
		m.detailList.Select(0)
	}
	m.resize()
	m.Status = fmt.Sprintf("Peeked %d messages from %s", len(msg.messages), msg.source)
	return m, nil
}

func (m Model) handleRequeueDone(msg requeueDoneMsg) (Model, tea.Cmd) {
	m.ClearLoading()
	m.markedMessages = make(map[string]struct{})
	if msg.err != nil {
		var dupErr *servicebus.DuplicateError
		if errors.As(msg.err, &dupErr) {
			m.duplicateMessages[dupErr.MessageID] = struct{}{}
			m.LastErr = fmt.Sprintf("message %s sent but not removed from DLQ (possible duplicate)", dupErr.MessageID)
		} else {
			m.LastErr = msg.err.Error()
		}
	} else {
		m.LastErr = ""
	}
	if msg.requeued > 0 {
		m.Status = fmt.Sprintf("%d of %d message(s) requeued", msg.requeued, msg.total)
	} else {
		m.Status = "Failed to requeue messages"
	}
	var peekCmd tea.Cmd
	m, peekCmd = m.rePeekMessages()
	return m, tea.Batch(peekCmd, refreshEntitiesCmd(m.service, m.currentNS))
}

func (m Model) handleDeleteDuplicateDone(msg deleteDuplicateDoneMsg) (Model, tea.Cmd) {
	m.ClearLoading()
	if msg.err != nil {
		m.LastErr = msg.err.Error()
		m.Status = "Failed to delete duplicate message"
		return m, nil
	}
	m.LastErr = ""
	delete(m.duplicateMessages, msg.messageID)
	m.Status = "Duplicate message deleted"
	var peekCmd tea.Cmd
	m, peekCmd = m.rePeekMessages()
	return m, tea.Batch(peekCmd, refreshEntitiesCmd(m.service, m.currentNS))
}

func (m Model) handleEntitiesRefreshed(msg entitiesRefreshedMsg) (Model, tea.Cmd) {
	if msg.err != nil {
		return m, nil
	}
	m.entities = msg.entities
	idx := m.entitiesList.Index()
	m.applyEntityFilter()
	if n := len(m.entitiesList.Items()); n > 0 {
		if idx >= n {
			idx = n - 1
		}
		m.entitiesList.Select(idx)
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	key := msg.String()

	if result := m.HandleOverlayKeys(key); result.Handled {
		if result.SelectSub != nil {
			return m.selectSubscription(*result.SelectSub)
		}
		if result.ThemeSelected {
			m.applyScheme(m.Schemes[m.ThemeOverlay.ActiveThemeIdx])
			ui.SaveThemeName(m.Schemes[m.ThemeOverlay.ActiveThemeIdx].Name)
		}
		return m, nil
	}

	// Message preview viewport captures keys when open.
	if m.viewingMessage {
		return m.handleViewingMessageKey(msg, key)
	}

	focusedFilterActive := m.focusedListSettingFilter()

	switch {
	case ui.ShouldQuit(key, m.Keymap.Quit, focusedFilterActive):
		return m, tea.Quit
	case m.Keymap.HalfPageDown.Matches(key):
		m.scrollFocusedHalfPage(1)
		return m, nil
	case m.Keymap.HalfPageUp.Matches(key):
		m.scrollFocusedHalfPage(-1)
		return m, nil
	case m.Keymap.NextFocus.Matches(key):
		if !focusedFilterActive {
			m.nextFocus()
			return m, nil
		}
	case m.Keymap.PreviousFocus.Matches(key):
		if !focusedFilterActive {
			m.previousFocus()
			return m, nil
		}
	case m.Keymap.RefreshScope.Matches(key):
		if !focusedFilterActive {
			return m.refresh()
		}
	case m.Keymap.OpenFocused.Matches(key):
		if focusedFilterActive {
			m.commitFocusedFilter()
			m.Status = fmt.Sprintf("Filter applied for %s", paneName(m.focus))
			return m, nil
		}
		return m.handleEnter()
	case m.Keymap.OpenFocusedAlt.Matches(key):
		if !focusedFilterActive {
			return m.handleEnter()
		}
	case m.Keymap.NavigateLeft.Matches(key):
		if !focusedFilterActive {
			return m.navigateLeft()
		}
	case m.Keymap.ToggleMark.Matches(key):
		if !focusedFilterActive && m.focus == detailPane && m.detailMode == detailMessages {
			item, ok := m.detailList.SelectedItem().(messageItem)
			if !ok {
				return m, nil
			}
			if item.duplicate {
				return m, nil
			}
			id := item.message.MessageID
			if _, marked := m.markedMessages[id]; marked {
				delete(m.markedMessages, id)
				m.Status = fmt.Sprintf("Unmarked %s (%d marked)", id, len(m.markedMessages))
			} else {
				m.markedMessages[id] = struct{}{}
				m.Status = fmt.Sprintf("Marked %s (%d marked)", id, len(m.markedMessages))
			}
			m.refreshItems()
			return m, nil
		}
	case m.Keymap.ShowActiveQueue.Matches(key):
		if !focusedFilterActive && m.focus == detailPane && m.detailMode == detailMessages {
			if m.deadLetter {
				m.deadLetter = false
				m.markedMessages = make(map[string]struct{})
				m.duplicateMessages = make(map[string]struct{})
				return m.rePeekMessages()
			}
		}
	case m.Keymap.ShowDeadLetterQueue.Matches(key):
		if !focusedFilterActive && m.focus == detailPane && m.detailMode == detailMessages {
			if !m.deadLetter {
				m.deadLetter = true
				m.markedMessages = make(map[string]struct{})
				m.duplicateMessages = make(map[string]struct{})
				return m.rePeekMessages()
			}
		}
	case m.Keymap.RequeueDLQ.Matches(key):
		if !focusedFilterActive && m.focus == detailPane && m.detailMode == detailMessages && m.deadLetter {
			messageIDs := m.collectRequeueIDs()
			if len(messageIDs) == 0 {
				return m, nil
			}
			m.SetLoading(m.focus)
			m.LastErr = ""
			m.Status = fmt.Sprintf("Requeuing %d message(s)...", len(messageIDs))
			return m, tea.Batch(spinner.Tick, requeueMessagesCmd(m.service, m.currentNS, m.currentEntity, m.viewingTopicSub, m.currentTopicSub, messageIDs))
		}
	case m.Keymap.DeleteDuplicate.Matches(key):
		if !focusedFilterActive && m.focus == detailPane && m.detailMode == detailMessages && m.deadLetter {
			item, ok := m.detailList.SelectedItem().(messageItem)
			if !ok || !item.duplicate {
				return m, nil
			}
			m.SetLoading(m.focus)
			m.LastErr = ""
			m.Status = "Deleting duplicate message..."
			return m, tea.Batch(spinner.Tick, deleteDuplicateCmd(m.service, m.currentNS, m.currentEntity, m.viewingTopicSub, m.currentTopicSub, item.message.MessageID))
		}
	case m.Keymap.ToggleDLQFilter.Matches(key):
		if !focusedFilterActive {
			m.dlqFilter = !m.dlqFilter
			m.applyEntityFilter()
			if m.dlqFilter {
				m.Status = "DLQ filter enabled – showing only entities with dead-letter messages"
			} else {
				m.Status = "DLQ filter disabled – showing all entities"
			}
			return m, nil
		}
	case m.Keymap.SubscriptionPicker.Matches(key):
		if !focusedFilterActive {
			m.SubOverlay.Open()
			m.SetLoading(-1)
			m.LastErr = ""
			m.Status = "Refreshing subscriptions..."
			return m, tea.Batch(spinner.Tick, fetchSubscriptionsCmd(m.service, m.cache.subscriptions, true))
		}
	case !m.EmbeddedMode && m.Keymap.ToggleThemePicker.Matches(key):
		if !focusedFilterActive && !m.ThemeOverlay.Active {
			m.ThemeOverlay.Open()
			return m, nil
		}
	case !m.EmbeddedMode && m.Keymap.ToggleHelp.Matches(key):
		if !focusedFilterActive && !m.ThemeOverlay.Active {
			if m.HelpOverlay.Active {
				m.HelpOverlay.Close()
			} else {
				m.HelpOverlay.Open("Azure Service Bus Explorer Help", m.HelpSections())
			}
			return m, nil
		}
	case m.Keymap.Inspect.Matches(key):
		if !focusedFilterActive {
			m.inspectFocusedItem()
			return m, nil
		}
	case m.Keymap.BackspaceUp.Matches(key):
		if !focusedFilterActive {
			return m.handleBackspace()
		}
	}

	// Key didn't match any app-specific handler — fall through to the
	// focused list so filter input and cursor keys reach it.
	var cmd tea.Cmd
	switch m.focus {
	case namespacesPane:
		m.namespacesList, cmd = m.namespacesList.Update(msg)
	case entitiesPane:
		m.entitiesList, cmd = m.entitiesList.Update(msg)
	case detailPane:
		m.detailList, cmd = m.detailList.Update(msg)
	}
	return m, cmd
}

// handleViewingMessageKey routes key events while the message-preview
// viewport is open. Most keys scroll the viewport; only quit and the
// message-back binding are intercepted.
func (m Model) handleViewingMessageKey(msg tea.KeyMsg, key string) (Model, tea.Cmd) {
	switch {
	case ui.ShouldQuit(key, m.Keymap.Quit, false):
		return m, tea.Quit
	case m.Keymap.MessageBack.Matches(key):
		m.viewingMessage = false
		m.selectedMessage = servicebus.PeekedMessage{}
		m.resize()
		m.Status = "Back to messages"
		return m, nil
	}
	var cmd tea.Cmd
	m.messageViewport, cmd = m.messageViewport.Update(msg)
	return m, cmd
}
