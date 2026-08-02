package sbapp

import (
	"strings"
	"testing"

	"github.com/karlssonsimon/lazyaz/internal/azure/servicebus"
	"github.com/karlssonsimon/lazyaz/internal/ui"

	"charm.land/bubbles/v2/list"
)

func schemeWithStringColor(name, base0B string) ui.Scheme {
	s := ui.FallbackScheme()
	s.Name = name
	s.Base0B = base0B
	return s
}

// The message body is syntax-highlighted once, when the selection
// changes. A theme switch has to re-highlight it, or the detail pane
// keeps showing the old theme's colors until a different message is
// selected.
func TestApplySchemeRehighlightsSelectedMessage(t *testing.T) {
	m := NewModel(nil, testConfig, nil)
	m.SubOverlay.Close()
	m.Width, m.Height = 120, 40
	m.resize()

	// resize only sizes the detail viewport once the messages pane is
	// on screen; size it directly so View renders something to compare.
	m.messageViewport.SetWidth(60)
	m.messageViewport.SetHeight(10)

	msg := servicebus.PeekedMessage{
		MessageID: "m-1",
		FullBody:  `{"greeting": "hello world"}`,
	}
	m.messageList.SetItems([]list.Item{messageItem{message: msg}})
	m.messageList.Select(0)

	m.ApplyScheme(schemeWithStringColor("before", "22C55E"))
	m.syncPreviewToSelection()
	before := m.messageViewport.View()

	m.ApplyScheme(schemeWithStringColor("after", "FF00AA"))
	after := m.messageViewport.View()

	if before == after {
		t.Fatal("message body still carries the previous theme's colors after a scheme change")
	}
	if !strings.Contains(after, "hello world") {
		t.Errorf("re-highlighted message lost its content:\n%s", after)
	}
}

// With nothing selected there is no body to re-highlight; the scheme
// change must not panic or invent content.
func TestApplySchemeWithNoMessageSelected(t *testing.T) {
	m := NewModel(nil, testConfig, nil)
	m.SubOverlay.Close()
	m.Width, m.Height = 120, 40
	m.resize()
	m.messageViewport.SetWidth(60)
	m.messageViewport.SetHeight(10)

	m.ApplyScheme(schemeWithStringColor("after", "FF00AA"))

	if strings.Contains(m.messageViewport.View(), "hello world") {
		t.Error("empty selection produced message content")
	}
}
