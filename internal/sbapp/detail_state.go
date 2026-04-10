package sbapp

import (
	"fmt"

	"github.com/karlssonsimon/lazyaz/internal/appshell"
	"github.com/karlssonsimon/lazyaz/internal/azure/servicebus"

	"charm.land/bubbles/v2/list"
)

// markScope returns the scope key under which marks live.
func (m Model) markScope() string {
	if !m.hasPeekTarget {
		return ""
	}
	dlq := "a"
	if m.deadLetter {
		dlq = "d"
	}
	if m.currentSubName == "" {
		return m.currentEntity.Name + "::" + dlq
	}
	return m.currentEntity.Name + "/" + m.currentSubName + "::" + dlq
}

func (m Model) currentMarks() map[string]struct{} {
	scope := m.markScope()
	if scope == "" {
		return nil
	}
	return m.markedMessages[scope]
}

func (m Model) currentDuplicates() map[string]struct{} {
	scope := m.markScope()
	if scope == "" {
		return nil
	}
	return m.duplicateMessages[scope]
}

func (m *Model) ensureMarks() map[string]struct{} {
	scope := m.markScope()
	if scope == "" {
		return nil
	}
	if m.markedMessages[scope] == nil {
		m.markedMessages[scope] = make(map[string]struct{})
	}
	return m.markedMessages[scope]
}

func (m *Model) ensureDuplicates() map[string]struct{} {
	scope := m.markScope()
	if scope == "" {
		return nil
	}
	if m.duplicateMessages[scope] == nil {
		m.duplicateMessages[scope] = make(map[string]struct{})
	}
	return m.duplicateMessages[scope]
}

func (m *Model) clearScopeMarks() {
	scope := m.markScope()
	if scope == "" {
		return
	}
	delete(m.markedMessages, scope)
}

func (m *Model) toggleVisualLineMode() {
	if m.visualLineMode {
		m.commitVisualSelection()
		m.visualLineMode = false
		m.visualAnchor = ""
		m.refreshMessageSelectionDisplay()
		m.Notify(appshell.LevelInfo, fmt.Sprintf("Visual mode off. %d marked.", len(m.currentMarks())))
		return
	}

	m.visualLineMode = true
	m.visualAnchor = m.currentMessageID()
	m.refreshMessageSelectionDisplay()
	if m.visualAnchor == "" {
		m.Notify(appshell.LevelInfo, "Visual mode on. Move up/down to select a range.")
		return
	}
	m.Notify(appshell.LevelInfo, fmt.Sprintf("Visual mode on. %d in range.", len(m.visualSelectionIDs())))
}

func (m *Model) commitVisualSelection() {
	if !m.visualLineMode {
		return
	}
	marks := m.ensureMarks()
	if marks == nil {
		return
	}
	for _, id := range m.visualSelectionIDs() {
		marks[id] = struct{}{}
	}
}

func (m *Model) swapVisualAnchor() {
	if !m.visualLineMode || m.visualAnchor == "" {
		return
	}
	oldAnchor := m.visualAnchor
	oldCursor := m.currentMessageID()
	if oldCursor == "" || oldCursor == oldAnchor {
		return
	}
	for i, it := range m.messageList.VisibleItems() {
		if mi, ok := it.(messageItem); ok && mi.message.MessageID == oldAnchor {
			m.messageList.Select(i)
			m.visualAnchor = oldCursor
			return
		}
	}
}

func (m Model) currentMessageID() string {
	item, ok := m.messageList.SelectedItem().(messageItem)
	if !ok {
		return ""
	}
	return item.message.MessageID
}

func (m Model) visualSelectionIDs() []string {
	if !m.visualLineMode {
		return nil
	}

	current := m.currentMessageID()
	if current == "" {
		return nil
	}

	anchor := m.visualAnchor
	if anchor == "" {
		anchor = current
	}

	// Use the full peeked messages list (not VisibleItems) so the
	// range includes items hidden by a filter.
	msgs := m.peekedMessages
	if len(msgs) == 0 {
		return nil
	}

	anchorIdx := -1
	currentIdx := -1
	for i, msg := range msgs {
		if anchorIdx < 0 && msg.MessageID == anchor {
			anchorIdx = i
		}
		if currentIdx < 0 && msg.MessageID == current {
			currentIdx = i
		}
	}
	if currentIdx < 0 {
		return nil
	}
	if anchorIdx < 0 {
		anchorIdx = currentIdx
	}

	start, end := anchorIdx, currentIdx
	if start > end {
		start, end = end, start
	}

	ids := make([]string, 0, end-start+1)
	for _, msg := range msgs[start : end+1] {
		ids = append(ids, msg.MessageID)
	}
	return ids
}

func (m Model) visualSelectionSet() map[string]struct{} {
	ids := m.visualSelectionIDs()
	if len(ids) == 0 {
		return nil
	}
	s := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		s[id] = struct{}{}
	}
	return s
}

func (m *Model) clearPeekState() {
	m.clearLockedMessages()
	m.peekedMessages = nil
	m.hasPeekTarget = false
	m.currentEntity = servicebus.Entity{}
	m.currentSubName = ""
	m.deadLetter = false
	m.visualLineMode = false
	m.visualAnchor = ""
	m.viewingMessage = false
	m.selectedMessage = servicebus.PeekedMessage{}
}

func (m *Model) clearAllMarks() {
	m.markedMessages = make(map[string]map[string]struct{})
	m.duplicateMessages = make(map[string]map[string]struct{})
}

func (m Model) collectRequeueIDs() []string {
	marks := m.currentMarks()
	dups := m.currentDuplicates()
	if len(marks) > 0 {
		var ids []string
		for _, msg := range m.peekedMessages {
			if _, ok := marks[msg.MessageID]; !ok {
				continue
			}
			if _, isDup := dups[msg.MessageID]; isDup {
				continue
			}
			ids = append(ids, msg.MessageID)
		}
		return ids
	}
	item, ok := m.messageList.SelectedItem().(messageItem)
	if !ok || item.duplicate {
		return nil
	}
	return []string{item.message.MessageID}
}

// refreshMessageItems rebuilds the message list items (for duplicate
// flag changes). Mark/visual rendering is handled by the delegate.
func (m *Model) refreshMessageItems() {
	idx := m.messageList.Index()
	m.messageList.SetItems(messagesToItems(m.peekedMessages, m.currentDuplicates()))
	m.messageList.Select(idx)
	m.refreshMessageSelectionDisplay()
}

// refreshMessageSelectionDisplay updates the delegate's mark/visual
// maps without rebuilding items.
func (m *Model) refreshMessageSelectionDisplay() {
	d := newMessageDelegate(m.Styles.Delegate, m.Styles)
	d.marked = m.currentMarks()
	d.visual = m.visualSelectionSet()
	m.messageList.SetDelegate(d)
}

// buildQueueTypeItems creates the 2-item list for the Active/DLQ picker.
func (m *Model) buildQueueTypeItems() {
	var active, dead int64
	if m.currentSubName == "" {
		active = m.currentEntity.ActiveMsgCount
		dead = m.currentEntity.DeadLetterCount
	} else {
		for _, sub := range m.subscriptions {
			if sub.Name == m.currentSubName {
				active = sub.ActiveMsgCount
				dead = sub.DeadLetterCount
				break
			}
		}
	}
	m.queueTypeList.SetItems([]list.Item{
		queueTypeItem{label: "Active", deadLetter: false, count: active},
		queueTypeItem{label: "DLQ", deadLetter: true, count: dead},
	})
}
