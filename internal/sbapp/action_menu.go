package sbapp

import (
	"context"
	"fmt"
	"time"

	"github.com/karlssonsimon/lazyaz/internal/appshell"
	"github.com/karlssonsimon/lazyaz/internal/fuzzy"
	"github.com/karlssonsimon/lazyaz/internal/keymap"
	"github.com/karlssonsimon/lazyaz/internal/ui"

	tea "charm.land/bubbletea/v2"
)

type actionID int

const (
	actionPeekMessages actionID = iota
	actionPeekMore
	actionClearMessages
	actionReceiveDLQ
	actionRequeueCurrent
	actionCompleteCurrent
	actionAbandonAll
	actionRequeueAllDLQ
	actionMoveAll
	actionMoveCurrent
	actionSortEntities
	actionFilterDLQ
	actionToggleMark
	actionToggleVisualLine
	actionInspect
	actionRefresh
	actionSubscriptionPicker
	actionThemePicker
	actionHelp
)

type action struct {
	id    actionID
	label string
	hint  string // keybinding shown right-aligned in menu
}

type actionMenuState struct {
	active    bool
	cursorIdx int
	query     string
	filtered  []int
	actions   []action
}

func (s *actionMenuState) open(actions []action) {
	s.active = true
	s.cursorIdx = 0
	s.query = ""
	s.filtered = nil
	s.actions = actions
}

func (s *actionMenuState) close() {
	*s = actionMenuState{}
}

func (s *actionMenuState) refilter() {
	if s.query == "" {
		s.filtered = nil
		s.cursorIdx = 0
		return
	}
	s.filtered = fuzzy.Filter(s.query, s.actions, func(a action) string {
		return a.label
	})
	if s.cursorIdx >= len(s.filtered) {
		s.cursorIdx = max(0, len(s.filtered)-1)
	}
}

func (s *actionMenuState) selectedAction() (action, bool) {
	list := s.actions
	if s.filtered != nil {
		if len(s.filtered) == 0 {
			return action{}, false
		}
		return list[s.filtered[s.cursorIdx]], true
	}
	if s.cursorIdx < len(list) {
		return list[s.cursorIdx], true
	}
	return action{}, false
}

func (s *actionMenuState) visibleCount() int {
	if s.filtered != nil {
		return len(s.filtered)
	}
	return len(s.actions)
}

func (s *actionMenuState) handleKey(key string, km keymap.Keymap) (selected bool, act action) {
	switch {
	case km.ThemeUp.Matches(key):
		if s.cursorIdx > 0 {
			s.cursorIdx--
		}
	case km.ThemeDown.Matches(key):
		if s.cursorIdx < s.visibleCount()-1 {
			s.cursorIdx++
		}
	case km.ThemeApply.Matches(key):
		if a, ok := s.selectedAction(); ok {
			s.close()
			return true, a
		}
	case km.ThemeCancel.Matches(key):
		if s.query != "" {
			s.query = ""
			s.filtered = nil
			s.cursorIdx = 0
		} else {
			s.close()
		}
	case km.BackspaceUp.Matches(key):
		if len(s.query) > 0 {
			s.query = s.query[:len(s.query)-1]
			s.refilter()
		}
	case key == "ctrl+v":
		if text := ui.ReadClipboard(); text != "" {
			s.query += text
			s.refilter()
		}
	default:
		if len(key) == 1 && key[0] >= 32 && key[0] < 127 {
			s.query += key
			s.refilter()
		}
	}
	return false, action{}
}

func (m Model) buildActions() []action {
	km := m.Keymap
	var actions []action

	// Entities pane — sort and filter.
	if m.focus == entitiesPane && m.hasNamespace {
		actions = append(actions, action{actionSortEntities, "Sort entities", km.ToggleDLQFilter.Short()})
		if m.entityDLQFilter {
			actions = append(actions, action{actionFilterDLQ, "Show all entities", ""})
		} else {
			actions = append(actions, action{actionFilterDLQ, "Filter: only with dead letters", ""})
		}
	}

	// Queue type pane — offer bulk operations when a queue type is selected.
	if m.focus == queueTypePane && m.hasPeekTarget {
		if item, ok := m.queueTypeList.SelectedItem().(queueTypeItem); ok && item.count > 0 {
			if item.deadLetter {
				actions = append(actions, action{
					actionRequeueAllDLQ,
					fmt.Sprintf("Requeue all DLQ messages (%d)", item.count),
					"",
				})
			}
			label := "active"
			if item.deadLetter {
				label = "DLQ"
			}
			actions = append(actions, action{
				actionMoveAll,
				fmt.Sprintf("Move all %s messages to... (%d)", label, item.count),
				"",
			})
		}
	}

	// Messages pane — peek, DLQ, and selection actions.
	if m.focus == messagesPane && m.hasPeekTarget {
		label := "active"
		if m.deadLetter {
			label = "DLQ"
		}

		// Peek actions (read-only, always available).
		if len(m.peekedMessages) == 0 && m.lockedMessages == nil {
			actions = append(actions, action{actionPeekMessages, fmt.Sprintf("Peek %s messages", label), ""})
		} else if m.lockedMessages == nil {
			actions = append(actions, action{actionPeekMore, fmt.Sprintf("Peek more %s messages", label), ""})
			actions = append(actions, action{actionPeekMessages, fmt.Sprintf("Peek %s messages (fresh)", label), ""})
			actions = append(actions, action{actionClearMessages, "Clear messages", ""})
		}

		// DLQ receive-with-lock actions.
		if m.deadLetter {
			if m.lockedMessages == nil {
				actions = append(actions, action{actionReceiveDLQ, "Receive DLQ messages (with lock)", ""})
			} else {
				n := len(m.currentMarks())
				if n == 0 {
					n = 1
				}
				actions = append(actions,
					action{actionRequeueCurrent, fmt.Sprintf("Requeue %d message(s)", n), ""},
					action{actionMoveCurrent, fmt.Sprintf("Move %d message(s) to...", n), ""},
					action{actionCompleteCurrent, fmt.Sprintf("Complete %d message(s) (remove from DLQ)", n), ""},
					action{actionAbandonAll, "Abandon all (release locks)", ""},
				)
			}
		}

		// Selection.
		actions = append(actions,
			action{actionToggleMark, "Toggle mark", km.ToggleMark.Short()},
			action{actionToggleVisualLine, "Toggle visual line selection", km.ToggleVisualLine.Short()},
		)
	}

	// App-wide actions — available from any pane.
	actions = append(actions,
		action{actionRefresh, "Refresh", km.RefreshScope.Short()},
		action{actionInspect, "Toggle details panel", km.Inspect.Short()},
		action{actionSubscriptionPicker, "Change subscription", km.SubscriptionPicker.Short()},
	)
	if !m.EmbeddedMode {
		actions = append(actions,
			action{actionThemePicker, "Open theme picker", km.ToggleThemePicker.Short()},
			action{actionHelp, "Toggle help", km.ToggleHelp.Short()},
		)
	}

	return actions
}

func (m Model) executeAction(act action) (Model, tea.Cmd) {
	switch act.id {
	case actionPeekMessages:
		m.clearLockedMessages()
		m.peekedMessages = nil
		m.messageList.ResetFilter()
		m.messageList.SetItems(nil)
		return m.doPeek(false)

	case actionPeekMore:
		return m.doPeek(true)

	case actionClearMessages:
		m.clearLockedMessages()
		m.peekedMessages = nil
		m.messageList.ResetFilter()
		m.messageList.SetItems(nil)
		m.messageList.Title = "Messages"
		if m.viewingMessage {
			m.closePreview()
		}
		m.Notify(appshell.LevelInfo, "Messages cleared")
		return m, nil

	case actionReceiveDLQ:
		m.startLoading(m.focus, "Receiving DLQ messages with lock...")
		return m, tea.Batch(m.Spinner.Tick,
			receiveDLQCmd(m.service, m.currentNS, m.currentEntity.Name, m.currentSubName, peekMaxMessages))

	case actionRequeueCurrent:
		if m.lockedMessages == nil {
			return m, nil
		}
		targets := m.lockedMessageTargets()
		if len(targets) == 0 {
			return m, nil
		}
		m.startLoading(m.focus, fmt.Sprintf("Requeuing %d message(s)...", len(targets)))
		return m, tea.Batch(m.Spinner.Tick,
			requeueDLQMarkedCmd(m.service, m.currentNS, m.currentEntity.Name, m.lockedMessages, targets))

	case actionCompleteCurrent:
		if m.lockedMessages == nil {
			return m, nil
		}
		targets := m.lockedMessageTargets()
		if len(targets) == 0 {
			return m, nil
		}
		m.startLoading(m.focus, fmt.Sprintf("Completing %d message(s)...", len(targets)))
		return m, tea.Batch(m.Spinner.Tick,
			completeDLQMarkedCmd(m.lockedMessages, targets))

	case actionRequeueAllDLQ:
		m.startLoading(m.focus, "Requeuing all DLQ messages...")
		_, dead := m.currentMessageCounts()
		return m, tea.Batch(m.Spinner.Tick,
			requeueAllDLQCmd(m.service, m.currentNS, m.currentEntity.Name, m.currentSubName, int(dead)))

	case actionMoveAll:
		m.openTargetPicker(actionMoveAll)
		return m, nil

	case actionMoveCurrent:
		if m.lockedMessages == nil {
			return m, nil
		}
		targets := m.lockedMessageTargets()
		if len(targets) == 0 {
			return m, nil
		}
		m.openTargetPicker(actionMoveCurrent)
		return m, nil

	case actionAbandonAll:
		if m.lockedMessages == nil {
			return m, nil
		}
		m.startLoading(m.focus, "Abandoning locks...")
		return m, tea.Batch(m.Spinner.Tick,
			abandonDLQCmd(m.lockedMessages))

	case actionSortEntities:
		m.entitySortOverlay.open(m.entitySortField, m.entitySortDesc)
		return m, nil

	case actionFilterDLQ:
		m.entityDLQFilter = !m.entityDLQFilter
		m.applyEntitySort()
		return m, nil

	case actionToggleMark:
		if m.focus == messagesPane {
			item, ok := m.messageList.SelectedItem().(messageItem)
			if !ok {
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
		}
		return m, nil

	case actionToggleVisualLine:
		if m.focus == messagesPane {
			m.toggleVisualLineMode()
		}
		return m, nil

	case actionRefresh:
		return m.refresh()

	case actionInspect:
		m.toggleInspect()
		return m, nil

	case actionSubscriptionPicker:
		m.SubOverlay.Open()
		m.startLoading(-1, "Refreshing subscriptions...")
		return m, tea.Batch(m.Spinner.Tick, fetchSubscriptionsCmd(m.service, m.cache.subscriptions, m.Subscriptions))

	case actionThemePicker:
		if !m.EmbeddedMode && !m.ThemeOverlay.Active {
			m.ThemeOverlay.Open()
		}
		return m, nil

	case actionHelp:
		if !m.EmbeddedMode {
			if m.HelpOverlay.Active {
				m.HelpOverlay.Close()
			} else {
				m.HelpOverlay.Open("Service Bus Explorer Help", m.HelpSections())
			}
		}
		return m, nil
	}

	return m, nil
}

// lockedMessageTargets returns the set of message IDs to operate on.
// If marks exist, returns those. Otherwise returns the currently
// selected message as a single-element set.
func (m Model) lockedMessageTargets() map[string]struct{} {
	marks := m.currentMarks()
	if len(marks) > 0 {
		return marks
	}
	item, ok := m.messageList.SelectedItem().(messageItem)
	if !ok {
		return nil
	}
	return map[string]struct{}{item.message.MessageID: {}}
}

// clearLockedMessages abandons and closes any active locked messages
// asynchronously to avoid blocking the UI thread.
func (m *Model) clearLockedMessages() {
	if m.lockedMessages != nil {
		locked := m.lockedMessages
		m.lockedMessages = nil
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			locked.Close(ctx)
		}()
	}
}

// abandonLockedIfHeld returns a tea.Cmd that abandons and closes any
// held locked messages. Use this when navigating away from the messages
// pane so the UI doesn't block on network calls.
func (m *Model) abandonLockedIfHeld() tea.Cmd {
	if m.lockedMessages == nil {
		return nil
	}
	locked := m.lockedMessages
	m.lockedMessages = nil
	return abandonDLQCmd(locked)
}

// doPeek fires a peek command for the current scope. When append is
// true, new messages are appended to the existing set.
func (m Model) doPeek(append bool) (Model, tea.Cmd) {
	if !m.hasPeekTarget {
		return m, nil
	}

	var fromSeqNo int64
	if append && len(m.peekedMessages) > 0 {
		fromSeqNo = m.peekedMessages[len(m.peekedMessages)-1].SequenceNumber + 1
	}

	label := "active"
	if m.deadLetter {
		label = "DLQ"
	}
	if m.currentSubName == "" {
		m.startLoading(m.focus, fmt.Sprintf("Peeking %s messages from queue %s", label, m.currentEntity.Name))
		return m, tea.Batch(m.Spinner.Tick, peekQueueMessagesCmd(m.service, m.currentNS, m.currentEntity.Name, m.deadLetter, append, false, fromSeqNo))
	}

	m.startLoading(m.focus, fmt.Sprintf("Peeking %s messages from %s/%s", label, m.currentEntity.Name, m.currentSubName))
	return m, tea.Batch(m.Spinner.Tick, peekSubscriptionMessagesCmd(m.service, m.currentNS, m.currentEntity.Name, m.currentSubName, m.deadLetter, append, false, fromSeqNo))
}

func (m Model) renderActionMenu(base string) string {
	s := &m.actionMenu
	indices := s.filtered
	if indices == nil {
		indices = make([]int, len(s.actions))
		for i := range s.actions {
			indices[i] = i
		}
	}
	items := make([]ui.OverlayItem, len(indices))
	for ci, si := range indices {
		items[ci] = ui.OverlayItem{
			Label: s.actions[si].label,
			Hint:  s.actions[si].hint,
		}
	}
	cfg := ui.OverlayListConfig{
		Title:      "Actions",
		Query:      s.query,
		CursorView: m.Cursor.View(),
		CloseHint:  m.Keymap.Cancel.Short(),
		MaxVisible: 10,
		Center:     true,
	}
	return ui.RenderOverlayList(cfg, items, s.cursorIdx, m.Styles.Overlay, m.Width, m.Height, base)
}
