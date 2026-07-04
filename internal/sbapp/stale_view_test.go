package sbapp

import (
	"testing"

	"github.com/karlssonsimon/lazyaz/internal/azure/servicebus"
)

// Abandon fires when the user navigates away from a locked view, so a
// late dlqAbandonMsg for the OLD lock session must not wipe whatever
// the user is looking at now — including a fresh lock session.
func TestStaleAbandonDoesNotWipeCurrentView(t *testing.T) {
	m := NewModel(nil, testConfig, nil)
	m.hasPeekTarget = true
	m.deadLetter = true
	m.currentNS = servicebus.Namespace{Name: "ns"}
	m.currentEntity = servicebus.Entity{Name: "entity-b"}
	m.focus = messagesPane

	// The user has moved on: a new lock session with peeked messages.
	current := &servicebus.ReceivedMessages{}
	m.lockedMessages = current
	m.peekedMessages = []servicebus.PeekedMessage{{MessageID: "m-1"}, {MessageID: "m-2"}}
	m.messageList.SetItems(m.messageItems())

	// Late result from the session released when leaving entity A.
	stale := &servicebus.ReceivedMessages{}
	updated, _ := m.handleDLQAbandon(dlqAbandonMsg{locked: stale})

	if updated.lockedMessages != current {
		t.Fatal("stale abandon released the current lock session")
	}
	if got := len(updated.peekedMessages); got != 2 {
		t.Fatalf("stale abandon wiped the current view: %d peeked messages left, want 2", got)
	}
	if got := len(updated.messageList.Items()); got != 2 {
		t.Fatalf("stale abandon cleared the message list: %d rows, want 2", got)
	}
}

// The explicit "Abandon all" flow still clears the view: the released
// session is the one on screen.
func TestMatchingAbandonClearsView(t *testing.T) {
	m := NewModel(nil, testConfig, nil)
	m.hasPeekTarget = true
	m.deadLetter = true
	m.currentNS = servicebus.Namespace{Name: "ns"}
	m.currentEntity = servicebus.Entity{Name: "entity-a"}
	m.focus = messagesPane

	current := &servicebus.ReceivedMessages{}
	m.lockedMessages = current
	m.peekedMessages = []servicebus.PeekedMessage{{MessageID: "m-1"}}
	m.messageList.SetItems(m.messageItems())

	updated, _ := m.handleDLQAbandon(dlqAbandonMsg{locked: current})

	if updated.lockedMessages != nil {
		t.Fatal("matching abandon should release the lock session")
	}
	if got := len(updated.messageList.Items()); got != 0 {
		t.Fatalf("matching abandon should clear the list, %d rows left", got)
	}
}
