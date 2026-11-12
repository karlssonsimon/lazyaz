package sbapp

import (
	"fmt"
	"strings"
	"testing"

	"github.com/karlssonsimon/lazyaz/internal/azure/servicebus"
)

// Issue #6: receive-with-lock was only offered on the DLQ. Active
// queues need it too — it's the only way to remove a specific message
// from a live queue.

func receiveTestModel(deadLetter bool) Model {
	m := NewModel(nil, testConfig, nil)
	m.hasPeekTarget = true
	m.deadLetter = deadLetter
	m.hasNamespace = true
	m.currentNS = servicebus.Namespace{Name: "ns"}
	m.currentEntity = servicebus.Entity{Name: "test-queue"}
	m.focus = messagesPane
	return m
}

func actionLabels(m Model) []string {
	actions := m.buildActions()
	labels := make([]string, len(actions))
	for i, a := range actions {
		labels[i] = a.label
	}
	return labels
}

func hasLabelContaining(labels []string, substr string) bool {
	for _, l := range labels {
		if strings.Contains(l, substr) {
			return true
		}
	}
	return false
}

func TestReceiveOfferedOnActiveQueue(t *testing.T) {
	m := receiveTestModel(false)
	labels := actionLabels(m)
	if !hasLabelContaining(labels, "Receive active messages (with lock)") {
		t.Fatalf("active queue must offer receive-with-lock, got %v", labels)
	}
}

func TestActiveLockedActionsHideRequeue(t *testing.T) {
	m := receiveTestModel(false)
	m.lockedMessages = &servicebus.ReceivedMessages{}
	labels := actionLabels(m)

	if hasLabelContaining(labels, "Requeue") {
		t.Fatalf("requeue makes no sense from the active queue, got %v", labels)
	}
	if !hasLabelContaining(labels, "remove from queue") {
		t.Fatalf("complete should say 'remove from queue' in active mode, got %v", labels)
	}
	if !hasLabelContaining(labels, "Move") || !hasLabelContaining(labels, "Abandon all") {
		t.Fatalf("move and abandon must stay available in active mode, got %v", labels)
	}
}

func TestDLQLockedActionsKeepRequeue(t *testing.T) {
	m := receiveTestModel(true)
	m.lockedMessages = &servicebus.ReceivedMessages{}
	labels := actionLabels(m)

	if !hasLabelContaining(labels, "Requeue") {
		t.Fatalf("DLQ locked mode must keep requeue, got %v", labels)
	}
	if !hasLabelContaining(labels, "remove from DLQ") {
		t.Fatalf("complete should say 'remove from DLQ' in DLQ mode, got %v", labels)
	}
}

func TestActiveReceiveInstallsLocks(t *testing.T) {
	m := receiveTestModel(false)
	updated, _ := m.handleMessagesReceived(messagesReceivedMsg{
		namespace:  m.currentNS,
		entityName: "test-queue",
		deadLetter: false,
		result:     &servicebus.ReceivedMessages{},
	})
	if updated.lockedMessages == nil {
		t.Fatal("active-mode receive result should install locked messages")
	}
	if got := updated.messageList.Title; got != "Locked (0)" {
		t.Fatalf("title = %q, want Locked (0)", got)
	}
}

// TestModeMismatchedReceiveIsStale pins the staleness guard: a result
// received for the DLQ while the user has switched to the active view
// (or vice versa) must not be installed.
func TestModeMismatchedReceiveIsStale(t *testing.T) {
	m := receiveTestModel(false)
	updated, _ := m.handleMessagesReceived(messagesReceivedMsg{
		namespace:  m.currentNS,
		entityName: "test-queue",
		deadLetter: true, // receive was fired for the DLQ
	})
	if updated.lockedMessages != nil {
		t.Fatal("mode-mismatched receive result must be dropped as stale")
	}
}

// Receive-with-lock takes as many messages as are peeked into the
// list, so "peek more" then receive locks everything on screen rather
// than the first batch only.
func TestReceiveCountFollowsPeekedMessages(t *testing.T) {
	m := receiveTestModel(true)
	if got := m.receiveCount(); got != peekMaxMessages {
		t.Fatalf("nothing peeked: receive count = %d, want %d", got, peekMaxMessages)
	}
	m.peekedMessages = make([]servicebus.PeekedMessage, 20)
	if got := m.receiveCount(); got != peekMaxMessages {
		t.Fatalf("20 peeked: receive count = %d, want the default %d", got, peekMaxMessages)
	}
	m.peekedMessages = make([]servicebus.PeekedMessage, 150)
	if got := m.receiveCount(); got != 150 {
		t.Fatalf("150 peeked: receive count = %d, want 150", got)
	}
	if !hasLabelContaining(actionLabels(m), "up to 150") {
		t.Fatalf("receive label should state the count, got %v", actionLabels(m))
	}
	m.peekedMessages = make([]servicebus.PeekedMessage, receiveMaxMessages+1)
	if got := m.receiveCount(); got != receiveMaxMessages {
		t.Fatalf("over the cap: receive count = %d, want %d", got, receiveMaxMessages)
	}
}

// With rows marked, the receive reaches the last marked row and no
// further: marking the first 50 of 100 peeked locks 50, not 100.
func TestReceiveCountStopsAtLastMarkedRow(t *testing.T) {
	m := receiveTestModel(true)
	m.peekedMessages = make([]servicebus.PeekedMessage, 100)
	for i := range m.peekedMessages {
		m.peekedMessages[i].MessageID = fmt.Sprintf("msg-%d", i)
	}
	m.messageList.SetItems(m.messageItems())
	mark := func(i int) {
		m.messageList.Select(i)
		m.executeAction(action{id: actionToggleMark})
	}
	for i := 0; i < 50; i++ {
		mark(i)
	}
	if got := m.receiveCount(); got != 50 {
		t.Fatalf("first 50 of 100 marked: receive count = %d, want 50", got)
	}
	mark(79)
	if got := m.receiveCount(); got != 80 {
		t.Fatalf("row 80 also marked: receive count = %d, want 80", got)
	}
}

// Typing a number in the action menu is a count for the highlighted
// action, not a search: the list stays whole, the cursor stays put, and
// Enter hands the count to the action.
func TestActionMenuTypedNumberIsACount(t *testing.T) {
	m := receiveTestModel(true)
	m.actionMenu.open(m.buildActions())
	all := len(m.actionMenu.Visible())
	m.actionMenu.Move(1)
	want, _ := m.actionMenu.Selected()

	m.actionMenu.TypeText("250")
	if got := len(m.actionMenu.Visible()); got != all {
		t.Fatalf("a typed number filtered the menu: %d of %d actions visible", got, all)
	}
	selected, act, count := m.actionMenu.handleKey("enter", m.Keymap)
	if !selected || act.id != want.id {
		t.Fatalf("enter picked %+v (selected=%v), want the highlighted %+v", act, selected, want)
	}
	if count != 250 {
		t.Fatalf("count = %d, want 250", count)
	}

	// Text is still a search, and carries no count.
	m.actionMenu.open(m.buildActions())
	m.actionMenu.TypeText("peek")
	if got := len(m.actionMenu.Visible()); got >= all {
		t.Fatalf("text query did not filter: %d of %d visible", got, all)
	}
	if _, _, count := m.actionMenu.handleKey("enter", m.Keymap); count != 0 {
		t.Fatalf("text query produced count %d, want 0", count)
	}
}

func TestParseCount(t *testing.T) {
	for query, want := range map[string]int{"250": 250, "": 0, "0": 0, "25a": 0, "peek": 0, "1234567": 0} {
		if got := parseCount(query); got != want {
			t.Errorf("parseCount(%q) = %d, want %d", query, got, want)
		}
	}
}
