package sbapp

import (
	"fmt"

	"github.com/karlssonsimon/lazyaz/internal/appshell"
	"github.com/karlssonsimon/lazyaz/internal/azure/servicebus"
	"github.com/karlssonsimon/lazyaz/internal/ui"

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

func (m *Model) clearScopeMarks() {
	scope := m.markScope()
	if scope == "" {
		return
	}
	delete(m.markedMessages, scope)
}

// migrateMarksToLocks rewrites marks set during peek (keyed by MessageID)
// to be keyed by LockID after a receive-with-lock pulls the same physical
// messages back. Marks for messages that weren't received are dropped:
// they reference messages no longer in scope of any operation, and
// keeping them inflates the visible mark count.
//
// Called from handleDLQReceived after m.peekedMessages has been replaced
// with the locked variants. Without this, re-marking on the locked view
// (which the user is tempted to do because the visual mark indicators
// follow LockID, not MessageID) leaves both keys in the marks map and
// doubles the count — issue #4.
func (m *Model) migrateMarksToLocks() {
	scope := m.markScope()
	marks := m.markedMessages[scope]
	if len(marks) == 0 {
		return
	}

	migrated := make(map[string]struct{}, len(marks))
	for _, msg := range m.peekedMessages {
		if msg.LockID == "" {
			continue
		}
		if _, byMessageID := marks[msg.MessageID]; byMessageID {
			migrated[msg.LockID] = struct{}{}
			continue
		}
		if _, byLockID := marks[msg.LockID]; byLockID {
			migrated[msg.LockID] = struct{}{}
		}
	}

	if len(migrated) == 0 {
		delete(m.markedMessages, scope)
		return
	}
	m.markedMessages[scope] = migrated
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
	m.visualAnchor = m.currentMessageKey()
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
	oldCursor := m.currentMessageKey()
	if oldCursor == "" || oldCursor == oldAnchor {
		return
	}
	for i, it := range m.messageList.VisibleItems() {
		if mi, ok := it.(messageItem); ok && messageOperationKey(mi.message) == oldAnchor {
			m.messageList.Select(i)
			m.visualAnchor = oldCursor
			return
		}
	}
}

func (m Model) currentMessageKey() string {
	item, ok := m.messageList.SelectedItem().(messageItem)
	if !ok {
		return ""
	}
	return messageOperationKey(item.message)
}

func (m Model) visualSelectionIDs() []string {
	if !m.visualLineMode {
		return nil
	}

	current := m.currentMessageKey()
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
		key := messageOperationKey(msg)
		if anchorIdx < 0 && key == anchor {
			anchorIdx = i
		}
		if currentIdx < 0 && key == current {
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
		ids = append(ids, messageOperationKey(msg))
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
}

// refreshMessageItems rebuilds the message list items. Mark/visual
// rendering is handled by the delegate.
func (m *Model) refreshMessageItems() {
	ui.SetItemsPreserveKey(&m.messageList, m.messageItems(), messageItemKey)
	m.refreshMessageSelectionDisplay()
}

func (m Model) messageItems() []list.Item {
	return messagesToItems(m.peekedMessages, m.messageContentWidth())
}

func (m Model) messageContentWidth() int {
	return m.messageList.Width()
}

// refreshMessageSelectionDisplay updates the delegate's mark/visual
// maps without rebuilding items.
func (m *Model) refreshMessageSelectionDisplay() {
	d := newMessageDelegate(m.Styles.Delegate, m.Styles)
	d.marked = m.currentMarks()
	d.visual = m.visualSelectionSet()
	m.messageList.SetDelegate(d)
}

// currentMessageCounts returns the active and dead-letter message counts
// for the current scope (queue or topic subscription).
func (m *Model) currentMessageCounts() (active, dead int64) {
	if m.currentSubName == "" {
		return m.currentEntity.ActiveMsgCount, m.currentEntity.DeadLetterCount
	}
	for _, sub := range m.subscriptions {
		if sub.Name == m.currentSubName {
			return sub.ActiveMsgCount, sub.DeadLetterCount
		}
	}
	return 0, 0
}

// buildQueueTypeItems creates the 2-item list for the Active/DLQ picker.
func (m *Model) buildQueueTypeItems() {
	active, dead := m.currentMessageCounts()
	m.queueTypeList.SetItems([]list.Item{
		queueTypeItem{label: "Active", deadLetter: false, count: active},
		queueTypeItem{label: "DLQ", deadLetter: true, count: dead},
	})
}
