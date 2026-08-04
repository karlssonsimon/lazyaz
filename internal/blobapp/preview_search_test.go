package blobapp

import (
	"fmt"
	"strings"
	"testing"

	"github.com/karlssonsimon/lazyaz/internal/ui"

	tea "charm.land/bubbletea/v2"
)

// searchModel opens a preview over a synthetic blob whose window is
// already loaded, so the tests exercise key routing and highlighting
// without a service.
func searchModel(t *testing.T, content string) Model {
	t.Helper()

	m := NewModel(nil, testConfig, nil)
	m.SubOverlay.Close()
	m.Width, m.Height = 120, 30
	m.hasAccount = true
	m.currentAccount.Name = "acct"
	m.hasContainer = true
	m.containerName = "cont"
	m.focus = previewPane

	data := []byte(content)
	m.preview.open = true
	m.preview.blobName = "data.log"
	m.preview.contentType = "text/plain"
	m.preview.blobSize = int64(len(data))
	m.preview.windowStart = 0
	m.preview.windowData = data
	m.preview.lineStarts = computeLineStarts(data)
	m.preview.rendered = renderPreviewContent(data, "data.log", "text/plain", false, m.Styles)
	m.preview.viewport.SetContent(m.preview.rendered)
	m.resize()
	return m
}

func typeKeys(m Model, keys ...string) Model {
	for _, k := range keys {
		var updated tea.Model
		if len(k) == 1 {
			updated, _ = m.Update(tea.KeyPressMsg{Code: rune(k[0]), Text: k})
		} else {
			updated, _ = m.Update(tea.KeyPressMsg{Code: keyCodeFor(k), Text: ""})
		}
		m = updated.(Model)
	}
	return m
}

func keyCodeFor(name string) rune {
	switch name {
	case "enter":
		return tea.KeyEnter
	case "esc":
		return tea.KeyEscape
	}
	return 0
}

// / opens the prompt, and while it is open the keys typed become the
// query rather than triggering preview bindings.
func TestPreviewSearchPromptCapturesKeys(t *testing.T) {
	m := searchModel(t, "alpha\nbeta\ngamma\n")

	m = typeKeys(m, "/")
	if !m.preview.search.bar.InputOpen {
		t.Fatal("search prompt did not open on /")
	}

	// "g" and "j" are preview bindings (gg chord, cursor down); while
	// the prompt is open they must land in the query instead.
	m = typeKeys(m, "g", "j")
	if got := m.preview.search.bar.Input.Value; got != "gj" {
		t.Errorf("query = %q, want %q — preview bindings leaked past the prompt", got, "gj")
	}
	// The g typed into the query must not have armed the gg chord: close
	// the prompt, move down, then press a single g — an armed chord
	// would fire and jump the cursor back to the top.
	m = typeKeys(m, "esc", "j")
	if m.preview.cursor == 0 {
		t.Fatal("setup: cursor did not move off the top")
	}
	m = typeKeys(m, "g")
	if m.preview.cursor == 0 {
		t.Error("typing g into the query armed the gg chord — a single g later fired it")
	}
}

func TestPreviewSearchEscapeClosesPrompt(t *testing.T) {
	m := searchModel(t, "alpha\nbeta\n")

	m = typeKeys(m, "/", "a", "esc")
	if m.preview.search.bar.InputOpen {
		t.Error("prompt still open after esc")
	}
	if m.preview.search.bar.Active() {
		t.Error("esc executed a search; it should abandon the query")
	}
}

// A backward search opens with the ? prompt glyph.
func TestPreviewSearchBackwardPrompt(t *testing.T) {
	m := searchModel(t, "alpha\nbeta\n")

	m = typeKeys(m, "?")
	if !m.preview.search.bar.InputOpen {
		t.Fatal("prompt did not open on ?")
	}
	if got := m.preview.search.bar.Direction; got != ui.SearchBackward {
		t.Errorf("direction = %v, want SearchBackward", got)
	}
}

// An invalid regex reports an error and leaves the prompt open so it can
// be corrected, rather than silently finding nothing.
func TestPreviewSearchInvalidPatternKeepsPromptOpen(t *testing.T) {
	m := searchModel(t, "alpha\nbeta\n")

	m = typeKeys(m, "/", "[", "enter")
	if !m.preview.search.bar.InputOpen {
		t.Error("prompt closed on an invalid pattern; expected it to stay open")
	}
	if m.preview.search.bar.Active() {
		t.Error("an invalid pattern was accepted as a search")
	}
}

// Matches inside the loaded window are highlighted, and the visible text
// is unchanged by the highlighting.
func TestPreviewSearchHighlightsMatchesInView(t *testing.T) {
	content := "alpha one\nbeta two\nalpha three\n"
	m := searchModel(t, content)

	m.preview.search.bar.Open(ui.SearchForward)
	m.preview.search.bar.Input.SetValue("alpha")
	if err := m.preview.search.bar.Accept(); err != nil {
		t.Fatalf("Accept: %v", err)
	}

	ranges := m.previewMatchRanges()
	if len(ranges) != 2 {
		t.Fatalf("highlighted %d rows, want 2: %v", len(ranges), ranges)
	}
	for row, rs := range ranges {
		if len(rs) != 1 {
			t.Errorf("row %d has %d ranges, want 1", row, len(rs))
			continue
		}
		if rs[0].Start != 0 || rs[0].End != 5 {
			t.Errorf("row %d range = [%d,%d), want [0,5)", row, rs[0].Start, rs[0].End)
		}
	}
}

// Column positions are display widths, so a match after multibyte text
// lands where it actually renders rather than at its byte offset.
func TestPreviewSearchHighlightColumnsAreDisplayWidths(t *testing.T) {
	// "det är " is 7 display columns but 8 bytes.
	m := searchModel(t, "det är NEEDLE\n")

	m.preview.search.bar.Open(ui.SearchForward)
	m.preview.search.bar.Input.SetValue("NEEDLE")
	if err := m.preview.search.bar.Accept(); err != nil {
		t.Fatalf("Accept: %v", err)
	}

	ranges := m.previewMatchRanges()
	if len(ranges) != 1 {
		t.Fatalf("highlighted %d rows, want 1", len(ranges))
	}
	for _, rs := range ranges {
		if rs[0].Start != 7 {
			t.Errorf("match starts at column %d, want 7 (byte offset would be 8)", rs[0].Start)
		}
	}
}

// Matches scrolled out of view are not highlighted.
func TestPreviewSearchSkipsMatchesOutsideViewport(t *testing.T) {
	var sb strings.Builder
	for i := 0; i < 200; i++ {
		fmt.Fprintf(&sb, "line %03d NEEDLE\n", i)
	}
	m := searchModel(t, sb.String())

	m.preview.search.bar.Open(ui.SearchForward)
	m.preview.search.bar.Input.SetValue("NEEDLE")
	if err := m.preview.search.bar.Accept(); err != nil {
		t.Fatalf("Accept: %v", err)
	}

	ranges := m.previewMatchRanges()
	if height := m.preview.viewport.Height(); len(ranges) > height {
		t.Errorf("highlighted %d rows for a viewport of %d", len(ranges), height)
	}
	if len(ranges) == 0 {
		t.Error("no matches highlighted at all")
	}
}

// n with no prior search says so instead of doing nothing.
func TestPreviewSearchRepeatWithoutPattern(t *testing.T) {
	m := searchModel(t, "alpha\n")
	before := m.preview.cursor

	m, _ = m.repeatPreviewSearch(ui.SearchForward)
	if m.preview.cursor != before {
		t.Errorf("cursor moved to %d without an active search", m.preview.cursor)
	}
}

// The footer only claims a row once the search bar has something to
// show, so an unsearched preview keeps its full height.
func TestPreviewFooterOnlyWhenSearching(t *testing.T) {
	m := searchModel(t, "alpha\n")
	if m.previewHasFooter() {
		t.Error("preview reserves a footer row before any search")
	}

	m.preview.search.bar.Open(ui.SearchForward)
	if !m.previewHasFooter() {
		t.Error("preview does not reserve a footer row while the prompt is open")
	}
}
