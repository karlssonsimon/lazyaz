package sbapp

import (
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

func (m *Model) clearPeekState() {
	m.peekedMessages = nil
	m.hasPeekTarget = false
	m.currentEntity = servicebus.Entity{}
	m.currentSubName = ""
	m.deadLetter = false
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

func (m *Model) refreshMessageItems() {
	idx := m.messageList.Index()
	m.messageList.SetItems(messagesToItems(m.peekedMessages, m.currentMarks(), m.currentDuplicates()))
	m.messageList.Select(idx)
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
