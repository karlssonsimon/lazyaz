package sbapp

import (
	"strings"
	"testing"
	"time"

	"github.com/karlssonsimon/lazyaz/internal/azure/servicebus"

	"charm.land/bubbles/v2/list"
)

func propsMessage(id string) servicebus.PeekedMessage {
	return servicebus.PeekedMessage{
		MessageID:      id,
		FullBody:       `{"ok":true}`,
		SequenceNumber: 42,
		DeliveryCount:  3,
		EnqueuedAt:     time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC),
		ContentType:    "application/json",
		State:          "Active",
		AppProperties: map[string]string{
			"tenant":   "htg",
			"retry":    "7",
			"aardvark": "first",
		},
	}
}

// messagePropsModel opens a message that carries custom properties.
func messagePropsModel(t *testing.T) Model {
	t.Helper()
	m := NewModel(nil, testConfig, nil)
	m.SubOverlay.Close()
	m.Width, m.Height = 120, 30
	m.resize()
	m.messageViewport.SetWidth(60)
	m.messageViewport.SetHeight(10)
	m.messageList.SetItems([]list.Item{
		messageItem{message: propsMessage("m-1")},
		messageItem{message: propsMessage("m-2")},
	})
	m.messageList.Select(0)
	m.viewingMessage = true
	m.focus = messagePreviewPane
	m.syncPreviewToSelection()
	return m
}

// p cycles body → broker → custom → body, and every reader — msgBody,
// the vim buffer, the viewport — follows the switch.
func TestMessagePropertiesCycle(t *testing.T) {
	m := messagePropsModel(t)
	if m.msgView != msgViewBody {
		t.Fatalf("initial view = %v, want body", m.msgView)
	}

	m = msgKeys(t, m, "p")
	if m.msgView != msgViewBroker {
		t.Fatalf("after p view = %v, want broker", m.msgView)
	}
	body := m.msgBody()
	for _, want := range []string{"Message ID", "m-1", "Sequence Number", "42", "Content Type", "application/json", "Dead-Letter Reason", "-"} {
		if !strings.Contains(body, want) {
			t.Errorf("broker table missing %q:\n%s", want, body)
		}
	}
	if m.msgBuf().LineCount() != len(brokerPropertyRows(m.selectedMessage)) {
		t.Errorf("vim buffer has %d lines, want one per broker row", m.msgBuf().LineCount())
	}

	m = msgKeys(t, m, "p")
	if m.msgView != msgViewCustom {
		t.Fatalf("after pp view = %v, want custom", m.msgView)
	}
	body = m.msgBody()
	if !strings.Contains(body, "tenant") || !strings.Contains(body, "htg") {
		t.Errorf("custom table missing tenant=htg:\n%s", body)
	}
	if strings.Index(body, "aardvark") > strings.Index(body, "retry") {
		t.Errorf("custom keys not sorted:\n%s", body)
	}

	m = msgKeys(t, m, "p")
	if m.msgView != msgViewBody || m.msgBody() != `{"ok":true}` {
		t.Fatalf("third p did not return to the body: view=%v body=%q", m.msgView, m.msgBody())
	}
}

// = is a body-view concern: in a property view it does nothing.
func TestMessagePropertiesFormatIsBodyOnly(t *testing.T) {
	m := messagePropsModel(t)
	m = msgKeys(t, m, "p", "=")
	if m.msgFormatted {
		t.Fatal("= formatted while a property table was shown")
	}
	if m.msgView != msgViewBroker {
		t.Fatalf("= changed the view to %v", m.msgView)
	}
}

// The chosen view sticks when the selection moves to another message,
// so stepping through a queue keeps the same table up.
func TestMessagePropertiesStickyAcrossMessages(t *testing.T) {
	m := messagePropsModel(t)
	m = msgKeys(t, m, "p", "p")
	m.messageList.Select(1)
	m.syncPreviewToSelection()
	if m.msgView != msgViewCustom {
		t.Fatalf("view reset to %v on selection change", m.msgView)
	}
	if m.selectedMessage.MessageID != "m-2" {
		t.Fatalf("selection did not move: %s", m.selectedMessage.MessageID)
	}
}

// p inside the vim capture switches the view and keeps the capture,
// with the cursor back at the origin.
func TestMessagePropertiesFromVimCapture(t *testing.T) {
	m := messagePropsModel(t)
	m = msgKeys(t, m, "v", "j", "p")
	if !m.msgVim.active {
		t.Fatal("switching views dropped the vim capture")
	}
	if m.msgView != msgViewBroker {
		t.Fatalf("view = %v, want broker", m.msgView)
	}
	if c := m.msgVim.cur; c.Line != 0 || c.Col != 0 {
		t.Fatalf("cursor = (%d,%d), want origin", c.Line, c.Col)
	}
}

// The title names the view so the reader always knows what they see.
func TestMessagePropertiesTitle(t *testing.T) {
	m := messagePropsModel(t)
	m.resize()
	if v := m.View().Content; strings.Contains(v, "broker properties") {
		t.Fatal("body view should not be labelled as a property table")
	}
	m = msgKeys(t, m, "p")
	if v := m.View().Content; !strings.Contains(v, "broker properties") {
		t.Fatal("broker view not named in the pane title")
	}
}

// An empty custom set renders the placeholder and never yanks it.
func TestMessagePropertiesEmptyCustom(t *testing.T) {
	msg := servicebus.PeekedMessage{MessageID: "m-0"}
	if got := customPropertiesText(msg); got != noCustomProperties {
		t.Fatalf("empty custom text = %q", got)
	}
	m := messagePropsModel(t)
	m.selectedMessage = msg
	m.msgView = msgViewCustom
	_, cmd := m.yankMsgView()
	if cmd != nil {
		t.Fatal("yank of the placeholder should be a no-op")
	}
}

// Multi-line values stay inside the value column.
func TestRenderPropRowsMultilineValue(t *testing.T) {
	got := renderPropRows([]propRow{
		{"A", "one"},
		{"Long Label", "first\nsecond"},
	})
	want := "A           one\nLong Label  first\n            second"
	if got != want {
		t.Fatalf("renderPropRows =\n%q\nwant\n%q", got, want)
	}
}

// The copy palette offers the tables and each custom property.
func TestMessageCopyTargetsIncludeProperties(t *testing.T) {
	targets := messageCopyTargets(propsMessage("m-1"))
	labels := make([]string, len(targets))
	for i, tgt := range targets {
		labels[i] = tgt.Label
	}
	joined := strings.Join(labels, "|")
	for _, want := range []string{"Broker properties", "Custom properties", "Property tenant", "Property retry"} {
		if !strings.Contains(joined, want) {
			t.Errorf("copy targets missing %q: %s", want, joined)
		}
	}
	for _, tgt := range targets {
		if tgt.Label == "Property tenant" && tgt.Value != "htg" {
			t.Errorf("Property tenant = %q, want htg", tgt.Value)
		}
	}
}
