package sbapp

import (
	"testing"

	"github.com/karlssonsimon/lazyaz/internal/azure/servicebus"
)

// TestMarksMigrateToLockIDsAfterReceiveWithLock locks down the fix for
// issue #4. Marks added during peek are keyed by MessageID; after a
// receive-with-lock the same physical messages get LockIDs and
// messageOperationKey starts returning the LockID, so the marks become
// orphans visually and re-marking double-counts them. The migration
// rewrites matching MessageID-keyed marks to LockID-keyed marks.
func TestMarksMigrateToLockIDsAfterReceiveWithLock(t *testing.T) {
	m := NewModel(nil, testConfig, nil)
	m.hasPeekTarget = true
	m.deadLetter = true
	m.currentEntity = servicebus.Entity{Name: "test-queue"}
	m.focus = messagesPane

	// Peek 3 DLQ messages.
	m.peekedMessages = []servicebus.PeekedMessage{
		{MessageID: "msg-1"},
		{MessageID: "msg-2"},
		{MessageID: "msg-3"},
	}
	m.messageList.SetItems(m.messageItems())

	// Mark all 3 as the user would.
	for i := 0; i < 3; i++ {
		m.messageList.Select(i)
		m.executeAction(action{id: actionToggleMark})
	}
	if got := len(m.currentMarks()); got != 3 {
		t.Fatalf("after mark all 3 peeked: want 3 marks, got %d", got)
	}

	// Receive with lock — 2 of the 3 messages came back. The third was
	// already gone from the queue (could happen if another consumer
	// took it, or peekMaxMessages capped it).
	m.lockedMessages = &servicebus.ReceivedMessages{}
	m.peekedMessages = []servicebus.PeekedMessage{
		{MessageID: "msg-1", LockID: "1:0"},
		{MessageID: "msg-2", LockID: "1:1"},
	}
	m.messageList.SetItems(m.messageItems())
	m.migrateMarksToLocks()

	marks := m.currentMarks()
	if len(marks) != 2 {
		t.Errorf("after receive: want 2 marks (matching the locked messages), got %d: %v", len(marks), marks)
	}
	if _, ok := marks["1:0"]; !ok {
		t.Errorf("LockID 1:0 should be marked, marks=%v", marks)
	}
	if _, ok := marks["1:1"]; !ok {
		t.Errorf("LockID 1:1 should be marked, marks=%v", marks)
	}
	if _, ok := marks["msg-3"]; ok {
		t.Errorf("orphan mark for unreceived msg-3 should be dropped, marks=%v", marks)
	}
	if _, ok := marks["msg-1"]; ok {
		t.Errorf("MessageID-keyed mark for msg-1 should be replaced, not retained, marks=%v", marks)
	}
}

// TestRequeueLabelMatchesMarkedCountAfterReceive is the user-visible
// repro from issue #4: marking all DLQ messages and receiving with
// lock should leave the action menu's Requeue count equal to the
// number of marked messages, not double it.
func TestRequeueLabelMatchesMarkedCountAfterReceive(t *testing.T) {
	m := NewModel(nil, testConfig, nil)
	m.hasPeekTarget = true
	m.deadLetter = true
	m.currentEntity = servicebus.Entity{Name: "test-queue"}
	m.focus = messagesPane

	m.peekedMessages = []servicebus.PeekedMessage{
		{MessageID: "msg-1"},
		{MessageID: "msg-2"},
	}
	m.messageList.SetItems(m.messageItems())
	for i := 0; i < 2; i++ {
		m.messageList.Select(i)
		m.executeAction(action{id: actionToggleMark})
	}

	m.lockedMessages = &servicebus.ReceivedMessages{}
	m.peekedMessages = []servicebus.PeekedMessage{
		{MessageID: "msg-1", LockID: "1:0"},
		{MessageID: "msg-2", LockID: "1:1"},
	}
	m.messageList.SetItems(m.messageItems())
	m.migrateMarksToLocks()

	// Toggling either locked row should now UNMARK it (not double-mark).
	m.messageList.Select(0)
	m.executeAction(action{id: actionToggleMark})
	if _, stillMarked := m.currentMarks()["1:0"]; stillMarked {
		t.Errorf("toggle on a marked locked row should unmark it, but 1:0 is still marked: %v", m.currentMarks())
	}

	// And the Requeue label should reflect the remaining single mark.
	var requeueLabel string
	for _, a := range m.buildActions() {
		if a.id == actionRequeueCurrent {
			requeueLabel = a.label
		}
	}
	if requeueLabel != "Requeue 1 message(s)" {
		t.Errorf("Requeue label = %q, want %q (marks=%v)", requeueLabel, "Requeue 1 message(s)", m.currentMarks())
	}
}
