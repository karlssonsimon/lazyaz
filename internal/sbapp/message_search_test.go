package sbapp

import (
	"strings"
	"testing"

	"github.com/karlssonsimon/lazyaz/internal/azure/servicebus"
	"github.com/karlssonsimon/lazyaz/internal/ui"

	"charm.land/bubbles/v2/list"
)

func messageSearchModel(t *testing.T, body string) Model {
	t.Helper()

	m := NewModel(nil, testConfig, nil)
	m.SubOverlay.Close()
	m.Width, m.Height = 120, 30
	m.resize()
	m.messageViewport.SetWidth(60)
	m.messageViewport.SetHeight(10)

	msg := servicebus.PeekedMessage{MessageID: "m-1", FullBody: body}
	m.messageList.SetItems([]list.Item{messageItem{message: msg}})
	m.messageList.Select(0)
	m.viewingMessage = true
	m.syncPreviewToSelection()
	return m
}

func TestMessageSearchOpensAndCapturesKeys(t *testing.T) {
	m := messageSearchModel(t, "alpha\nbeta\ngamma\n")

	if _, consumed := m.handleMessageSearchKey("/"); !consumed {
		t.Fatal("/ was not consumed")
	}
	if !m.messageSearch.bar.InputOpen {
		t.Fatal("prompt did not open")
	}

	// "g" is the gg chord prefix in the body; while typing it must land
	// in the query.
	for _, k := range []string{"g", "a"} {
		if _, consumed := m.handleMessageSearchKey(k); !consumed {
			t.Fatalf("key %q not consumed by the open prompt", k)
		}
	}
	if got := m.messageSearch.bar.Input.Value; got != "ga" {
		t.Errorf("query = %q, want %q", got, "ga")
	}
}

func TestMessageSearchFindsAndScrolls(t *testing.T) {
	var sb strings.Builder
	for i := 0; i < 200; i++ {
		sb.WriteString("filler\n")
	}
	sb.WriteString("the NEEDLE is here\n")
	m := messageSearchModel(t, sb.String())

	m.messageSearch.bar.Open(ui.SearchForward)
	m.messageSearch.bar.Input.SetValue("NEEDLE")
	if err := m.messageSearch.bar.Accept(); err != nil {
		t.Fatalf("Accept: %v", err)
	}
	m.runMessageSearch(ui.SearchForward, true)

	if m.messageSearch.cursor == 0 {
		t.Fatal("cursor did not move to the match")
	}
	if m.messageViewport.YOffset() == 0 {
		t.Error("viewport did not scroll to reveal the match")
	}
}

// n advances rather than re-finding the match already under the cursor.
func TestMessageSearchRepeatAdvances(t *testing.T) {
	m := messageSearchModel(t, "one HIT two\nthree HIT four\nfive HIT six\n")

	m.messageSearch.bar.Open(ui.SearchForward)
	m.messageSearch.bar.Input.SetValue("HIT")
	if err := m.messageSearch.bar.Accept(); err != nil {
		t.Fatalf("Accept: %v", err)
	}

	m.runMessageSearch(ui.SearchForward, true)
	first := m.messageSearch.cursor
	m.runMessageSearch(ui.SearchForward, false)
	second := m.messageSearch.cursor

	if second <= first {
		t.Errorf("repeat landed at %d, want past the first match at %d", second, first)
	}
}

// The body is in memory, so wrapping is free and happens immediately
// rather than needing a second n.
func TestMessageSearchWrapsImmediately(t *testing.T) {
	m := messageSearchModel(t, "only HIT here\n")

	m.messageSearch.bar.Open(ui.SearchForward)
	m.messageSearch.bar.Input.SetValue("HIT")
	if err := m.messageSearch.bar.Accept(); err != nil {
		t.Fatalf("Accept: %v", err)
	}

	m.runMessageSearch(ui.SearchForward, true)
	first := m.messageSearch.cursor
	m.runMessageSearch(ui.SearchForward, false)

	if m.messageSearch.cursor != first {
		t.Errorf("wrapped to %d, want back to the only match at %d", m.messageSearch.cursor, first)
	}
}

func TestMessageSearchHighlightsMatches(t *testing.T) {
	m := messageSearchModel(t, "alpha one\nbeta two\nalpha three\n")

	m.messageSearch.bar.Open(ui.SearchForward)
	m.messageSearch.bar.Input.SetValue("alpha")
	if err := m.messageSearch.bar.Accept(); err != nil {
		t.Fatalf("Accept: %v", err)
	}

	ranges := m.messageMatchRanges()
	if len(ranges) != 2 {
		t.Fatalf("highlighted %d rows, want 2", len(ranges))
	}
	for row, rs := range ranges {
		if rs[0].Start != 0 || rs[0].End != 5 {
			t.Errorf("row %d range = [%d,%d), want [0,5)", row, rs[0].Start, rs[0].End)
		}
	}
}

func TestMessageSearchBufferFocused(t *testing.T) {
	m := messageSearchModel(t, "body\n")
	if !m.BufferSearchFocused() {
		t.Error("message body should own the search keys while being viewed")
	}

	m.viewingMessage = false
	if m.BufferSearchFocused() {
		t.Error("search keys claimed while the body is not being viewed")
	}
}
