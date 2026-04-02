package sbapp

import (
	"errors"
	"fmt"

	"azure-storage/internal/cache"
	"azure-storage/internal/azure/servicebus"
	"azure-storage/internal/ui"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
)

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.resize()
		return m, nil

	case spinner.TickMsg:
		if !m.loading {
			return m, nil
		}
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case subscriptionsLoadedMsg:
		if msg.err != nil {
			m.loading = false
			m.lastErr = msg.err.Error()
			m.status = "Failed to load subscriptions"
			return m, nil
		}

		m.lastErr = ""
		m.subscriptions = msg.subscriptions

		if msg.done {
			m.loading = false
			m.cache.subscriptions.Set("", msg.subscriptions)
			m.status = fmt.Sprintf("Loaded %d subscriptions.", len(msg.subscriptions))
			if !m.hasSubscription {
				m.subOverlay.Open()
			}
		}

		return m, msg.next

	case namespacesLoadedMsg:
		if !m.hasSubscription || m.currentSub.ID != msg.subscriptionID {
			return m, nil
		}

		if msg.err != nil {
			m.loading = false
			m.lastErr = msg.err.Error()
			m.status = fmt.Sprintf("Failed to load namespaces in %s", subscriptionDisplayName(m.currentSub))
			return m, nil
		}

		m.lastErr = ""
		m.namespaces = msg.namespaces
		m.namespacesList.Title = fmt.Sprintf("Namespaces (%d)", len(msg.namespaces))
		ui.SetItemsPreserveIndex(&m.namespacesList, namespacesToItems(msg.namespaces))

		if msg.done {
			m.loading = false
			m.cache.namespaces.Set(msg.subscriptionID, msg.namespaces)
			m.status = fmt.Sprintf("Loaded %d namespaces from %s.", len(msg.namespaces), subscriptionDisplayName(m.currentSub))
		}

		return m, msg.next

	case entitiesLoadedMsg:
		if !m.hasNamespace || m.currentNS.Name != msg.namespace.Name {
			return m, nil
		}

		if msg.err != nil {
			m.loading = false
			m.lastErr = msg.err.Error()
			m.status = fmt.Sprintf("Failed to load entities in %s", msg.namespace.Name)
			return m, nil
		}

		m.lastErr = ""
		m.entities = msg.entities
		items := entitiesToFilteredItems(m.entities, m.dlqFilter)
		ui.SetItemsPreserveIndex(&m.entitiesList, items)
		m.entitiesList.Title = m.entitiesPaneTitle()

		if msg.done {
			m.loading = false
			m.cache.entities.Set(cache.Key(m.currentSub.ID, msg.namespace.Name), msg.entities)
			m.status = fmt.Sprintf("Loaded %d entities from %s.", len(msg.entities), msg.namespace.Name)
		}

		return m, msg.next

	case topicSubscriptionsLoadedMsg:
		if !m.hasEntity || m.currentEntity.Kind != servicebus.EntityTopic {
			return m, nil
		}
		if m.currentNS.Name != msg.namespace.Name || m.currentEntity.Name != msg.topicName {
			return m, nil
		}

		if msg.err != nil {
			m.loading = false
			m.lastErr = msg.err.Error()
			m.status = fmt.Sprintf("Failed to load subscriptions for topic %s", msg.topicName)
			return m, nil
		}

		m.lastErr = ""
		m.topicSubs = msg.subs
		m.detailMode = detailTopicSubscriptions
		m.detailList.Title = fmt.Sprintf("Topic Subscriptions (%d)", len(msg.subs))
		ui.SetItemsPreserveIndex(&m.detailList, topicSubsToItems(msg.subs))

		if msg.done {
			m.loading = false
			m.cache.topicSubs.Set(cache.Key(m.currentSub.ID, msg.namespace.Name, msg.topicName), msg.subs)
			m.status = fmt.Sprintf("Loaded %d subscriptions for topic %s", len(msg.subs), msg.topicName)
		}

		return m, msg.next

	case messagesLoadedMsg:
		// Messages are ephemeral peek results — not cached.
		m.loading = false
		if msg.err != nil {
			m.lastErr = msg.err.Error()
			m.status = fmt.Sprintf("Failed to peek messages from %s", msg.source)
			return m, nil
		}

		m.lastErr = ""
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
		m.status = fmt.Sprintf("Peeked %d messages from %s", len(msg.messages), msg.source)
		return m, nil

	case requeueDoneMsg:
		m.loading = false
		m.markedMessages = make(map[string]struct{})
		if msg.err != nil {
			var dupErr *servicebus.DuplicateError
			if errors.As(msg.err, &dupErr) {
				m.duplicateMessages[dupErr.MessageID] = struct{}{}
				m.lastErr = fmt.Sprintf("message %s sent but not removed from DLQ (possible duplicate)", dupErr.MessageID)
			} else {
				m.lastErr = msg.err.Error()
			}
		} else {
			m.lastErr = ""
		}
		if msg.requeued > 0 {
			m.status = fmt.Sprintf("%d of %d message(s) requeued", msg.requeued, msg.total)
		} else {
			m.status = "Failed to requeue messages"
		}
		var peekCmd tea.Cmd
		m, peekCmd = m.rePeekMessages()
		return m, tea.Batch(peekCmd, refreshEntitiesCmd(m.service, m.currentNS))

	case deleteDuplicateDoneMsg:
		m.loading = false
		if msg.err != nil {
			m.lastErr = msg.err.Error()
			m.status = "Failed to delete duplicate message"
			return m, nil
		}
		m.lastErr = ""
		delete(m.duplicateMessages, msg.messageID)
		m.status = "Duplicate message deleted"
		var peekCmd tea.Cmd
		m, peekCmd = m.rePeekMessages()
		return m, tea.Batch(peekCmd, refreshEntitiesCmd(m.service, m.currentNS))

	case entitiesRefreshedMsg:
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

	case tea.KeyMsg:
		key := msg.String()

		if m.subOverlay.Active {
			if sub, ok := m.subOverlay.HandleKey(key, ui.ThemeKeyBindings{
				Up: m.keymap.ThemeUp, Down: m.keymap.ThemeDown,
				Apply: m.keymap.ThemeApply, Cancel: m.keymap.ThemeCancel,
			}, m.subscriptions); ok {
				return m.selectSubscription(sub)
			}
			return m, nil
		}

		if !m.EmbeddedMode && m.helpOverlay.Active {
			m.helpOverlay.HandleKey(key, ui.HelpKeyBindings{
				Up: m.keymap.ThemeUp, Down: m.keymap.ThemeDown,
				Close: m.keymap.ToggleHelp,
			})
			return m, nil
		}
		if !m.EmbeddedMode && m.themeOverlay.Active {
			if m.themeOverlay.HandleKey(key, ui.ThemeKeyBindings{
				Up: m.keymap.ThemeUp, Down: m.keymap.ThemeDown,
				Apply: m.keymap.ThemeApply, Cancel: m.keymap.ThemeCancel,
			}, m.schemes) {
				m.applyScheme(m.schemes[m.themeOverlay.ActiveThemeIdx])
				ui.SaveThemeName(m.schemes[m.themeOverlay.ActiveThemeIdx].Name)
			}
			return m, nil
		}

		if m.viewingMessage {
			switch {
			case ui.ShouldQuit(key, m.keymap.Quit, false):
				return m, tea.Quit
			case m.keymap.MessageBack.Matches(key):
				m.viewingMessage = false
				m.selectedMessage = servicebus.PeekedMessage{}
				m.resize()
				m.status = "Back to messages"
				return m, nil
			default:
				m.messageViewport, cmd = m.messageViewport.Update(msg)
				return m, cmd
			}
		}

		focusedFilterActive := m.focusedListSettingFilter()

		switch {
		case ui.ShouldQuit(key, m.keymap.Quit, focusedFilterActive):
			return m, tea.Quit
		case m.keymap.HalfPageDown.Matches(key):
			m.scrollFocusedHalfPage(1)
			return m, nil
		case m.keymap.HalfPageUp.Matches(key):
			m.scrollFocusedHalfPage(-1)
			return m, nil
		case m.keymap.NextFocus.Matches(key):
			if !focusedFilterActive {
				m.nextFocus()
				return m, nil
			}
		case m.keymap.PreviousFocus.Matches(key):
			if !focusedFilterActive {
				m.previousFocus()
				return m, nil
			}
		case m.keymap.ReloadSubscriptions.Matches(key):
			if !focusedFilterActive {
				m.loading = true
				m.lastErr = ""
				m.status = "Refreshing subscriptions..."
				return m, tea.Batch(spinner.Tick, fetchSubscriptionsCmd(m.service, m.cache.subscriptions))
			}
		case m.keymap.RefreshScope.Matches(key):
			if !focusedFilterActive {
				return m.refresh()
			}
		case m.keymap.OpenFocused.Matches(key):
			if focusedFilterActive {
				m.commitFocusedFilter()
				m.status = fmt.Sprintf("Filter applied for %s", paneName(m.focus))
				return m, nil
			}
			return m.handleEnter()
		case m.keymap.OpenFocusedAlt.Matches(key):
			if !focusedFilterActive {
				return m.handleEnter()
			}
		case m.keymap.NavigateLeft.Matches(key):
			if !focusedFilterActive {
				return m.navigateLeft()
			}
		case m.keymap.ToggleMark.Matches(key):
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
					m.status = fmt.Sprintf("Unmarked %s (%d marked)", id, len(m.markedMessages))
				} else {
					m.markedMessages[id] = struct{}{}
					m.status = fmt.Sprintf("Marked %s (%d marked)", id, len(m.markedMessages))
				}
				m.refreshMessageItems()
				return m, nil
			}
		case m.keymap.ShowActiveQueue.Matches(key):
			if !focusedFilterActive && m.focus == detailPane && m.detailMode == detailMessages {
				if m.deadLetter {
					m.deadLetter = false
					m.markedMessages = make(map[string]struct{})
					m.duplicateMessages = make(map[string]struct{})
					return m.rePeekMessages()
				}
			}
		case m.keymap.ShowDeadLetterQueue.Matches(key):
			if !focusedFilterActive && m.focus == detailPane && m.detailMode == detailMessages {
				if !m.deadLetter {
					m.deadLetter = true
					m.markedMessages = make(map[string]struct{})
					m.duplicateMessages = make(map[string]struct{})
					return m.rePeekMessages()
				}
			}
		case m.keymap.RequeueDLQ.Matches(key):
			if !focusedFilterActive && m.focus == detailPane && m.detailMode == detailMessages && m.deadLetter {
				messageIDs := m.collectRequeueIDs()
				if len(messageIDs) == 0 {
					return m, nil
				}
				m.loading = true
				m.lastErr = ""
				m.status = fmt.Sprintf("Requeuing %d message(s)...", len(messageIDs))
				return m, tea.Batch(spinner.Tick, requeueMessagesCmd(m.service, m.currentNS, m.currentEntity, m.viewingTopicSub, m.currentTopicSub, messageIDs))
			}
		case m.keymap.DeleteDuplicate.Matches(key):
			if !focusedFilterActive && m.focus == detailPane && m.detailMode == detailMessages && m.deadLetter {
				item, ok := m.detailList.SelectedItem().(messageItem)
				if !ok || !item.duplicate {
					return m, nil
				}
				m.loading = true
				m.lastErr = ""
				m.status = "Deleting duplicate message..."
				return m, tea.Batch(spinner.Tick, deleteDuplicateCmd(m.service, m.currentNS, m.currentEntity, m.viewingTopicSub, m.currentTopicSub, item.message.MessageID))
			}
		case m.keymap.ToggleDLQFilter.Matches(key):
			if !focusedFilterActive {
				m.dlqFilter = !m.dlqFilter
				m.applyEntityFilter()
				if m.dlqFilter {
					m.status = "DLQ filter enabled – showing only entities with dead-letter messages"
				} else {
					m.status = "DLQ filter disabled – showing all entities"
				}
				return m, nil
			}
		case m.keymap.SubscriptionPicker.Matches(key):
			if !focusedFilterActive {
				m.subOverlay.Open()
				return m, nil
			}
		case !m.EmbeddedMode && m.keymap.ToggleThemePicker.Matches(key):
			if !focusedFilterActive && !m.themeOverlay.Active {
				m.themeOverlay.Open()
				return m, nil
			}
		case !m.EmbeddedMode && m.keymap.ToggleHelp.Matches(key):
			if !focusedFilterActive && !m.themeOverlay.Active {
				if m.helpOverlay.Active {
					m.helpOverlay.Close()
				} else {
					m.helpOverlay.Open("Azure Service Bus Explorer Help", m.HelpSections())
				}
				return m, nil
			}
		case m.keymap.BackspaceUp.Matches(key):
			if !focusedFilterActive {
				return m.handleBackspace()
			}
		}
	}

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
