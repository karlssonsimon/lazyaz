package sbapp

import (
	"strings"
	"testing"

	"github.com/karlssonsimon/lazyaz/internal/azure/servicebus"
)

func sessionModel(t *testing.T) Model {
	t.Helper()
	m := NewModel(nil, testConfig, nil)
	m.SubOverlay.Close()
	m.hasNamespace = true
	m.hasPeekTarget = true
	m.currentEntity = servicebus.Entity{Name: "orders", Kind: servicebus.EntityQueue, RequiresSession: true}
	m.focus = messagesPane
	return m
}

// A session-enabled queue peeks through sessions on the active side
// only; its DLQ is a plain peek.
func TestPeekViaSessionsFollowsEntityAndDLQ(t *testing.T) {
	m := sessionModel(t)
	if !m.peekViaSessions() {
		t.Fatal("active queue of a session entity should walk sessions")
	}
	m.deadLetter = true
	if m.peekViaSessions() {
		t.Fatal("the DLQ is never session-aware")
	}
	m.deadLetter = false
	m.currentEntity.RequiresSession = false
	if m.peekViaSessions() {
		t.Fatal("plain queue should not walk sessions")
	}
}

// For a topic, the flag lives on the open subscription.
func TestCurrentRequiresSessionReadsSubscription(t *testing.T) {
	m := sessionModel(t)
	m.currentEntity = servicebus.Entity{Name: "events", Kind: servicebus.EntityTopic}
	m.subscriptions = []servicebus.TopicSubscription{
		{Name: "plain"},
		{Name: "ordered", RequiresSession: true},
	}
	m.currentSubName = "ordered"
	if !m.currentRequiresSession() {
		t.Fatal("session subscription not detected")
	}
	m.currentSubName = "plain"
	if m.currentRequiresSession() {
		t.Fatal("plain subscription flagged as session")
	}
}

// Receive-with-lock is offered on a session queue and says it walks
// sessions; the DLQ keeps the plain wording.
func TestReceiveActionLabelsSessionWalk(t *testing.T) {
	m := sessionModel(t)
	label := func() string {
		for _, a := range m.buildActions() {
			if a.id == actionReceiveMessages {
				return a.label
			}
		}
		return ""
	}
	if got := label(); !strings.Contains(got, "walking sessions") {
		t.Fatalf("active receive label = %q, want the session note", got)
	}
	m.deadLetter = true
	if got := label(); got == "" || strings.Contains(got, "sessions") {
		t.Fatalf("DLQ receive label = %q, want plain wording", got)
	}
}

func TestDistinctSessions(t *testing.T) {
	msgs := []servicebus.PeekedMessage{{SessionID: "a"}, {SessionID: "b"}, {SessionID: "a"}}
	if n := distinctSessions(msgs); n != 2 {
		t.Fatalf("distinctSessions = %d, want 2", n)
	}
}
