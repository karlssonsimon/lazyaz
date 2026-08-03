package sbapp

import (
	"fmt"
	"time"

	"github.com/karlssonsimon/lazyaz/internal/appshell"
	"github.com/karlssonsimon/lazyaz/internal/azure/servicebus"
	"github.com/karlssonsimon/lazyaz/internal/ui"

	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
)

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if paste, ok := msg.(tea.PasteMsg); ok {
		text := paste.String()
		switch {
		case m.SubOverlay.Active:
			m.SubOverlay.PasteText(text, m.Subscriptions)
			return m, nil
		case m.ThemeOverlay.Active:
			m.ThemeOverlay.PasteText(text, m.Schemes)
			return m, nil
		case m.HelpOverlay.Active:
			m.HelpOverlay.PasteText(text)
			return m, nil
		case m.entitySortOverlay.Active:
			m.entitySortOverlay.TypeText(text)
			return m, nil
		case m.targetPicker.active:
			ti := ui.TextInput{Value: m.targetPicker.query, Cursor: m.targetPicker.queryCaret}
			ti.Insert(text)
			m.targetPicker.query = ti.Value
			m.targetPicker.queryCaret = ti.Cursor
			m.targetPicker.refilter()
			return m, nil
		case m.actionMenu.Active:
			m.actionMenu.TypeText(text)
			return m, nil
		case m.copyOverlay.Active:
			m.copyOverlay.TypeText(text)
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
	case messagesReceivedMsg:
		return m.handleMessagesReceived(msg)
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
		// Modals and overlays are keyboard-only; swallow clicks so a
		// double-click can't drill into a list underneath them.
		switch m.inputMode() {
		case ModeNormal, ModeListFilter, ModeVisualLine, ModeMessagePreview:
		default:
			return m, nil
		}
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

	case list.FilterMatchesMsg:
		// Dropped: filtering runs synchronously on each keystroke
		// (ui.SyncFilter). These async results apply last-writer-wins,
		// so a stale keystroke's result could overwrite a newer one.
		return m, nil
	}

	return m.updateFocusedList(msg)
}

func (m Model) updateFocusedList(msg tea.Msg) (Model, tea.Cmd) {
	var cmd tea.Cmd
	switch m.focus {
	case namespacesPane:
		cmd = ui.UpdateListSyncFilter(&m.namespacesList, msg)
	case entitiesPane:
		cmd = ui.UpdateListSyncFilter(&m.entitiesList, msg)
	case subscriptionsPane:
		cmd = ui.UpdateListSyncFilter(&m.subscriptionsList, msg)
	case queueTypePane:
		cmd = ui.UpdateListSyncFilter(&m.queueTypeList, msg)
	case messagesPane:
		cmd = ui.UpdateListSyncFilter(&m.messageList, msg)
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
	matched, status, selectPreferred, cmd := m.Model.HandleSubscriptionsLoaded(msg, m.cache.subscriptions)
	if !selectPreferred {
		return m, cmd
	}
	next, selectCmd := m.selectSubscription(matched)
	next.ClearLoading()
	next.ResolveSpinner(next.LoadingSpinnerID, appshell.LevelSuccess, status)
	return next, selectCmd
}

func (m Model) handleNamespacesLoaded(msg namespacesLoadedMsg) (Model, tea.Cmd) {
	if !m.HasSubscription || m.CurrentSub.ID != msg.subscriptionID {
		return m, nil
	}
	if msg.err != nil {
		m.ClearLoading()
		m.ResolveSpinner(m.LoadingSpinnerID, appshell.LevelError, fmt.Sprintf("Failed to load namespaces in %s: %s", ui.SubscriptionDisplayName(m.CurrentSub), msg.err.Error()))
		return m, nil
	}

	m.namespaces = msg.namespaces
	m.namespacesList.Title = fmt.Sprintf("Namespaces (%d)", len(m.namespaces))
	ui.SetItemsPreserveKey(&m.namespacesList, namespacesToItems(m.namespaces), namespaceItemKey)

	if msg.done {
		status := fmt.Sprintf("Loaded %d namespaces from %s in %s", len(m.namespaces), ui.SubscriptionDisplayName(m.CurrentSub), time.Since(m.LoadingStartedAt).Round(time.Millisecond))
		m.ClearLoading()
		m.ResolveSpinner(m.LoadingSpinnerID, appshell.LevelSuccess, status)
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
		m.ResolveSpinner(m.LoadingSpinnerID, appshell.LevelError, fmt.Sprintf("Failed to load entities in %s: %s", msg.namespace.Name, msg.err.Error()))
		return m, nil
	}

	m.entities = msg.entities
	m.rebuildEntitiesItems()
	m.entitiesList.Title = m.entitiesPaneTitle()

	if msg.done {
		status := fmt.Sprintf("Loaded %d entities from %s in %s", len(m.entities), msg.namespace.Name, time.Since(m.LoadingStartedAt).Round(time.Millisecond))
		m.ClearLoading()
		m.ResolveSpinner(m.LoadingSpinnerID, appshell.LevelSuccess, status)
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
		m.ResolveSpinner(m.LoadingSpinnerID, appshell.LevelError, fmt.Sprintf("Failed to load subscriptions for topic %s: %s", msg.topicName, msg.err.Error()))
		return m, nil
	}

	m.subscriptions = msg.subs
	ui.SetItemsPreserveKey(&m.subscriptionsList, subscriptionsToItems(msg.subs), subscriptionItemKey)
	m.subscriptionsList.Title = fmt.Sprintf("Subscriptions (%d)", len(msg.subs))

	if msg.done {
		status := fmt.Sprintf("Loaded %d subscriptions for topic %s in %s", len(m.subscriptions), msg.topicName, time.Since(m.LoadingStartedAt).Round(time.Millisecond))
		m.ClearLoading()
		m.ResolveSpinner(m.LoadingSpinnerID, appshell.LevelSuccess, status)
		updated, navCmd := m.advancePendingNav()
		return updated, navCmd
	}
	return m, msg.next
}

func (m Model) handleMessagesLoaded(msg messagesLoadedMsg) (Model, tea.Cmd) {
	// Drop results for a scope the user has already left — without this,
	// an in-flight peek lands in whatever queue/sub is now current.
	if !m.hasPeekTarget || msg.namespace.Name != m.currentNS.Name ||
		msg.entityName != m.currentEntity.Name || msg.subName != m.currentSubName ||
		msg.deadLetter != m.deadLetter {
		return m, nil
	}
	if msg.err != nil {
		m.ClearLoading()
		m.ResolveSpinner(m.LoadingSpinnerID, appshell.LevelError, fmt.Sprintf("Failed to peek messages from %s: %s", msg.source, msg.err.Error()))
		return m, nil
	}

	if msg.repeek {
		// Append mode ("peek more") — add to existing messages.
		m.peekedMessages = append(m.peekedMessages, msg.messages...)
	} else {
		m.peekedMessages = msg.messages
	}
	items := m.messageItems()
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
	m.ResolveSpinner(m.LoadingSpinnerID, appshell.LevelSuccess, fmt.Sprintf("Peeked %d %s messages from %s", len(msg.messages), label, msg.source))
	return m, nil
}

func (m Model) handleEntitiesRefreshed(msg entitiesRefreshedMsg) (Model, tea.Cmd) {
	if !m.hasNamespace || msg.namespace.Name != m.currentNS.Name {
		return m, nil
	}
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

func (m Model) handleMessagesReceived(msg messagesReceivedMsg) (Model, tea.Cmd) {
	// Stale receive: the user navigated away (or switched between the
	// active queue and the DLQ) while the lock receive was in flight.
	// Release the locks in the background instead of installing them
	// against whatever scope is now current.
	if !m.hasPeekTarget || m.deadLetter != msg.deadLetter || msg.namespace.Name != m.currentNS.Name ||
		msg.entityName != m.currentEntity.Name || msg.subName != m.currentSubName {
		closeLockedAsync(msg.result)
		return m, nil
	}
	scope := "active"
	title := "Locked"
	if msg.deadLetter {
		scope = "DLQ"
		title = "DLQ Locked"
	}
	m.ClearLoading()
	if msg.err != nil {
		m.ResolveSpinner(m.LoadingSpinnerID, appshell.LevelError, fmt.Sprintf("Failed to receive %s messages: %s", scope, msg.err.Error()))
		return m, nil
	}

	m.lockedMessages = msg.result
	m.peekedMessages = msg.result.PeekedMessages()
	m.migrateMarksToLocks()

	m.messageList.ResetFilter()
	m.messageList.SetItems(m.messageItems())
	if len(m.peekedMessages) > 0 {
		m.messageList.Select(0)
	}
	m.messageList.Title = fmt.Sprintf("%s (%d)", title, len(m.peekedMessages))
	m.resize()
	m.ResolveSpinner(m.LoadingSpinnerID, appshell.LevelSuccess, fmt.Sprintf("Received %d %s messages with lock", len(m.peekedMessages), scope))
	return m, nil
}

func (m Model) handleDLQComplete(msg dlqCompleteMsg) (Model, tea.Cmd) {
	m.ClearLoading()
	if msg.err != nil {
		partial := ""
		if len(msg.completed) > 0 {
			partial = fmt.Sprintf(" (%d completed before error)", len(msg.completed))
		}
		m.ResolveSpinner(m.LoadingSpinnerID, appshell.LevelError, fmt.Sprintf("Failed to complete messages%s: %s", partial, msg.err.Error()))
	} else {
		queueWord := "queue"
		if m.deadLetter {
			queueWord = "DLQ"
		}
		m.ResolveSpinner(m.LoadingSpinnerID, appshell.LevelSuccess, fmt.Sprintf("Completed %d message(s) (removed from %s)", len(msg.completed), queueWord))
	}
	// Only mutate the view when the result belongs to the lock session
	// still on screen (see handleDLQAbandon).
	if msg.locked == m.lockedMessages {
		for _, id := range msg.completed {
			m.removeLockedMessage(id)
		}
		m.clearScopeMarks()
		m.refreshMessageSelectionDisplay()
	}
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
		m.ResolveSpinner(m.LoadingSpinnerID, appshell.LevelError, fmt.Sprintf("Failed to requeue messages%s: %s", partial, msg.err.Error()))
	} else {
		m.ResolveSpinner(m.LoadingSpinnerID, appshell.LevelSuccess, fmt.Sprintf("Requeued %d message(s) to active queue", len(msg.requeued)))
	}
	// Only mutate the view when the result belongs to the lock session
	// still on screen — a late result from a scope the user has left
	// must not touch the current one.
	if msg.locked == m.lockedMessages {
		for _, id := range msg.requeued {
			m.removeLockedMessage(id)
		}
		m.clearScopeMarks()
		m.refreshMessageSelectionDisplay()
	}
	if len(msg.requeued) > 0 && m.hasNamespace {
		return m, refreshEntitiesCmd(m.service, m.currentNS)
	}
	return m, nil
}

func (m Model) handleDLQAbandon(msg dlqAbandonMsg) (Model, tea.Cmd) {
	m.ClearLoading()
	// Abandon also fires when the user navigates away from a locked
	// view (abandonLockedIfHeld nils lockedMessages first). Only clear
	// the pane when the released session is the one still displayed —
	// otherwise this late result would wipe whatever the user opened
	// since, including a fresh lock session on another entity.
	if msg.locked == m.lockedMessages {
		m.lockedMessages = nil // receiver already closed by the command
		m.peekedMessages = nil
		m.messageList.ResetFilter()
		m.messageList.SetItems(nil)
		m.messageList.Title = "Messages"
		if m.viewingMessage {
			m.closePreview()
		}
	}
	if msg.err != nil {
		m.ResolveSpinner(m.LoadingSpinnerID, appshell.LevelError, fmt.Sprintf("Failed to abandon: %s", msg.err.Error()))
	} else {
		m.ResolveSpinner(m.LoadingSpinnerID, appshell.LevelSuccess, "Locks released")
	}
	return m, nil
}

// removeLockedMessage removes a processed message from both the locked
// set and the display list.
func (m *Model) removeLockedMessage(messageKey string) {
	if m.lockedMessages == nil {
		return
	}

	m.lockedMessages.RemoveByID(messageKey)

	// Remove from peeked display.
	var peeked []servicebus.PeekedMessage
	for _, pm := range m.peekedMessages {
		if messageOperationKey(pm) != messageKey {
			peeked = append(peeked, pm)
		}
	}
	m.peekedMessages = peeked
	m.messageList.SetItems(m.messageItems())
	m.messageList.Title = fmt.Sprintf("DLQ Locked (%d)", len(m.peekedMessages))

	// If no more locked messages, clean up the receiver. Close in a
	// goroutine — it's a network call and this runs on the UI thread
	// (same pattern as clearLockedMessages).
	if m.lockedMessages.Len() == 0 {
		locked := m.lockedMessages
		m.lockedMessages = nil
		closeLockedAsync(locked)
	}
}

func (m Model) handleDLQRequeueAll(msg dlqRequeueAllMsg) (Model, tea.Cmd) {
	m.ClearLoading()
	if msg.err != nil {
		partial := ""
		if msg.requeued > 0 {
			partial = fmt.Sprintf(" (%d requeued before error)", msg.requeued)
		}
		m.ResolveSpinner(m.LoadingSpinnerID, appshell.LevelError, fmt.Sprintf("Failed to requeue DLQ messages%s: %s", partial, msg.err.Error()))
	} else {
		m.ResolveSpinner(m.LoadingSpinnerID, appshell.LevelSuccess, fmt.Sprintf("Requeued all %d DLQ messages to active queue", msg.requeued))
	}
	// Refresh entity counts to reflect the change.
	if m.hasNamespace {
		return m, refreshEntitiesCmd(m.service, m.currentNS)
	}
	return m, nil
}

func (m Model) handleMoveAllDone(msg moveAllDoneMsg) (Model, tea.Cmd) {
	m.ClearLoading()
	kind := "active"
	if msg.deadLetter {
		kind = "DLQ"
	}
	if msg.err != nil {
		partial := ""
		if msg.moved > 0 {
			partial = fmt.Sprintf(" (%d moved before error)", msg.moved)
		}
		m.ResolveSpinner(m.LoadingSpinnerID, appshell.LevelError, fmt.Sprintf("Failed to move %s messages%s: %s", kind, partial, msg.err.Error()))
	} else {
		m.ResolveSpinner(m.LoadingSpinnerID, appshell.LevelSuccess, fmt.Sprintf("Moved all %d %s messages", msg.moved, kind))
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
		m.ResolveSpinner(m.LoadingSpinnerID, appshell.LevelError, fmt.Sprintf("Failed to move messages%s: %s", partial, msg.err.Error()))
	} else {
		m.ResolveSpinner(m.LoadingSpinnerID, appshell.LevelSuccess, fmt.Sprintf("Moved %d message(s)", len(msg.moved)))
	}
	// Only mutate the view when the result belongs to the lock session
	// still on screen (see handleDLQAbandon).
	if msg.locked == m.lockedMessages {
		for _, id := range msg.moved {
			m.removeLockedMessage(id)
		}
		m.clearScopeMarks()
		m.refreshMessageSelectionDisplay()
	}
	if len(msg.moved) > 0 && m.hasNamespace {
		return m, refreshEntitiesCmd(m.service, m.currentNS)
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	key := msg.String()

	switch m.inputMode() {
	case ModeConfirm:
		switch m.confirmModal.HandleKey(key) {
		case ui.ConfirmActionConfirm:
			act := m.confirmAction
			m.confirmAction = nil
			if act != nil {
				return act(m)
			}
			return m, nil
		case ui.ConfirmActionCancel:
			m.confirmAction = nil
			return m, nil
		}
		return m, nil

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

	case ModeCopyPalette:
		if target, picked := m.copyOverlay.HandleKey(key, m.Keymap); picked {
			return m.copyToClipboard(target.Value)
		}
		return m, nil

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
	case m.scrollMotion(key):
		return m, nil
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

	// Cursor movement falls through to the list; refresh the highlight
	// whenever the cursor actually moved so custom cursor bindings work
	// too (the old check matched only the stock movement keys).
	before := m.messageList.Index()
	m2, cmd := m.updateFocusedList(msg)
	if m2.focus == messagesPane && m2.visualLineMode && m2.messageList.Index() != before {
		m2.refreshMessageSelectionDisplay()
		m2.Notify(appshell.LevelInfo, fmt.Sprintf("Visual mode on. %d in range.", len(m2.visualSelectionIDs())))
	}
	return m2, cmd
}

func (m Model) handleNormalKey(msg tea.KeyMsg, key string) (Model, tea.Cmd) {
	// Esc peels selection state like a stack on the messages pane:
	//  1. If the bubbles list has an applied filter → clear it.
	//  2. If marks exist → clear them (ExitVisualLine case below).
	//  3. Otherwise fall through.
	if m.Keymap.Cancel.Matches(key) && m.focus == messagesPane {
		if m.messageList.FilterState() != list.Unfiltered {
			m.messageList.ResetFilter()
			return m, nil
		}
	}

	switch {
	case ui.ShouldQuit(key, m.Keymap.Quit, false):
		return m, tea.Quit
	case m.scrollMotion(key):
		return m, nil
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
			if !ok {
				return m, nil
			}
			marks := m.ensureMarks()
			id := messageOperationKey(item.message)
			if _, marked := marks[id]; marked {
				delete(marks, id)
				m.Notify(appshell.LevelInfo, fmt.Sprintf("Unmarked %s (%d marked)", ui.EmptyToDash(item.message.MessageID), len(marks)))
			} else {
				marks[id] = struct{}{}
				m.Notify(appshell.LevelInfo, fmt.Sprintf("Marked %s (%d marked)", ui.EmptyToDash(item.message.MessageID), len(marks)))
			}
			m.refreshMessageSelectionDisplay()
			return m, nil
		}
	case m.Keymap.ExitVisualLine.Matches(key):
		if m.focus == messagesPane && len(m.currentMarks()) > 0 {
			count := len(m.currentMarks())
			m.clearScopeMarks()
			m.refreshMessageSelectionDisplay()
			m.Notify(appshell.LevelInfo, fmt.Sprintf("Cleared %d marks", count))
			return m, nil
		}
	case m.Keymap.YankMessageBody.Matches(key):
		if m.focus == messagesPane {
			item, ok := m.messageList.SelectedItem().(messageItem)
			if !ok {
				return m, nil
			}
			return m.yankMessageBody(item.message.FullBody)
		}
	case m.Keymap.CopyPalette.Matches(key):
		return m.openCopyPalette()
	case m.Keymap.ActionMenu.Matches(key):
		m.actionMenu.open(m.buildActions())
		return m, nil
	case m.Keymap.ToggleDLQFilter.Matches(key):
		// "s" on the entities pane opens the entity sort overlay.
		// (Keymap field name is stale — historically toggled a DLQ filter,
		// now repurposed for sort. The action menu still exposes "Sort
		// entities" with the same shortcut for discoverability.)
		if m.focus == entitiesPane && m.hasNamespace {
			m.entitySortOverlay.open(m.entitySortField, m.entitySortDesc)
			return m, nil
		}
	// The guard lives in the case condition so the key can fall through
	// to ReloadSubscriptions when both share a chord (standard keymap
	// binds ctrl+r to both) and the DLQ context doesn't apply.
	case m.Keymap.RequeueDLQ.Matches(key) && m.focus == messagesPane && m.deadLetter:
		if m.lockedMessages == nil {
			m.Notify(appshell.LevelInfo, "Receive DLQ messages with lock first")
			return m, nil
		}
		return m.openRequeueConfirm()
	case m.Keymap.SubscriptionPicker.Matches(key):
		m.SubOverlay.Open()
		m.StartLoading(-1, "Refreshing subscriptions...")
		return m, tea.Batch(m.Spinner.Tick, appshell.FetchSubscriptionsCmd(m.service, m.cache.subscriptions, m.Tenant, m.Subscriptions))
	case m.Keymap.ReloadSubscriptions.Matches(key):
		m.SubOverlay.Open()
		m.StartLoading(-1, "Refreshing subscriptions...")
		return m, tea.Batch(m.Spinner.Tick, appshell.FetchSubscriptionsCmd(m.service, m.cache.subscriptions, m.Tenant, m.Subscriptions))
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
	if messageOperationKey(item.message) == messageOperationKey(m.selectedMessage) && messageOperationKey(item.message) != "" {
		return
	}
	m.selectedMessage = item.message
	m.messageViewport.SetContent(m.Styles.Syntax.HighlightJSON(item.message.FullBody))
	m.messageViewport.GotoTop()
}

// rehighlightSelectedMessage re-renders the message body against the
// current styles. HighlightJSON bakes colors into the string when the
// selection changes, so without this the detail pane keeps painting the
// previous theme until a different message is picked. Scroll position
// is preserved — a theme switch shouldn't move the reader.
func (m *Model) rehighlightSelectedMessage() {
	if _, ok := m.messageList.SelectedItem().(messageItem); !ok {
		m.messageViewport.SetContent(m.Styles.Muted.Render("(no message selected)"))
		return
	}
	offset := m.messageViewport.YOffset()
	m.messageViewport.SetContent(m.Styles.Syntax.HighlightJSON(m.selectedMessage.FullBody))
	m.messageViewport.SetYOffset(offset)
}

func (m Model) handleViewingMessageKey(msg tea.KeyMsg, key string) (Model, tea.Cmd) {
	// The search prompt owns every key while it is open, so a query can
	// contain characters that are otherwise message-body bindings.
	if cmd, consumed := m.handleMessageSearchKey(key); consumed {
		m.pendingMessageG = false
		return m, cmd
	}

	switch {
	case ui.ShouldQuit(key, m.Keymap.Quit, false):
		return m, tea.Quit
	case m.Keymap.NextFocus.Matches(key):
		m.pendingMessageG = false
		m.nextFocus()
		return m, nil
	case m.Keymap.PreviousFocus.Matches(key):
		m.pendingMessageG = false
		m.previousFocus()
		return m, nil
	case m.Keymap.ActionMenu.Matches(key):
		m.pendingMessageG = false
		m.actionMenu.open(m.buildActions())
		return m, nil
	case m.Keymap.YankMessageBody.Matches(key):
		m.pendingMessageG = false
		return m.yankMessageBody(m.selectedMessage.FullBody)
	case m.Keymap.CopyPalette.Matches(key):
		m.pendingMessageG = false
		return m.openCopyPalette()
	case m.Keymap.JumpBottom.Matches(key):
		m.pendingMessageG = false
		m.messageViewport.GotoBottom()
		return m, nil
	case m.Keymap.FullPageDown.Matches(key):
		m.pendingMessageG = false
		m.messageViewport.PageDown()
		return m, nil
	case m.Keymap.FullPageUp.Matches(key):
		m.pendingMessageG = false
		m.messageViewport.PageUp()
		return m, nil
	// The message body scrolls without a cursor, so ctrl+e / ctrl+y move
	// the view a single line — the plain vim meaning.
	case m.Keymap.ScrollLineDown.Matches(key):
		m.pendingMessageG = false
		m.messageViewport.ScrollDown(1)
		return m, nil
	case m.Keymap.ScrollLineUp.Matches(key):
		m.pendingMessageG = false
		m.messageViewport.ScrollUp(1)
		return m, nil
	case m.Keymap.JumpTopPrefix.Matches(key):
		// Home jumps immediately; bare g keeps the gg chord.
		if key == "home" || m.pendingMessageG {
			m.pendingMessageG = false
			m.messageViewport.GotoTop()
			return m, nil
		}
		m.pendingMessageG = true
		m.Notify(appshell.LevelInfo, "Press g again for top")
		return m, nil
	case m.Keymap.MessageBack.Matches(key):
		m.pendingMessageG = false
		m.transitionTo(messagesPane)
		m.Notify(appshell.LevelInfo, "Back to message list")
		return m, nil
	}
	m.pendingMessageG = false
	var cmd tea.Cmd
	m.messageViewport, cmd = m.messageViewport.Update(msg)
	return m, cmd
}

// yankMessageBody copies a message body to the clipboard via the async
// clipboardMsg flow shared with mouse text selection. Empty bodies
// short-circuit with an info toast instead of clobbering the clipboard.
func (m Model) yankMessageBody(body string) (Model, tea.Cmd) {
	if body == "" {
		m.Notify(appshell.LevelInfo, "Message body is empty")
		return m, nil
	}
	return m, func() tea.Msg {
		if err := ui.WriteClipboard(body); err != nil {
			return clipboardMsg{err: err}
		}
		return clipboardMsg{text: body}
	}
}
