package sbapp

import (
	"errors"
	"fmt"
	"time"

	"github.com/karlssonsimon/lazyaz/internal/appshell"
	"github.com/karlssonsimon/lazyaz/internal/azure/servicebus"
	"github.com/karlssonsimon/lazyaz/internal/ui"

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
	case requeueDoneMsg:
		return m.handleRequeueDone(msg)
	case deleteDuplicateDoneMsg:
		return m.handleDeleteDuplicateDone(msg)
	case entitiesRefreshedMsg:
		return m.handleEntitiesRefreshed(msg)

	case tea.KeyMsg:
		return m.handleKey(msg)

	case tea.MouseClickMsg:
		if m.viewingMessage {
			region := m.messageViewportRegion()
			if m.textSelection.HandleMouseClick(msg, region) {
				return m, nil
			}
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
		return m, nil
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
		return m, nil
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
		return m, nil
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
	items := messagesToItems(m.peekedMessages, m.currentMarks(), m.currentDuplicates())
	if msg.repeek {
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
	m.messageList.Title = fmt.Sprintf("Messages (%d)", len(msg.messages))
	m.resize()
	m.ClearLoading()
	label := "active"
	if msg.deadLetter {
		label = "DLQ"
	}
	m.ResolveSpinner(m.loadingSpinnerID, appshell.LevelSuccess, fmt.Sprintf("Peeked %d %s messages from %s", len(msg.messages), label, msg.source))
	return m, nil
}

func (m Model) handleRequeueDone(msg requeueDoneMsg) (Model, tea.Cmd) {
	m.ClearLoading()
	m.clearScopeMarks()
	switch {
	case msg.err != nil:
		var dupErr *servicebus.DuplicateError
		if errors.As(msg.err, &dupErr) {
			m.ensureDuplicates()[dupErr.MessageID] = struct{}{}
			m.ResolveSpinner(m.loadingSpinnerID, appshell.LevelWarn, fmt.Sprintf("Message %s sent but not removed from DLQ (possible duplicate)", dupErr.MessageID))
		} else {
			m.ResolveSpinner(m.loadingSpinnerID, appshell.LevelError, fmt.Sprintf("Failed to requeue messages: %s", msg.err.Error()))
		}
	case msg.requeued > 0:
		m.ResolveSpinner(m.loadingSpinnerID, appshell.LevelSuccess, fmt.Sprintf("%d of %d message(s) requeued", msg.requeued, msg.total))
	default:
		m.ResolveSpinner(m.loadingSpinnerID, appshell.LevelError, "Failed to requeue messages")
	}
	var peekCmd tea.Cmd
	m, peekCmd = m.rePeekMessages(true)
	return m, tea.Batch(peekCmd, refreshEntitiesCmd(m.service, m.currentNS))
}

func (m Model) handleDeleteDuplicateDone(msg deleteDuplicateDoneMsg) (Model, tea.Cmd) {
	m.ClearLoading()
	if msg.err != nil {
		m.ResolveSpinner(m.loadingSpinnerID, appshell.LevelError, fmt.Sprintf("Failed to delete duplicate message: %s", msg.err.Error()))
		return m, nil
	}
	if dups := m.currentDuplicates(); dups != nil {
		delete(dups, msg.messageID)
	}
	m.ResolveSpinner(m.loadingSpinnerID, appshell.LevelSuccess, "Duplicate message deleted")
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
	// Refresh queue type counts if we're looking at one.
	if m.hasPeekTarget {
		m.buildQueueTypeItems()
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

	if m.actionMenu.active {
		if selected, act := m.actionMenu.handleKey(key, m.Keymap); selected {
			return m.executeAction(act)
		}
		return m, nil
	}

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
			m.Notify(appshell.LevelInfo, fmt.Sprintf("Filter applied for %s", paneName(m.focus)))
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
	case m.Keymap.ActionMenu.Matches(key):
		if !focusedFilterActive && m.focus == messagesPane && m.hasPeekTarget {
			m.actionMenu.open(m.buildActions())
			return m, nil
		}
	case m.Keymap.ToggleDLQFilter.Matches(key):
		if !focusedFilterActive {
			m.dlqSort = !m.dlqSort
			m.applyDLQSort()
			if m.dlqSort {
				m.Notify(appshell.LevelInfo, "DLQ-first sort enabled")
			} else {
				m.Notify(appshell.LevelInfo, "DLQ-first sort disabled")
			}
			return m, nil
		}
	case m.Keymap.SubscriptionPicker.Matches(key):
		if !focusedFilterActive {
			m.SubOverlay.Open()
			m.SetLoading(-1)
			m.loadingSpinnerID = m.NotifySpinner("Refreshing subscriptions...")
			return m, tea.Batch(m.Spinner.Tick, fetchSubscriptionsCmd(m.service, m.cache.subscriptions, m.Subscriptions))
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
		m.setFocus(messagesPane)
		m.Notify(appshell.LevelInfo, "Back to message list")
		return m, nil
	}
	var cmd tea.Cmd
	m.messageViewport, cmd = m.messageViewport.Update(msg)
	return m, cmd
}
