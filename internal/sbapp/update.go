package sbapp

import (
	"errors"
	"fmt"
	"time"

	"github.com/karlssonsimon/lazyaz/internal/appshell"
	"github.com/karlssonsimon/lazyaz/internal/azure/servicebus"
	"github.com/karlssonsimon/lazyaz/internal/cache"
	"github.com/karlssonsimon/lazyaz/internal/ui"

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
		m.Notify(appshell.LevelError, fmt.Sprintf("Failed to load subscriptions: %s", msg.Err.Error()))
		return m, nil
	}

	m.LastErr = ""
	m.Subscriptions = msg.Subscriptions
	// Keep the overlay's filtered view in sync with streaming results
	// so new subscriptions matching the user's query appear immediately.
	if m.SubOverlay.Active {
		m.SubOverlay.Refilter(m.Subscriptions)
	}

	if msg.Done {
		m.cache.subscriptions.Set("", msg.Subscriptions)
		status := fmt.Sprintf("Loaded %d subscriptions in %s", len(msg.Subscriptions), time.Since(m.LoadingStartedAt).Round(time.Millisecond))
		if !m.HasSubscription {
			if matched, ok := m.TryApplyPreferredSubscription(); ok {
				// The constructor opened the picker overlay; selectSubscription
				// drives navigation but doesn't dismiss it (the interactive
				// path is dismissed inside the overlay's HandleKey). Close
				// it here so the data loading behind it actually shows.
				m.SubOverlay.Close()
				next, selectCmd := m.selectSubscription(matched)
				return next, tea.Batch(selectCmd, next.FinishLoading(status))
			}
			m.SubOverlay.Open()
		}
		return m, m.FinishLoading(status)
	}

	return m, msg.Next
}

func (m Model) handleNamespacesLoaded(msg namespacesLoadedMsg) (Model, tea.Cmd) {
	if !m.HasSubscription || m.CurrentSub.ID != msg.subscriptionID {
		return m, nil
	}
	if m.namespacesSession == nil || m.namespacesSession.Gen() != msg.gen {
		return m, nil
	}

	if msg.err != nil {
		m.ClearLoading()
		m.Notify(appshell.LevelError, fmt.Sprintf("Failed to load namespaces in %s: %s", ui.SubscriptionDisplayName(m.CurrentSub), msg.err.Error()))
		m.namespacesSession = nil
		return m, nil
	}

	m.LastErr = ""
	m.namespacesSession.Apply(msg.namespaces)
	m.namespaces = m.namespacesSession.Items()
	m.namespacesList.Title = fmt.Sprintf("Namespaces (%d)", len(m.namespaces))
	ui.SetItemsPreserveKey(&m.namespacesList, namespacesToItems(m.namespaces), namespaceItemKey)

	if msg.done {
		m.namespaces = m.namespacesSession.Finalize()
		m.namespacesSession = nil
		m.cache.namespaces.Set(msg.subscriptionID, m.namespaces)
		m.namespacesList.Title = fmt.Sprintf("Namespaces (%d)", len(m.namespaces))
		ui.SetItemsPreserveKey(&m.namespacesList, namespacesToItems(m.namespaces), namespaceItemKey)
		status := fmt.Sprintf("Loaded %d namespaces from %s in %s", len(m.namespaces), ui.SubscriptionDisplayName(m.CurrentSub), time.Since(m.LoadingStartedAt).Round(time.Millisecond))
		return m, m.FinishLoading(status)
	}

	return m, msg.next
}

func (m Model) handleEntitiesLoaded(msg entitiesLoadedMsg) (Model, tea.Cmd) {
	if !m.hasNamespace || m.currentNS.Name != msg.namespace.Name {
		return m, nil
	}
	if m.entitiesSession == nil || m.entitiesSession.Gen() != msg.gen {
		return m, nil
	}

	if msg.err != nil {
		m.ClearLoading()
		m.Notify(appshell.LevelError, fmt.Sprintf("Failed to load entities in %s: %s", msg.namespace.Name, msg.err.Error()))
		m.entitiesSession = nil
		return m, nil
	}

	m.LastErr = ""
	m.entitiesSession.Apply(msg.entities)
	m.entities = m.entitiesSession.Items()
	m.rebuildEntitiesItems()
	m.entitiesList.Title = m.entitiesPaneTitle()

	if msg.done {
		m.entities = m.entitiesSession.Finalize()
		m.entitiesSession = nil
		m.cache.entities.Set(cache.Key(m.CurrentSub.ID, msg.namespace.Name), m.entities)
		m.rebuildEntitiesItems()
		m.entitiesList.Title = m.entitiesPaneTitle()
		status := fmt.Sprintf("Loaded %d entities from %s in %s", len(m.entities), msg.namespace.Name, time.Since(m.LoadingStartedAt).Round(time.Millisecond))
		return m, m.FinishLoading(status)
	}

	return m, msg.next
}

func (m Model) handleTopicSubscriptionsLoaded(msg topicSubscriptionsLoadedMsg) (Model, tea.Cmd) {
	// Drop pages for stale fetches: scope changed, topic changed, or
	// session was abandoned (e.g. user expanded a different topic).
	if !m.hasNamespace || m.currentNS.Name != msg.namespace.Name {
		return m, nil
	}
	if m.topicSubsSession == nil || m.topicSubsSession.Gen() != msg.gen || m.topicSubsFetching != msg.topicName {
		return m, nil
	}
	if !m.expandedTopics[msg.topicName] {
		// User collapsed the topic before the fetch finished — drop.
		return m, nil
	}

	if msg.err != nil {
		m.ClearLoading()
		m.Notify(appshell.LevelError, fmt.Sprintf("Failed to load subscriptions for topic %s: %s", msg.topicName, msg.err.Error()))
		m.topicSubsSession = nil
		m.topicSubsFetching = ""
		return m, nil
	}

	m.LastErr = ""
	m.topicSubsSession.Apply(msg.subs)
	m.topicSubsByTopic[msg.topicName] = m.topicSubsSession.Items()
	m.rebuildEntitiesItems()

	if msg.done {
		m.topicSubsByTopic[msg.topicName] = m.topicSubsSession.Finalize()
		m.topicSubsSession = nil
		m.topicSubsFetching = ""
		m.cache.topicSubs.Set(cache.Key(m.CurrentSub.ID, msg.namespace.Name, msg.topicName), m.topicSubsByTopic[msg.topicName])
		m.rebuildEntitiesItems()
		status := fmt.Sprintf("Loaded %d subscriptions for topic %s in %s", len(m.topicSubsByTopic[msg.topicName]), msg.topicName, time.Since(m.LoadingStartedAt).Round(time.Millisecond))
		return m, m.FinishLoading(status)
	}

	return m, msg.next
}

func (m Model) handleMessagesLoaded(msg messagesLoadedMsg) (Model, tea.Cmd) {
	// Messages are ephemeral peek results — not cached.
	m.ClearLoading()
	if msg.err != nil {
		m.Notify(appshell.LevelError, fmt.Sprintf("Failed to peek messages from %s: %s", msg.source, msg.err.Error()))
		return m, nil
	}

	m.LastErr = ""
	m.peekedMessages = msg.messages
	items := messagesToItems(msg.messages, m.currentMarks(), m.currentDuplicates())
	if msg.repeek {
		// Same scope (after requeue, delete-duplicate, or R-key refresh):
		// keep the cursor on whichever message the user was looking at,
		// or clamp to the same numeric index if it's gone.
		ui.SetItemsPreserveKey(&m.detailList, items, messageItemKey)
	} else {
		// Fresh items (entering an entity or active↔DLQ toggle): clear
		// filter and start at the top. The preview pane is NOT torn
		// down here — peekQueue/peekTopicSub close it explicitly when
		// entering a different entity, and DLQ toggle within the same
		// entity should keep the preview open and just have it follow
		// the new selection.
		m.detailList.ResetFilter()
		m.detailList.SetItems(items)
		if len(msg.messages) > 0 {
			m.detailList.Select(0)
		}
	}
	if m.viewingMessage {
		// Force the preview pane to resync to the (possibly new)
		// selected message. Clearing selectedMessage makes
		// syncPreviewToSelection treat the current selection as a change.
		m.selectedMessage = servicebus.PeekedMessage{}
		m.syncPreviewToSelection()
	}
	m.detailList.Title = fmt.Sprintf("Messages (%d)", len(msg.messages))
	m.resize()
	m.Status = fmt.Sprintf("Peeked %d messages from %s", len(msg.messages), msg.source)
	return m, nil
}

func (m Model) handleRequeueDone(msg requeueDoneMsg) (Model, tea.Cmd) {
	m.ClearLoading()
	// Successful requeues consume the marks for the current scope.
	m.clearScopeMarks()
	switch {
	case msg.err != nil:
		var dupErr *servicebus.DuplicateError
		if errors.As(msg.err, &dupErr) {
			m.ensureDuplicates()[dupErr.MessageID] = struct{}{}
			m.Notify(appshell.LevelWarn, fmt.Sprintf("Message %s sent but not removed from DLQ (possible duplicate)", dupErr.MessageID))
		} else {
			m.Notify(appshell.LevelError, fmt.Sprintf("Failed to requeue messages: %s", msg.err.Error()))
		}
	case msg.requeued > 0:
		m.Notify(appshell.LevelSuccess, fmt.Sprintf("%d of %d message(s) requeued", msg.requeued, msg.total))
	default:
		m.Notify(appshell.LevelError, "Failed to requeue messages")
	}
	var peekCmd tea.Cmd
	m, peekCmd = m.rePeekMessages(true)
	return m, tea.Batch(peekCmd, refreshEntitiesCmd(m.service, m.currentNS))
}

func (m Model) handleDeleteDuplicateDone(msg deleteDuplicateDoneMsg) (Model, tea.Cmd) {
	m.ClearLoading()
	if msg.err != nil {
		m.Notify(appshell.LevelError, fmt.Sprintf("Failed to delete duplicate message: %s", msg.err.Error()))
		return m, nil
	}
	if dups := m.currentDuplicates(); dups != nil {
		delete(dups, msg.messageID)
	}
	m.Notify(appshell.LevelSuccess, "Duplicate message deleted")
	var peekCmd tea.Cmd
	m, peekCmd = m.rePeekMessages(true)
	return m, tea.Batch(peekCmd, refreshEntitiesCmd(m.service, m.currentNS))
}

func (m Model) handleEntitiesRefreshed(msg entitiesRefreshedMsg) (Model, tea.Cmd) {
	if msg.err != nil {
		return m, nil
	}
	m.entities = msg.entities
	m.rebuildEntitiesItems()
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

	// Message preview viewport captures keys only when it has focus.
	// When focus is on the message list (with the preview still open),
	// keys flow through normally so the user can navigate messages while
	// the preview pane updates live.
	if m.viewingMessage && m.focus == messagePreviewPane {
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
		if !focusedFilterActive && m.focus == detailPane && m.hasPeekTarget {
			item, ok := m.detailList.SelectedItem().(messageItem)
			if !ok {
				return m, nil
			}
			if item.duplicate {
				return m, nil
			}
			marks := m.ensureMarks()
			id := item.message.MessageID
			if _, marked := marks[id]; marked {
				delete(marks, id)
				m.Status = fmt.Sprintf("Unmarked %s (%d marked)", id, len(marks))
			} else {
				marks[id] = struct{}{}
				m.Status = fmt.Sprintf("Marked %s (%d marked)", id, len(marks))
			}
			m.refreshItems()
			return m, nil
		}
	case m.Keymap.ShowActiveQueue.Matches(key):
		if !focusedFilterActive && m.focus == detailPane && m.hasPeekTarget {
			if m.deadLetter {
				m.deadLetter = false
				m.clearDetailListForRePeek()
				return m.rePeekMessages(false)
			}
			return m, nil
		}
		// Same key cycles entity-type tabs when focused on the entities pane.
		if !focusedFilterActive && m.focus == entitiesPane {
			m.cycleEntityFilter(-1)
			return m, nil
		}
	case m.Keymap.ShowDeadLetterQueue.Matches(key):
		if !focusedFilterActive && m.focus == detailPane && m.hasPeekTarget {
			if !m.deadLetter {
				m.deadLetter = true
				m.clearDetailListForRePeek()
				return m.rePeekMessages(false)
			}
			return m, nil
		}
		if !focusedFilterActive && m.focus == entitiesPane {
			m.cycleEntityFilter(1)
			return m, nil
		}
	case m.Keymap.RequeueDLQ.Matches(key):
		if !focusedFilterActive && m.focus == detailPane && m.hasPeekTarget && m.deadLetter {
			messageIDs := m.collectRequeueIDs()
			if len(messageIDs) == 0 {
				return m, nil
			}
			m.SetLoading(m.focus)
			m.LastErr = ""
			m.Status = fmt.Sprintf("Requeuing %d message(s)...", len(messageIDs))
			return m, tea.Batch(spinner.Tick, requeueMessagesCmd(m.service, m.currentNS, m.currentEntity, m.currentSubName, messageIDs))
		}
	case m.Keymap.DeleteDuplicate.Matches(key):
		if !focusedFilterActive && m.focus == detailPane && m.hasPeekTarget && m.deadLetter {
			item, ok := m.detailList.SelectedItem().(messageItem)
			if !ok || !item.duplicate {
				return m, nil
			}
			m.SetLoading(m.focus)
			m.LastErr = ""
			m.Status = "Deleting duplicate message..."
			return m, tea.Batch(spinner.Tick, deleteDuplicateCmd(m.service, m.currentNS, m.currentEntity, m.currentSubName, item.message.MessageID))
		}
	case m.Keymap.ToggleDLQFilter.Matches(key):
		if !focusedFilterActive {
			m.dlqSort = !m.dlqSort
			m.applyDLQSort()
			if m.dlqSort {
				m.Status = "DLQ-first sort enabled – entities with dead-letter messages on top"
			} else {
				m.Status = "DLQ-first sort disabled – entities in default order"
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
			m.toggleInspect()
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
		// If the preview pane is mounted, sync its content to whatever
		// message the cursor just landed on. This makes the preview
		// follow the cursor live.
		if m.viewingMessage {
			m.syncPreviewToSelection()
		}
	}
	return m, cmd
}

// syncPreviewToSelection refreshes the message preview viewport to
// match whatever the message list cursor currently points at. Called
// after detail-list cursor movement to make the preview pane follow
// the cursor live. When the list is empty (e.g. switching to a DLQ
// tab with zero messages), the viewport is cleared to a placeholder
// instead of showing stale content from the previous selection.
func (m *Model) syncPreviewToSelection() {
	item, ok := m.detailList.SelectedItem().(messageItem)
	if !ok {
		// Nothing to show — clear the viewport so the user sees a
		// clean placeholder instead of the previously selected body.
		m.selectedMessage = servicebus.PeekedMessage{}
		m.messageViewport.SetContent(m.Styles.Muted.Render("(no message selected)"))
		m.messageViewport.GotoTop()
		return
	}
	if item.message.MessageID == m.selectedMessage.MessageID && item.message.MessageID != "" {
		return // no change
	}
	m.selectedMessage = item.message
	m.messageViewport.SetContent(m.Styles.Syntax.HighlightJSON(item.message.FullBody))
	m.messageViewport.GotoTop()
}

// handleViewingMessageKey routes key events while the message-preview
// pane has focus. Quit, focus changes, and the back binding are
// intercepted; everything else scrolls the viewport.
//
// The back binding (h/left/backspace/esc) just moves focus to the
// message list without closing the preview — the preview pane stays
// mounted and follows the cursor as the user navigates messages. The
// preview is only torn down when the user actually leaves the entity
// (peeking a different one, going back to the entities pane, etc.).
//
// Note: this handler runs only when m.focus == messagePreviewPane, so
// the message list is reachable normally via Tab/Shift+Tab without
// being trapped here.
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
		m.focus = detailPane
		m.Status = "Back to message list"
		return m, nil
	}
	var cmd tea.Cmd
	m.messageViewport, cmd = m.messageViewport.Update(msg)
	return m, cmd
}
