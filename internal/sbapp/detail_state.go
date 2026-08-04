package sbapp

import (
	"fmt"

	"github.com/karlssonsimon/lazyaz/internal/appshell"
	"github.com/karlssonsimon/lazyaz/internal/azure/servicebus"
	"github.com/karlssonsimon/lazyaz/internal/ui"
	"github.com/karlssonsimon/lazyaz/internal/vim"

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

func (m Model) currentMarks() vim.MarkSet {
	scope := m.markScope()
	if scope == "" {
		return vim.MarkSet{}
	}
	return m.markedMessages[scope]
}

func (m *Model) ensureMarks() vim.MarkSet {
	scope := m.markScope()
	if scope == "" {
		// Read-only zero set: no scope means nothing to mark into,
		// matching the nil map this used to return.
		return vim.MarkSet{}
	}
	if _, ok := m.markedMessages[scope]; !ok {
		m.markedMessages[scope] = vim.NewMarkSet()
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
// Called from handleMessagesReceived after m.peekedMessages has been replaced
// with the locked variants. Without this, re-marking on the locked view
// (which the user is tempted to do because the visual mark indicators
// follow LockID, not MessageID) leaves both keys in the marks map and
// doubles the count — issue #4.
func (m *Model) migrateMarksToLocks() {
	scope := m.markScope()
	marks := m.markedMessages[scope]
	if marks.Len() == 0 {
		return
	}

	migrated := vim.NewMarkSet()
	for _, msg := range m.peekedMessages {
		if msg.LockID == "" {
			continue
		}
		if marks.Contains(msg.MessageID) || marks.Contains(msg.LockID) {
			migrated.Add(msg.LockID)
		}
	}

	if migrated.Len() == 0 {
		delete(m.markedMessages, scope)
		return
	}
	m.markedMessages[scope] = migrated
}

func (m *Model) toggleVisualLineMode() {
	if m.visual.Active() {
		m.commitVisualSelection()
		m.visual.Stop()
		m.refreshMessageSelectionDisplay()
		m.Notify(appshell.LevelInfo, fmt.Sprintf("Visual mode off. %d marked.", m.currentMarks().Len()))
		return
	}

	m.visual.Start(m.currentMessageKey())
	m.refreshMessageSelectionDisplay()
	if m.visual.Anchor() == "" {
		m.Notify(appshell.LevelInfo, "Visual mode on. Move up/down to select a range.")
		return
	}
	m.Notify(appshell.LevelInfo, fmt.Sprintf("Visual mode on. %d in range.", len(m.visualSelectionIDs())))
}

func (m *Model) commitVisualSelection() {
	if !m.visual.Active() {
		return
	}
	marks := m.ensureMarks()
	if marks.Items() == nil {
		return
	}
	for _, id := range m.visualSelectionIDs() {
		marks.Add(id)
	}
}

func (m *Model) swapVisualAnchor() {
	if !m.visual.Active() || m.visual.Anchor() == "" {
		return
	}
	anchorIdx := m.visual.AnchorIdx(m.messageList.VisibleVersion(), m.findMessageIdx)
	if anchorIdx < 0 {
		return
	}
	oldCursor := m.currentMessageKey()
	if oldCursor == "" || oldCursor == m.visual.Anchor() {
		return
	}
	m.visual.SetAnchorWithIdx(oldCursor, m.messageList.Index(), m.messageList.VisibleVersion())
	m.messageList.Select(anchorIdx)
}

// findMessageIdx locates a message operation key in the visible list,
// -1 when absent. This is the scan vim.Visual caches per visible
// version.
func (m *Model) findMessageIdx(key string) int {
	for i, it := range m.messageList.VisibleItems() {
		if mi, ok := it.(messageItem); ok && messageOperationKey(mi.message) == key {
			return i
		}
	}
	return -1
}

func (m Model) currentMessageKey() string {
	item, ok := m.messageList.SelectedItem().(messageItem)
	if !ok {
		return ""
	}
	return messageOperationKey(item.message)
}

func (m Model) visualSelectionIDs() []string {
	if m.currentMessageKey() == "" {
		return nil
	}

	// The range covers exactly what the user sees — with a filter
	// applied, hidden rows between the endpoints stay unselected.
	visible := m.messageList.VisibleItems()
	if len(visible) == 0 {
		return nil
	}

	start, end, ok := m.visual.Range(m.messageList.Index(), m.messageList.VisibleVersion(), m.findMessageIdx)
	if !ok {
		return nil
	}

	ids := make([]string, 0, end-start+1)
	for _, it := range visible[start : end+1] {
		if mi, ok := it.(messageItem); ok {
			ids = append(ids, messageOperationKey(mi.message))
		}
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
	m.visual.Stop()
	m.viewingMessage = false
	m.selectedMessage = servicebus.PeekedMessage{}
}

func (m *Model) clearAllMarks() {
	m.markedMessages = make(map[string]vim.MarkSet)
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
	d := ui.NewMarkDelegate(m.Styles.Delegate, m.Styles, messageMarkKey)
	d.Marked = m.currentMarks().Items()
	d.Visual = m.visualSelectionSet()
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
