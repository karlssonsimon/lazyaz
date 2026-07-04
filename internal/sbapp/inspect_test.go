package sbapp

import (
	"testing"

	"github.com/karlssonsimon/lazyaz/internal/azure/servicebus"
)

// Issue #7: the inspect strip (K) must surface message properties and
// broker metadata, not just ID/time/body.
func TestMessageInspectShowsProperties(t *testing.T) {
	m := NewModel(nil, testConfig, nil)
	m.hasPeekTarget = true
	m.deadLetter = true
	m.focus = messagesPane
	m.peekedMessages = []servicebus.PeekedMessage{{
		MessageID:             "m-1",
		ContentType:           "application/json",
		CorrelationID:         "corr-9",
		Subject:               "order-created",
		SessionID:             "sess-1",
		DeadLetterReason:      "MaxDeliveryCountExceeded",
		DeadLetterDescription: "gave up",
		AppProperties:         map[string]string{"tenant": "htg", "retry": "7"},
	}}
	m.messageList.SetItems(m.messageItems())

	_, fields := m.inspectFor(messagesPane)
	got := map[string]string{}
	for _, f := range fields {
		got[f.Label] = f.Value
	}

	want := map[string]string{
		"Content Type":    "application/json",
		"Correlation ID":  "corr-9",
		"Subject":         "order-created",
		"Session":         "sess-1",
		"DLQ Reason":      "MaxDeliveryCountExceeded",
		"DLQ Description": "gave up",
		"Properties":      "retry=7 · tenant=htg",
	}
	for label, value := range want {
		if got[label] != value {
			t.Errorf("field %q = %q, want %q", label, got[label], value)
		}
	}
}

// The DLQ fields only apply to the dead-letter view; the active view
// keeps a stable, shorter field set.
func TestMessageInspectHidesDLQFieldsOnActive(t *testing.T) {
	m := NewModel(nil, testConfig, nil)
	m.hasPeekTarget = true
	m.deadLetter = false
	m.focus = messagesPane
	m.peekedMessages = []servicebus.PeekedMessage{{MessageID: "m-1"}}
	m.messageList.SetItems(m.messageItems())

	_, fields := m.inspectFor(messagesPane)
	for _, f := range fields {
		if f.Label == "DLQ Reason" || f.Label == "DLQ Description" {
			t.Fatalf("active view must not show %q", f.Label)
		}
	}
}
