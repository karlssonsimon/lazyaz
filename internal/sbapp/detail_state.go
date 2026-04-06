package sbapp

import "github.com/karlssonsimon/lazyaz/internal/azure/servicebus"

// markScope returns the scope key under which marks (and duplicate
// flags) live for the current peek target. Marks are scoped to the
// triple (entity, sub-name-or-empty, active|dlq) so switching the
// active/DLQ tab — or returning later to the same scope — preserves
// the user's selections.
//
// Returns "" when no peek target is active.
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

// currentMarks returns the marked-message set for the current peek
// scope. Returns an empty (read-only) map when no peek target is
// active. Mutating helpers must use ensureMarks instead.
func (m Model) currentMarks() map[string]struct{} {
	scope := m.markScope()
	if scope == "" {
		return nil
	}
	return m.markedMessages[scope]
}

// currentDuplicates returns the duplicate-id set for the current peek
// scope.
func (m Model) currentDuplicates() map[string]struct{} {
	scope := m.markScope()
	if scope == "" {
		return nil
	}
	return m.duplicateMessages[scope]
}

// ensureMarks lazily initializes and returns the marked-set for the
// current scope, ready for mutation.
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

// ensureDuplicates lazily initializes and returns the duplicate-set for
// the current scope, ready for mutation.
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

// clearScopeMarks removes all marks/duplicates for the current scope.
// Used after a successful requeue or delete-duplicate when the marks
// for that scope are now stale.
func (m *Model) clearScopeMarks() {
	scope := m.markScope()
	if scope == "" {
		return
	}
	delete(m.markedMessages, scope)
}

// clearPeekState resets all state related to the currently peeked
// message stream — invoked when the user navigates away from a peek
// scope (e.g., changing namespace or selecting a different entity).
// It does NOT touch the entities pane or its tree state. Per-scope
// marks survive across entity changes within a namespace; they're
// only fully wiped on namespace change via clearAllMarks.
func (m *Model) clearPeekState() {
	m.peekedMessages = nil
	m.hasPeekTarget = false
	m.currentEntity = servicebus.Entity{}
	m.currentSubName = ""
	m.deadLetter = false
	m.viewingMessage = false
	m.selectedMessage = servicebus.PeekedMessage{}
}

// clearAllMarks wipes every scope's marks and duplicates. Called on
// namespace change so old marks don't bleed across namespaces.
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
	item, ok := m.detailList.SelectedItem().(messageItem)
	if !ok || item.duplicate {
		return nil
	}
	return []string{item.message.MessageID}
}

func (m *Model) refreshItems() {
	idx := m.detailList.Index()
	m.detailList.SetItems(messagesToItems(m.peekedMessages, m.currentMarks(), m.currentDuplicates()))
	m.detailList.Select(idx)
}

// clearDetailListForRePeek empties the message list (and the preview
// viewport, if mounted) so the user doesn't see stale messages from the
// previous tab while a new peek is in flight. Used when toggling
// active↔DLQ — the new message set is entirely different and even a
// brief lingering of the old data is misleading.
func (m *Model) clearDetailListForRePeek() {
	m.peekedMessages = nil
	m.detailList.ResetFilter()
	m.detailList.SetItems(nil)
	if m.viewingMessage {
		m.selectedMessage = servicebus.PeekedMessage{}
		m.messageViewport.SetContent(m.Styles.Muted.Render("(loading…)"))
		m.messageViewport.GotoTop()
	}
}
