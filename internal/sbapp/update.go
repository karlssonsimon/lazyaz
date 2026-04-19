package sbapp

import (
	"context"
	"fmt"
	"time"

	"github.com/karlssonsimon/lazyaz/internal/appshell"
	"github.com/karlssonsimon/lazyaz/internal/azure/servicebus"
	"github.com/karlssonsimon/lazyaz/internal/ui"

	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
)

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if paste, ok := msg.(tea.PasteMsg); ok {
		text := paste.String()
		switch {
		case m.SubOverlay.Active:
			m.SubOverlay.Query += text
			m.SubOverlay.Refilter(m.Subscriptions)
			return m, nil
		case m.ThemeOverlay.Active:
			m.ThemeOverlay.PasteText(text, m.Schemes)
			return m, nil
		case m.HelpOverlay.Active:
			m.HelpOverlay.PasteText(text)
			return m, nil
		case m.entitySortOverlay.active:
			m.entitySortOverlay.query += text
			m.entitySortOverlay.refilter()
			return m, nil
		case m.targetPicker.active:
			m.targetPicker.query += text
			m.targetPicker.refilter()
			return m, nil
		default:
			return m.updateFocusedList(msg)
		}
	}

	if cursorModel, cursorCmd := m.Cursor.Update(msg); cursorCmd != nil {
		m.Cursor = cursorModel
		_, listCmd := m.updateFocusedList(msg)
		return m, tea.Batch(cursorCmd, listCmd)
	}

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
	case dlqReceivedMsg:
		return m.handleDLQReceived(msg)
	case dlqCompleteMsg:
		return m.handleDLQComplete(msg)
	case dlqRequeueMsg:
		return m.handleDLQRequeue(msg)
	case dlqRequeueAllMsg:
		return m.handleDLQRequeueAll(msg)
	case dlqAbandonMsg:
		return m.handleDLQAbandon(msg)
	case entitiesRefreshedMsg:
		return m.handleEntitiesRefreshed(msg)
	case moveAllDoneMsg:
		return m.handleMoveAllDone(msg)
	case moveMarkedDoneMsg:
		return m.handleMoveMarkedDone(msg)
	case targetEntitiesLoadedMsg:
		return m.handleTargetEntitiesLoaded(msg)

	case tea.KeyMsg:
		return m.handleKey(msg)

	case tea.MouseClickMsg:
		if m.viewingMessage {
			region := m.messageViewportRegion()
			if m.textSelection.HandleMouseClick(msg, region) {
				return m, nil
			}
		}
		if consumed, double := m.handleMouseClick(msg); consumed {
			if double {
				return m.handleEnter()
			}
			return m, nil
		}

	case tea.MouseMotionMsg:
		if m.textSelection.Active {
			region := m.messageViewportRegion()
			m.textSelection.HandleMouseMotion(msg, region)
			return m, nil
		}
	case tea.MouseReleaseMsg:
		if m.textSelection.Active {
			region := m.messageViewportRegion()
			text, ok := m.textSelection.HandleMouseRelease(msg, m.messageViewport, region)
			if ok {
				return m, func() tea.Msg {
					if err := ui.WriteClipboard(text); err != nil {
						return clipboardMsg{err: err}
					}
					return clipboardMsg{text: text}
				}
			}
			return m, nil
		}
	case clipboardMsg:
		if msg.err != nil {
			m.Notify(appshell.LevelError, fmt.Sprintf("Clipboard: %s", msg.err.Error()))
		} else {
			m.Notify(appshell.LevelSuccess, fmt.Sprintf("Copied to clipboard: %s", ui.TrimToWidth(msg.text, 60)))
		}
		return m, nil
	}

	return m.updateFocusedList(msg)
}

func (m Model) updateFocusedList(msg tea.Msg) (Model, tea.Cmd) {
	var cmd tea.Cmd
	switch m.focus {
	case namespacesPane:
		m.namespacesList, cmd = m.namespacesList.Update(msg)
	case entitiesPane:
		m.entitiesList, cmd = m.entitiesList.Update(msg)
	case subscriptionsPane:
		m.subscriptionsList, cmd = m.subscriptionsList.Update(msg)
	case queueTypePane:
		m.queueTypeList, cmd = m.queueTypeList.Update(msg)
	case messagesPane:
		m.messageList, cmd = m.messageList.Update(msg)
		if m.viewingMessage {
			m.syncPreviewToSelection()
		}
	}
	return m, cmd
}

type clipboardMsg struct {
	text string
	err  error
}

func (m Model) handleSubscriptionsLoaded(msg appshell.SubscriptionsLoadedMsg) (Model, tea.Cmd) {
	if msg.Err != nil {
		m.ClearLoading()
		m.ResolveSpinner(m.loadingSpinnerID, appshell.LevelError, fmt.Sprintf("Failed to load subscriptions: %s", msg.Err.Error()))
		return m, nil
	}

	m.Subscriptions = msg.Subscriptions
	if m.SubOverlay.Active {
		m.SubOverlay.Refilter(m.Subscriptions)
	}

	if msg.Done {
		m.cache.subscriptions.Set("", msg.Subscriptions)
		status := fmt.Sprintf("Loaded %d subscriptions in %s", len(msg.Subscriptions), time.Since(m.LoadingStartedAt).Round(time.Millisecond))
		if !m.HasSubscription {
			if matched, ok := m.TryApplyPreferredSubscription(); ok {
				m.SubOverlay.Close()
				next, selectCmd := m.selectSubscription(matched)
				next.ClearLoading()
				next.ResolveSpinner(next.loadingSpinnerID, appshell.LevelSuccess, status)
				return next, selectCmd
			}
			m.SubOverlay.Open()
		}
		m.ClearLoading()
		m.ResolveSpinner(m.loadingSpinnerID, appshell.LevelSuccess, status)
		return m, nil
	}

	return m, msg.Next
}

func (m Model) handleNamespacesLoaded(msg namespacesLoadedMsg) (Model, tea.Cmd) {
	if !m.HasSubscription || m.CurrentSub.ID != msg.subscriptionID {
		return m, nil
	}
	if msg.err != nil {
		m.ClearLoading()
		m.ResolveSpinner(m.loadingSpinnerID, appshell.LevelError, fmt.Sprintf("Failed to load namespaces in %s: %s", ui.SubscriptionDisplayName(m.CurrentSub), msg.err.Error()))
		return m, nil
	}

	m.namespaces = msg.namespaces
	m.namespacesList.Title = fmt.Sprintf("Namespaces (%d)", len(m.namespaces))
	ui.SetItemsPreserveKey(&m.namespacesList, namespacesToItems(m.namespaces), namespaceItemKey)

	if msg.done {
		status := fmt.Sprintf("Loaded %d namespaces from %s in %s", len(m.namespaces), ui.SubscriptionDisplayName(m.CurrentSub), time.Since(m.LoadingStartedAt).Round(time.Millisecond))
		m.ClearLoading()
		m.ResolveSpinner(m.loadingSpinnerID, appshell.LevelSuccess, status)
		updated, navCmd := m.advancePendingNav()
		return updated, navCmd
	}
	return m, msg.next
}

func (m Model) handleEntitiesLoaded(msg entitiesLoadedMsg) (Model, tea.Cmd) {
	if !m.hasNamespace || m.currentNS.Name != msg.namespace.Name {
		return m, nil
	}
	if msg.err != nil {
		m.ClearLoading()
		m.ResolveSpinner(m.loadingSpinnerID, appshell.LevelError, fmt.Sprintf("Failed to load entities in %s: %s", msg.namespace.Name, msg.err.Error()))
		return m, nil
	}

	m.entities = msg.entities
	m.rebuildEntitiesItems()
	m.entitiesList.Title = m.entitiesPaneTitle()

	if msg.done {
		status := fmt.Sprintf("Loaded %d entities from %s in %s", len(m.entities), msg.namespace.Name, time.Since(m.LoadingStartedAt).Round(time.Millisecond))
		m.ClearLoading()
		m.ResolveSpinner(m.loadingSpinnerID, appshell.LevelSuccess, status)
		updated, navCmd := m.advancePendingNav()
		return updated, navCmd
	}
	return m, msg.next
}

func (m Model) handleTopicSubscriptionsLoaded(msg topicSubscriptionsLoadedMsg) (Model, tea.Cmd) {
	if !m.hasNamespace || m.currentNS.Name != msg.namespace.Name {
		return m, nil
	}
	if m.currentEntity.Name != msg.topicName || !m.isTopicSelected() {
		return m, nil
	}
	if msg.err != nil {
		m.ClearLoading()
		m.ResolveSpinner(m.loadingSpinnerID, appshell.LevelError, fmt.Sprintf("Failed to load subscriptions for topic %s: %s", msg.topicName, msg.err.Error()))
		return m, nil
	}

	m.subscriptions = msg.subs
	ui.SetItemsPreserveKey(&m.subscriptionsList, subscriptionsToItems(msg.subs), subscriptionItemKey)
	m.subscriptionsList.Title = fmt.Sprintf("Subscriptions (%d)", len(msg.subs))

	if msg.done {
		status := fmt.Sprintf("Loaded %d subscriptions for topic %s in %s", len(m.subscriptions), msg.topicName, time.Since(m.LoadingStartedAt).Round(time.Millisecond))
		m.ClearLoading()
		m.ResolveSpinner(m.loadingSpinnerID, appshell.LevelSuccess, status)
		updated, navCmd := m.advancePendingNav()
		return updated, navCmd
	}
	return m, msg.next
}

func (m Model) handleMessagesLoaded(msg messagesLoadedMsg) (Model, tea.Cmd) {
	if msg.err != nil {
		m.ClearLoading()
		m.ResolveSpinner(m.loadingSpinnerID, appshell.LevelError, fmt.Sprintf("Failed to peek messages from %s: %s", msg.source, msg.err.Error()))
		return m, nil
	}

	if msg.repeek {
		// Append mode ("peek more") — add to existing messages.
		m.peekedMessages = append(m.peekedMessages, msg.messages...)
	} else {
		m.peekedMessages = msg.messages
	}
	items := messagesToItems(m.peekedMessages, m.currentDuplicates())
	if msg.repeek || msg.preserveCursor {
		ui.SetItemsPreserveKey(&m.messageList, items, messageItemKey)
	} else {
		m.messageList.ResetFilter()
		m.messageList.SetItems(items)
		if len(msg.messages) > 0 {
			m.messageList.Select(0)
		}
	}
	if m.viewingMessage {
		m.selectedMessage = servicebus.PeekedMessage{}
		m.syncPreviewToSelection()
	}
	m.messageList.Title = fmt.Sprintf("Messages (%d)", len(m.peekedMessages))
	m.resize()
	m.ClearLoading()
	label := "active"
	if msg.deadLetter {
		label = "DLQ"
	}
	m.ResolveSpinner(m.loadingSpinnerID, appshell.LevelSuccess, fmt.Sprintf("Peeked %d %s messages from %s", len(msg.messages), label, msg.source))
	return m, nil
}


func (m Model) handleEntitiesRefreshed(msg entitiesRefreshedMsg) (Model, tea.Cmd) {
	if msg.err != nil {
		return m, nil
	}
	m.entities = msg.entities
	m.rebuildEntitiesItems()
	// Sync currentEntity so queue type counts reflect the latest data.
	if m.hasPeekTarget {
		for _, e := range m.entities {
			if e.Name == m.currentEntity.Name {
				m.currentEntity = e
				break
			}
		}
		m.buildQueueTypeItems()
	}
	return m, nil
}

func (m Model) handleDLQReceived(msg dlqReceivedMsg) (Model, tea.Cmd) {
	m.ClearLoading()
	if msg.err != nil {
		m.ResolveSpinner(m.loadingSpinnerID, appshell.LevelError, fmt.Sprintf("Failed to receive DLQ messages: %s", msg.err.Error()))
		return m, nil
	}

	m.lockedMessages = msg.result

	// Convert received messages to peeked format for display.
	m.peekedMessages = make([]servicebus.PeekedMessage, len(msg.result.Messages))
	for i, raw := range msg.result.Messages {
		pm := servicebus.PeekedMessage{
			MessageID: raw.MessageID,
			FullBody:  string(raw.Body),
		}
		if raw.EnqueuedTime != nil {
			pm.EnqueuedAt = *raw.EnqueuedTime
		}
		if len(raw.Body) > 512 {
			pm.BodyPreview = string(raw.Body[:512]) + "..."
		} else {
			pm.BodyPreview = string(raw.Body)
		}
		m.peekedMessages[i] = pm
	}

	m.messageList.ResetFilter()
	m.messageList.SetItems(messagesToItems(m.peekedMessages, nil))
	if len(m.peekedMessages) > 0 {
		m.messageList.Select(0)
	}
	m.messageList.Title = fmt.Sprintf("DLQ Locked (%d)", len(m.peekedMessages))
	m.resize()
	m.ResolveSpinner(m.loadingSpinnerID, appshell.LevelSuccess, fmt.Sprintf("Received %d DLQ messages with lock", len(m.peekedMessages)))
	return m, nil
}

func (m Model) handleDLQComplete(msg dlqCompleteMsg) (Model, tea.Cmd) {
	m.ClearLoading()
	if msg.err != nil {
		partial := ""
		if len(msg.completed) > 0 {
			partial = fmt.Sprintf(" (%d completed before error)", len(msg.completed))
		}
		m.ResolveSpinner(m.loadingSpinnerID, appshell.LevelError, fmt.Sprintf("Failed to complete messages%s: %s", partial, msg.err.Error()))
	} else {
		m.ResolveSpinner(m.loadingSpinnerID, appshell.LevelSuccess, fmt.Sprintf("Completed %d message(s) (removed from DLQ)", len(msg.completed)))
	}
	for _, id := range msg.completed {
		m.removeLockedMessage(id)
	}
	m.clearScopeMarks()
	m.refreshMessageSelectionDisplay()
	if len(msg.completed) > 0 && m.hasNamespace {
		return m, refreshEntitiesCmd(m.service, m.currentNS)
	}
	return m, nil
}

func (m Model) handleDLQRequeue(msg dlqRequeueMsg) (Model, tea.Cmd) {
	m.ClearLoading()
	if msg.err != nil {
		partial := ""
		if len(msg.requeued) > 0 {
			partial = fmt.Sprintf(" (%d requeued before error)", len(msg.requeued))
		}
		m.ResolveSpinner(m.loadingSpinnerID, appshell.LevelError, fmt.Sprintf("Failed to requeue messages%s: %s", partial, msg.err.Error()))
	} else {
		m.ResolveSpinner(m.loadingSpinnerID, appshell.LevelSuccess, fmt.Sprintf("Requeued %d message(s) to active queue", len(msg.requeued)))
	}
	for _, id := range msg.requeued {
		m.removeLockedMessage(id)
	}
	m.clearScopeMarks()
	m.refreshMessageSelectionDisplay()
	if len(msg.requeued) > 0 && m.hasNamespace {
		return m, refreshEntitiesCmd(m.service, m.currentNS)
	}
	return m, nil
}

func (m Model) handleDLQAbandon(msg dlqAbandonMsg) (Model, tea.Cmd) {
	m.ClearLoading()
	m.lockedMessages = nil // receiver already closed by the command
	m.peekedMessages = nil
	m.messageList.ResetFilter()
	m.messageList.SetItems(nil)
	m.messageList.Title = "Messages"
	if m.viewingMessage {
		m.closePreview()
	}
	if msg.err != nil {
		m.ResolveSpinner(m.loadingSpinnerID, appshell.LevelError, fmt.Sprintf("Failed to abandon: %s", msg.err.Error()))
	} else {
		m.ResolveSpinner(m.loadingSpinnerID, appshell.LevelSuccess, "Locks released")
	}
	return m, nil
}

// removeLockedMessage removes a processed message from both the locked
// set and the display list.
func (m *Model) removeLockedMessage(messageID string) {
	if m.lockedMessages == nil {
		return
	}

	// Remove from locked messages slice.
	remaining := make([]*azservicebus.ReceivedMessage, 0, len(m.lockedMessages.Messages))
	for _, msg := range m.lockedMessages.Messages {
		if msg.MessageID != messageID {
			remaining = append(remaining, msg)
		}
	}
	m.lockedMessages.Messages = remaining

	// Remove from peeked display.
	var peeked []servicebus.PeekedMessage
	for _, pm := range m.peekedMessages {
		if pm.MessageID != messageID {
			peeked = append(peeked, pm)
		}
	}
	m.peekedMessages = peeked
	m.messageList.SetItems(messagesToItems(m.peekedMessages, nil))
	m.messageList.Title = fmt.Sprintf("DLQ Locked (%d)", len(m.peekedMessages))

	// If no more locked messages, clean up the receiver.
	if len(m.lockedMessages.Messages) == 0 {
		m.lockedMessages.Receiver.Close(context.Background())
		m.lockedMessages = nil
	}
}

func (m Model) handleDLQRequeueAll(msg dlqRequeueAllMsg) (Model, tea.Cmd) {
	m.ClearLoading()
	if msg.err != nil {
		partial := ""
		if msg.requeued > 0 {
			partial = fmt.Sprintf(" (%d requeued before error)", msg.requeued)
		}
		m.ResolveSpinner(m.loadingSpinnerID, appshell.LevelError, fmt.Sprintf("Failed to requeue DLQ messages%s: %s", partial, msg.err.Error()))
	} else {
		m.ResolveSpinner(m.loadingSpinnerID, appshell.LevelSuccess, fmt.Sprintf("Requeued all %d DLQ messages to active queue", msg.requeued))
	}
	// Refresh entity counts to reflect the change.
	if m.hasNamespace {
		return m, refreshEntitiesCmd(m.service, m.currentNS)
	}
	return m, nil
}

func (m Model) handleMoveAllDone(msg moveAllDoneMsg) (Model, tea.Cmd) {
	m.ClearLoading()
	if msg.err != nil {
		partial := ""
		if msg.moved > 0 {
			partial = fmt.Sprintf(" (%d moved before error)", msg.moved)
		}
		m.ResolveSpinner(m.loadingSpinnerID, appshell.LevelError, fmt.Sprintf("Failed to move DLQ messages%s: %s", partial, msg.err.Error()))
	} else {
		m.ResolveSpinner(m.loadingSpinnerID, appshell.LevelSuccess, fmt.Sprintf("Moved all %d DLQ messages", msg.moved))
	}
	if m.hasNamespace {
		return m, refreshEntitiesCmd(m.service, m.currentNS)
	}
	return m, nil
}

func (m Model) handleMoveMarkedDone(msg moveMarkedDoneMsg) (Model, tea.Cmd) {
	m.ClearLoading()
	if msg.err != nil {
		partial := ""
		if len(msg.moved) > 0 {
			partial = fmt.Sprintf(" (%d moved before error)", len(msg.moved))
		}
		m.ResolveSpinner(m.loadingSpinnerID, appshell.LevelError, fmt.Sprintf("Failed to move messages%s: %s", partial, msg.err.Error()))
	} else {
		m.ResolveSpinner(m.loadingSpinnerID, appshell.LevelSuccess, fmt.Sprintf("Moved %d message(s)", len(msg.moved)))
	}
	for _, id := range msg.moved {
		m.removeLockedMessage(id)
	}
	m.clearScopeMarks()
	m.refreshMessageSelectionDisplay()
	if len(msg.moved) > 0 && m.hasNamespace {
		return m, refreshEntitiesCmd(m.service, m.currentNS)
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	key := msg.String()

	switch m.inputMode() {
	case ModeOverlay:
		if result := m.HandleOverlayKeys(key); result.Handled {
			if result.SelectSub != nil {
				return m.selectSubscription(*result.SelectSub)
			}
			if result.ThemeSelected {
				m.applyScheme(m.Schemes[m.ThemeOverlay.ActiveThemeIdx])
				ui.SaveThemeName(m.Schemes[m.ThemeOverlay.ActiveThemeIdx].Name)
			}
		}
		return m, nil

	case ModeSortOverlay:
		result := m.entitySortOverlay.handleKey(key, m.Keymap)
		if result.applied {
			m.entitySortField = result.field
			m.entitySortDesc = result.desc
			m.applyEntitySort()
		}
		return m, nil

	case ModeTargetPicker:
		return m.updateTargetPicker(msg)

	case ModeActionMenu:
		if selected, act := m.actionMenu.handleKey(key, m.Keymap); selected {
			return m.executeAction(act)
		}
		return m, nil

	case ModeMessagePreview:
		return m.handleViewingMessageKey(msg, key)

	case ModeListFilter:
		return m.handleListFilterKey(msg, key)

	case ModeVisualLine:
		return m.handleVisualLineKey(msg, key)

	case ModeNormal:
		return m.handleNormalKey(msg, key)
	}

	return m, nil
}

func (m Model) handleListFilterKey(msg tea.KeyMsg, key string) (Model, tea.Cmd) {
	switch {
	case ui.ShouldQuit(key, m.Keymap.Quit, true):
		return m, tea.Quit
	case m.Keymap.OpenFocused.Matches(key):
		m.commitFocusedFilter()
		m.Notify(appshell.LevelInfo, fmt.Sprintf("Filter applied for %s", paneName(m.focus)))
		return m, nil
	}
	return m.updateFocusedList(msg)
}

func (m Model) handleVisualLineKey(msg tea.KeyMsg, key string) (Model, tea.Cmd) {
	switch {
	case ui.ShouldQuit(key, m.Keymap.Quit, false):
		return m, tea.Quit
	case m.Keymap.HalfPageDown.Matches(key):
		m.scrollFocusedHalfPage(1)
		return m, nil
	case m.Keymap.HalfPageUp.Matches(key):
		m.scrollFocusedHalfPage(-1)
		return m, nil
	case m.Keymap.VisualSwapAnchor.Matches(key):
		m.swapVisualAnchor()
		m.refreshMessageSelectionDisplay()
		return m, nil
	case m.Keymap.ExitVisualLine.Matches(key):
		m.commitVisualSelection()
		m.visualLineMode = false
		m.visualAnchor = ""
		m.refreshMessageSelectionDisplay()
		m.Notify(appshell.LevelInfo, fmt.Sprintf("Visual mode off. %d marked.", len(m.currentMarks())))
		return m, nil
	}

	m2, cmd := m.updateFocusedList(msg)
	if m.Keymap.BlobVisualMove.Matches(key) && m2.focus == messagesPane && m2.visualLineMode {
		m2.refreshMessageSelectionDisplay()
		m2.Notify(appshell.LevelInfo, fmt.Sprintf("Visual mode on. %d in range.", len(m2.visualSelectionIDs())))
	}
	return m2, cmd
}

func (m Model) handleNormalKey(msg tea.KeyMsg, key string) (Model, tea.Cmd) {
	switch {
	case ui.ShouldQuit(key, m.Keymap.Quit, false):
		return m, tea.Quit
	case m.Keymap.HalfPageDown.Matches(key):
		m.scrollFocusedHalfPage(1)
		return m, nil
	case m.Keymap.HalfPageUp.Matches(key):
		m.scrollFocusedHalfPage(-1)
		return m, nil
	case m.Keymap.NextFocus.Matches(key):
		m.nextFocus()
		return m, nil
	case m.Keymap.PreviousFocus.Matches(key):
		m.previousFocus()
		return m, nil
	case m.Keymap.RefreshScope.Matches(key):
		return m.refresh()
	case m.Keymap.OpenFocused.Matches(key):
		return m.handleEnter()
	case m.Keymap.OpenFocusedAlt.Matches(key):
		return m.handleEnter()
	case m.Keymap.NavigateLeft.Matches(key):
		return m.navigateLeft()
	case m.Keymap.ToggleVisualLine.Matches(key):
		if m.focus == messagesPane {
			m.toggleVisualLineMode()
			return m, nil
		}
	case m.Keymap.ToggleMark.Matches(key):
		if m.focus == messagesPane && m.hasPeekTarget {
			item, ok := m.messageList.SelectedItem().(messageItem)
			if !ok || item.duplicate {
				return m, nil
			}
			marks := m.ensureMarks()
			id := item.message.MessageID
			if _, marked := marks[id]; marked {
				delete(marks, id)
				m.Notify(appshell.LevelInfo, fmt.Sprintf("Unmarked %s (%d marked)", id, len(marks)))
			} else {
				marks[id] = struct{}{}
				m.Notify(appshell.LevelInfo, fmt.Sprintf("Marked %s (%d marked)", id, len(marks)))
			}
			m.refreshMessageSelectionDisplay()
			return m, nil
		}
	case m.Keymap.ActionMenu.Matches(key):
		m.actionMenu.open(m.buildActions())
		return m, nil
	case m.Keymap.ToggleDLQFilter.Matches(key):
		if m.focus == entitiesPane && m.hasNamespace {
			actions := m.buildActions()
			if len(actions) > 0 {
				m.actionMenu.open(actions)
			}
			return m, nil
		}
	case m.Keymap.SubscriptionPicker.Matches(key):
		m.SubOverlay.Open()
		m.startLoading(-1, "Refreshing subscriptions...")
		return m, tea.Batch(m.Spinner.Tick, fetchSubscriptionsCmd(m.service, m.cache.subscriptions, m.Subscriptions))
	case !m.EmbeddedMode && m.Keymap.ToggleThemePicker.Matches(key):
		if !m.ThemeOverlay.Active {
			m.ThemeOverlay.Open()
			return m, nil
		}
	case !m.EmbeddedMode && m.Keymap.ToggleHelp.Matches(key):
		if !m.ThemeOverlay.Active {
			if m.HelpOverlay.Active {
				m.HelpOverlay.Close()
			} else {
				m.HelpOverlay.Open("Azure Service Bus Explorer Help", m.HelpSections())
			}
			return m, nil
		}
	case m.Keymap.Inspect.Matches(key):
		m.toggleInspect()
		return m, nil
	case m.Keymap.BackspaceUp.Matches(key):
		return m.handleBackspace()
	}

	return m.updateFocusedList(msg)
}

func (m *Model) syncPreviewToSelection() {
	item, ok := m.messageList.SelectedItem().(messageItem)
	if !ok {
		m.selectedMessage = servicebus.PeekedMessage{}
		m.messageViewport.SetContent(m.Styles.Muted.Render("(no message selected)"))
		m.messageViewport.GotoTop()
		return
	}
	if item.message.MessageID == m.selectedMessage.MessageID && item.message.MessageID != "" {
		return
	}
	m.selectedMessage = item.message
	m.messageViewport.SetContent(m.Styles.Syntax.HighlightJSON(item.message.FullBody))
	m.messageViewport.GotoTop()
}

func (m Model) handleViewingMessageKey(msg tea.KeyMsg, key string) (Model, tea.Cmd) {
	switch {
	case ui.ShouldQuit(key, m.Keymap.Quit, false):
		return m, tea.Quit
	case m.Keymap.NextFocus.Matches(key):
		m.nextFocus()
		return m, nil
	case m.Keymap.PreviousFocus.Matches(key):
		m.previousFocus()
		return m, nil
	case m.Keymap.MessageBack.Matches(key):
		m.transitionTo(messagesPane)
		m.Notify(appshell.LevelInfo, "Back to message list")
		return m, nil
	}
	var cmd tea.Cmd
	m.messageViewport, cmd = m.messageViewport.Update(msg)
	return m, cmd
}
