package sbapp

import (
	"fmt"

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
)

type action struct {
	id    actionID
	label string
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
	var actions []action

	if m.focus == messagesPane && m.hasPeekTarget {
		label := "active"
		if m.deadLetter {
			label = "DLQ"
		}
		if len(m.peekedMessages) == 0 {
			actions = append(actions, action{
				id:    actionPeekMessages,
				label: fmt.Sprintf("Peek %s messages", label),
			})
		} else {
			actions = append(actions, action{
				id:    actionPeekMore,
				label: fmt.Sprintf("Peek more %s messages", label),
			})
			actions = append(actions, action{
				id:    actionPeekMessages,
				label: fmt.Sprintf("Peek %s messages (fresh)", label),
			})
			actions = append(actions, action{
				id:    actionClearMessages,
				label: "Clear messages",
			})
		}
	}

	return actions
}

func (m Model) executeAction(act action) (Model, tea.Cmd) {
	switch act.id {
	case actionPeekMessages:
		m.peekedMessages = nil
		m.messageList.ResetFilter()
		m.messageList.SetItems(nil)
		return m.doPeek(false)

	case actionPeekMore:
		return m.doPeek(true)

	case actionClearMessages:
		m.peekedMessages = nil
		m.messageList.ResetFilter()
		m.messageList.SetItems(nil)
		m.messageList.Title = "Messages"
		if m.viewingMessage {
			m.closePreview()
		}
		m.Notify(appshell.LevelInfo, "Messages cleared")
		return m, nil
	}

	return m, nil
}

// doPeek fires a peek command for the current scope. When append is
// true, new messages are appended to the existing set.
func (m Model) doPeek(append bool) (Model, tea.Cmd) {
	if !m.hasPeekTarget {
		return m, nil
	}

	label := "active"
	if m.deadLetter {
		label = "DLQ"
	}
	m.SetLoading(m.focus)

	if m.currentSubName == "" {
		m.loadingSpinnerID = m.NotifySpinner(fmt.Sprintf("Peeking %s messages from queue %s", label, m.currentEntity.Name))
		return m, tea.Batch(m.Spinner.Tick, peekQueueMessagesCmd(m.service, m.currentNS, m.currentEntity.Name, m.deadLetter, append))
	}

	m.loadingSpinnerID = m.NotifySpinner(fmt.Sprintf("Peeking %s messages from %s/%s", label, m.currentEntity.Name, m.currentSubName))
	return m, tea.Batch(m.Spinner.Tick, peekSubscriptionMessagesCmd(m.service, m.currentNS, m.currentEntity.Name, m.currentSubName, m.deadLetter, append))
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
