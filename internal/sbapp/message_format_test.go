package sbapp

import (
	"strings"
	"testing"

	"github.com/karlssonsimon/lazyaz/internal/ui"
)

const minifiedBody = `{"alpha":1,"beta":{"gamma":[1,2,3]},"delta":"end"}`

// = formats the body in memory and everything reads the formatted text;
// = again restores the raw view with the reader's position.
func TestMessageFormatToggle(t *testing.T) {
	m := messageSearchModel(t, minifiedBody)

	m = msgKeys(t, m, "=")
	if !m.msgFormatted || m.msgFormattedKind != "JSON" {
		t.Fatalf("formatted = %v kind = %q, want true JSON", m.msgFormatted, m.msgFormattedKind)
	}
	if m.msgBuf().LineCount() < 2 {
		t.Error("msgBuf still sees the minified body")
	}
	if !strings.Contains(m.msgBody(), "\n") {
		t.Error("msgBody did not switch to the formatted text")
	}

	m = msgKeys(t, m, "=")
	if m.msgFormatted {
		t.Fatal("second = did not restore")
	}
	if m.msgBody() != minifiedBody {
		t.Error("raw body not restored")
	}
	if m.msgFormatStash != nil {
		t.Error("stash not dropped on restore")
	}
}

// = works inside the vim capture, and the capture survives.
func TestMessageFormatFromVimCapture(t *testing.T) {
	m := messageSearchModel(t, minifiedBody)
	m = msgKeys(t, m, "v", "=")
	if !m.msgFormatted {
		t.Fatal("= inside the capture did not format")
	}
	if !m.msgVim.active {
		t.Error("formatting dropped the vim capture")
	}
}

// Search runs against the formatted text while active.
func TestMessageFormatSearchSeesFormattedText(t *testing.T) {
	m := messageSearchModel(t, minifiedBody)
	m = msgKeys(t, m, "=")

	m.messageSearch.bar.Open(ui.SearchForward)
	for _, r := range "gamma" {
		m.handleMessageSearchKey(string(r))
	}
	m.handleMessageSearchKey("enter")

	want := strings.Index(m.msgBody(), "gamma")
	if m.messageSearch.cursor != want {
		t.Errorf("search cursor = %d, want %d (offset in formatted text)", m.messageSearch.cursor, want)
	}
}

// Selecting a different message drops the formatted view.
func TestMessageFormatDropsOnSelectionChange(t *testing.T) {
	m := messageSearchModel(t, minifiedBody)
	m = msgKeys(t, m, "=")
	if !m.msgFormatted {
		t.Fatal("setup: format failed")
	}

	m.selectedMessage.MessageID = "changed"
	m.syncPreviewToSelection()
	if m.msgFormatted {
		t.Error("format state survived a selection change")
	}
}

// Non-JSON/XML bodies refuse and change nothing.
func TestMessageFormatRefusesPlainText(t *testing.T) {
	m := messageSearchModel(t, "just words\nplain lines\n")
	m = msgKeys(t, m, "=")
	if m.msgFormatted {
		t.Fatal("plain text was formatted")
	}
	if m.msgBody() != "just words\nplain lines\n" {
		t.Error("refusal changed the body")
	}
}
