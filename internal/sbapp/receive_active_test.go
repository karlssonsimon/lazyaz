package sbapp

import (
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
