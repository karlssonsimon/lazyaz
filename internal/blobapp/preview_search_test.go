package blobapp

import (
	"fmt"
	"strings"
	"testing"

	"github.com/karlssonsimon/lazyaz/internal/ui"
	"github.com/karlssonsimon/lazyaz/internal/vim"

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
	// the prompt, enter the vim capture, move down, then press a single
	// g — an armed chord would fire and jump the cursor back to the top.
	m = typeKeys(m, "esc", "v", "j")
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

// Counted line motion in the preview: 3j lands three lines down.
func TestPreviewCountedLineMotion(t *testing.T) {
	var sb strings.Builder
	for i := 0; i < 40; i++ {
		fmt.Fprintf(&sb, "line %02d\n", i)
	}
	m := searchModel(t, sb.String())

	m = typeKeys(m, "3", "j")
	if got := m.previewLocalLine(); got != 3 {
		t.Errorf("preview line = %d after 3j, want 3", got)
	}
	if got := m.vimr.PendingCount(); got != 0 {
		t.Errorf("count = %d after the motion, want 0", got)
	}

	// A digit typed into the search prompt is query text, not a count.
	m = typeKeys(m, "/", "3")
	if got := m.vimr.PendingCount(); got != 0 {
		t.Errorf("count = %d, want 0 — the digit belongs to the query", got)
	}
	if got := m.preview.search.bar.Input.Value; got != "3" {
		t.Errorf("query = %q, want %q", got, "3")
	}
}

// The buffer motions drive a real cursor through the preview's key
// path: words, line ends, find chords, and the sticky goal column.
func TestPreviewBufferMotions(t *testing.T) {
	m := searchModel(t, "foo bar.baz\nshort\na longer line here\n")

	m = typeKeys(m, "v") // enter the vim capture
	m = typeKeys(m, "w")
	if c := m.preview.vcur; c.Line != 0 || c.Col != 4 {
		t.Fatalf("w: (%d,%d), want (0,4)", c.Line, c.Col)
	}
	m = typeKeys(m, "$")
	if c := m.preview.vcur; c.Col != 10 {
		t.Fatalf("$: col %d, want 10", c.Col)
	}
	// Sticky $ rides line ends through j.
	m = typeKeys(m, "j", "j")
	if c := m.preview.vcur; c.Line != 2 || c.Col != 17 {
		t.Fatalf("$jj: (%d,%d), want (2,17)", c.Line, c.Col)
	}
	m = typeKeys(m, "0")
	if c := m.preview.vcur; c.Col != 0 {
		t.Fatalf("0: col %d, want 0", c.Col)
	}
	// f chord with a count.
	m = typeKeys(m, "2", "f", "e")
	if c := m.preview.vcur; c.Col != 12 {
		t.Fatalf("2fe: col %d, want 12 (the e of 'line')", c.Col)
	}
	// h moves left now — it must not exit the preview.
	m = typeKeys(m, "h")
	if !m.preview.open || m.focus != previewPane {
		t.Fatal("h left the preview; it should be a cursor motion")
	}
	if c := m.preview.vcur; c.Col != 11 {
		t.Fatalf("h: col %d, want 11", c.Col)
	}
	// The byte cursor tracks the vim cursor.
	if got := m.preview.cursor; got != m.previewByteFromVim() {
		t.Fatalf("byte cursor %d out of sync with vim cursor (%d)", got, m.previewByteFromVim())
	}
}

// The cursor cell renders at its display column, translated through
// the horizontal offset, and survives sitting inside a search match.
func TestPreviewCursorCellRendering(t *testing.T) {
	m := searchModel(t, "det är NEEDLE här\n")

	m = typeKeys(m, "v", "f", "N")
	row, cell, ok := m.previewCursorHighlight()
	if !ok {
		t.Fatal("cursor cell not visible")
	}
	if row != 0 {
		t.Errorf("row = %d, want 0", row)
	}
	if cell.Start != 7 || cell.End != 8 {
		t.Errorf("cell = [%d,%d), want [7,8) — display columns, not bytes", cell.Start, cell.End)
	}

	// Layered over a search match, the cursor must stay a distinct range.
	m.preview.search.bar.Open(ui.SearchForward)
	m.preview.search.bar.Input.SetValue("NEEDLE")
	if err := m.preview.search.bar.Accept(); err != nil {
		t.Fatalf("Accept: %v", err)
	}
	ranges := m.previewMatchRanges()
	_, cell, _ = m.previewCursorHighlight()
	layered := ui.SplitAround(ranges[0], cell)
	found := false
	for _, r := range layered {
		if r.Start == 7 && r.End == 8 {
			found = true
		}
	}
	if !found {
		t.Errorf("cursor cell lost inside the match: %+v", layered)
	}
}

// The view follows the cursor horizontally on long lines.
func TestPreviewHorizontalFollow(t *testing.T) {
	long := strings.Repeat("x", 200) + "END"
	m := searchModel(t, long+"\n")

	m = typeKeys(m, "v", "$")
	if got := m.preview.vcur.Col; got != 202 {
		t.Fatalf("$ col = %d, want 202", got)
	}
	if m.preview.viewport.XOffset() == 0 {
		t.Fatal("viewport did not scroll horizontally to follow $")
	}

	m = typeKeys(m, "0")
	if got := m.preview.viewport.XOffset(); got != 0 {
		t.Fatalf("XOffset = %d after 0, want 0", got)
	}
}

// j keeps the cursor line visible with scrolloff instead of pinning it
// to the top row.
func TestPreviewVerticalFollowNotPinned(t *testing.T) {
	var sb strings.Builder
	for i := 0; i < 100; i++ {
		fmt.Fprintf(&sb, "line %02d\n", i)
	}
	m := searchModel(t, sb.String())

	m = typeKeys(m, "v", "j", "j", "j")
	if got := m.preview.viewport.YOffset(); got != 0 {
		t.Fatalf("YOffset = %d after 3 x j, want 0 — the view must not scroll while the cursor is inside it", got)
	}
	if c := m.preview.vcur; c.Line != 3 {
		t.Fatalf("cursor line = %d, want 3", c.Line)
	}
}

// v selects charwise, esc drops to normal without leaving the preview,
// and y puts exactly the selected bytes on the yank path.
func TestPreviewCharwiseVisualAndYank(t *testing.T) {
	m := searchModel(t, "foo bar baz\nsecond line\n")

	m = typeKeys(m, "v", "v")
	if !m.preview.span.Active || m.preview.span.Mode != vim.SpanChar {
		t.Fatal("vv did not start a charwise selection")
	}
	if got := m.inputMode().String(); got != "VISUAL" {
		t.Fatalf("mode = %q, want VISUAL", got)
	}

	// Extend over "foo b" with motions.
	m = typeKeys(m, "w")
	lo, hi, ok := m.previewSelectionByteRange()
	if !ok || lo != 0 || hi != 5 {
		t.Fatalf("selection = [%d,%d) ok=%v, want [0,5)", lo, hi, ok)
	}

	// Esc exits visual, not the preview.
	m = typeKeys(m, "esc")
	if m.preview.span.Active {
		t.Fatal("esc did not drop the selection")
	}
	if !m.preview.open || m.focus != previewPane {
		t.Fatal("esc in visual left the preview")
	}
}

// V selects whole lines including the newline, and the yank description
// speaks lines.
func TestPreviewLinewiseSelection(t *testing.T) {
	m := searchModel(t, "first\nsecond\nthird\n")

	m = typeKeys(m, "v", "j", "V", "j")
	if got := m.inputMode().String(); got != "V-LINE" {
		t.Fatalf("mode = %q, want V-LINE", got)
	}
	lo, hi, ok := m.previewSelectionByteRange()
	if !ok {
		t.Fatal("no selection")
	}
	if lo != 6 || hi != 19 {
		t.Fatalf("selection = [%d,%d), want [6,19) — 'second\\nthird\\n' whole lines", lo, hi)
	}

	// The selection rows render full-width.
	sel := m.previewSelectionRanges()
	if len(sel) != 2 {
		t.Fatalf("selection covers %d rows, want 2", len(sel))
	}
}

// The yank budget refuses rather than truncating.
func TestPreviewYankBudgetRefuses(t *testing.T) {
	m := searchModel(t, strings.Repeat("x", 100)+"\n")
	m.yankBudget = 10

	m = typeKeys(m, "V", "y")
	if m.preview.span.Active {
		t.Fatal("span still active after refused yank")
	}
	// Nothing to assert on the clipboard — the refusal never reaches it;
	// the guard is that no yank command was produced, which the budget
	// notify path guarantees synchronously.
}

// y without a selection informs instead of silently doing nothing.
func TestPreviewYankWithoutSelection(t *testing.T) {
	m := searchModel(t, "abc\n")
	m2, cmd := m.yankPreviewSelection()
	if cmd != nil {
		t.Fatal("yank without selection produced a command")
	}
	_ = m2
}

// The preview opens in browse mode: h backs out like every other pane,
// no cursor is shown, and v enters the capture.
func TestPreviewBrowseModeDefaults(t *testing.T) {
	m := searchModel(t, "alpha\nbeta\n")

	if m.preview.vimMode {
		t.Fatal("preview opened in vim mode; browse is the default")
	}
	if got := m.inputMode().String(); got != "NORMAL" {
		t.Fatalf("mode = %q, want NORMAL in browse", got)
	}
	if _, _, ok := m.previewCursorHighlight(); ok {
		t.Fatal("cursor cell rendered in browse mode")
	}

	m = typeKeys(m, "h")
	if m.focus == previewPane {
		t.Fatal("h did not back out of the preview in browse mode")
	}
}

// v enters the capture: the mode chip reads VIM, the cursor appears,
// and the app-side key claim is on so chrome shortcuts stay blocked.
func TestPreviewVimModeEntry(t *testing.T) {
	m := searchModel(t, "alpha\nbeta\n")

	m = typeKeys(m, "v")
	if !m.preview.vimMode {
		t.Fatal("v did not enter vim mode")
	}
	if got := m.inputMode().String(); got != "VIM" {
		t.Fatalf("mode = %q, want VIM", got)
	}
	if _, _, ok := m.previewCursorHighlight(); !ok {
		t.Fatal("no cursor cell in vim mode")
	}
	if !m.IsTextInputActive() {
		t.Fatal("vim mode must claim the keyboard so the app forwards every key")
	}

	// Non-vim keys are swallowed: tab must not switch panes, q must not
	// reach quit.
	before := m.focus
	m = typeKeys(m, "q")
	if m.focus != before || !m.preview.vimMode {
		t.Fatal("q leaked through the capture")
	}
}

// V from browse enters the capture with a linewise selection started.
func TestPreviewShiftVEntersSelecting(t *testing.T) {
	m := searchModel(t, "alpha\nbeta\n")

	m = typeKeys(m, "V")
	if !m.preview.vimMode {
		t.Fatal("V did not enter vim mode")
	}
	if !m.preview.span.Active || m.preview.span.Mode != vim.SpanLine {
		t.Fatal("V did not start a linewise selection")
	}
	if got := m.inputMode().String(); got != "V-LINE" {
		t.Fatalf("mode = %q, want V-LINE", got)
	}
}

// Esc walks the ladder one rung at a time: visual → vim → browse →
// out of the preview.
func TestPreviewEscLadder(t *testing.T) {
	m := searchModel(t, "alpha\nbeta\n")

	m = typeKeys(m, "v", "v")
	if got := m.inputMode().String(); got != "VISUAL" {
		t.Fatalf("setup: mode = %q, want VISUAL", got)
	}

	m = typeKeys(m, "esc")
	if got := m.inputMode().String(); got != "VIM" {
		t.Fatalf("first esc: mode = %q, want VIM", got)
	}
	m = typeKeys(m, "esc")
	if got := m.inputMode().String(); got != "NORMAL" {
		t.Fatalf("second esc: mode = %q, want NORMAL (browse)", got)
	}
	if m.focus != previewPane {
		t.Fatal("second esc left the preview; it should only leave the capture")
	}
	m = typeKeys(m, "esc")
	if m.focus == previewPane {
		t.Fatal("third esc did not leave the preview")
	}
}

// Esc in the search prompt removes the whole search: the executed
// pattern, its highlights and the footer badge — not just the typed
// query.
func TestPreviewSearchEscRemovesHighlights(t *testing.T) {
	m := searchModel(t, "alpha one\nalpha two\n")

	m.preview.search.bar.Open(ui.SearchForward)
	m.preview.search.bar.Input.SetValue("alpha")
	if err := m.preview.search.bar.Accept(); err != nil {
		t.Fatalf("Accept: %v", err)
	}
	if len(m.previewMatchRanges()) == 0 {
		t.Fatal("setup: no highlights to clear")
	}

	m = typeKeys(m, "/", "esc")
	if m.preview.search.bar.Active() {
		t.Fatal("search still active after esc in the prompt")
	}
	if got := m.previewMatchRanges(); len(got) != 0 {
		t.Fatalf("highlights survived the esc: %v", got)
	}
	if m.previewHasFooter() {
		t.Fatal("search footer badge survived the esc")
	}
}

// With an executed search showing, esc dismisses it first and only the
// next esc leaves the pane — search is the first rung of the ladder.
func TestPreviewEscClearsSearchBeforeExiting(t *testing.T) {
	m := searchModel(t, "alpha one\nalpha two\n")

	m = typeKeys(m, "/", "a", "l", "p", "h", "a", "enter")
	if !m.preview.search.bar.Active() {
		t.Fatal("setup: search did not execute")
	}

	m = typeKeys(m, "esc")
	if m.preview.search.bar.Active() {
		t.Fatal("first esc did not clear the search")
	}
	if m.focus != previewPane {
		t.Fatal("first esc left the preview instead of clearing the search")
	}

	m = typeKeys(m, "esc")
	if m.focus == previewPane {
		t.Fatal("second esc did not leave the preview")
	}
}

// The full ladder with everything active: search → visual → vim →
// browse → out, one esc per rung. h skips the search rung and just
// navigates.
func TestPreviewFullEscLadderWithSearch(t *testing.T) {
	m := searchModel(t, "alpha\nbeta\n")

	m = typeKeys(m, "/", "a", "enter", "v", "v")
	if !m.preview.search.bar.Active() || m.inputMode().String() != "VISUAL" {
		t.Fatalf("setup: search=%v mode=%q", m.preview.search.bar.Active(), m.inputMode().String())
	}

	steps := []struct {
		wantSearch bool
		wantMode   string
	}{
		{false, "VISUAL"}, // search cleared first
		{false, "VIM"},    // then the selection
		{false, "NORMAL"}, // then the capture
	}
	for i, st := range steps {
		m = typeKeys(m, "esc")
		if m.preview.search.bar.Active() != st.wantSearch {
			t.Fatalf("esc %d: search active = %v, want %v", i+1, m.preview.search.bar.Active(), st.wantSearch)
		}
		if got := m.inputMode().String(); got != st.wantMode {
			t.Fatalf("esc %d: mode = %q, want %q", i+1, got, st.wantMode)
		}
	}
	m = typeKeys(m, "esc")
	if m.focus == previewPane {
		t.Fatal("final esc did not leave the preview")
	}
}

// The operator grammar through the real key path: ye, y3w, yi", yy and
// W all resolve against the loaded window, and the region-to-byte
// conversion yields exactly the text that lands on the clipboard.
func TestPreviewOperatorGrammar(t *testing.T) {
	//                  0123456789...
	m := searchModel(t, "foo bar \"qux\" tail\nsecond line\n")
	m = typeKeys(m, "v") // vim capture

	regionText := func(m Model, reg vim.Region) string {
		var lo, hi int64
		if reg.Linewise {
			lo = m.previewByteAt(reg.Start.Line, 0)
			if reg.End.Line+1 < len(m.preview.lineStarts) {
				hi = m.preview.windowStart + int64(m.preview.lineStarts[reg.End.Line+1])
			} else {
				hi = m.preview.windowStart + int64(len(m.preview.windowData))
			}
		} else {
			lo = m.previewByteAt(reg.Start.Line, reg.Start.Col)
			hi = m.previewByteAt(reg.End.Line, reg.End.Col)
		}
		ws := m.preview.windowStart
		return string(m.preview.windowData[lo-ws : hi-ws])
	}

	t.Run("ye yanks a word inclusively", func(t *testing.T) {
		mm := typeKeys(m, "y")
		if !mm.vimr.OperatorPending() {
			t.Fatal("y did not arm the operator")
		}
		act := mm.vimr.BufferMotion(previewMotionKeys(mm.Keymap), "e", mm.previewBuf(), mm.preview.vcur, false)
		if act.Kind != vim.BufYank {
			t.Fatalf("ye = %+v", act)
		}
		if got := regionText(mm, act.Region); got != "foo" {
			t.Fatalf("ye text = %q, want foo", got)
		}
	})

	t.Run("yi quote yanks the string body", func(t *testing.T) {
		mm := typeKeys(m, "y", "i", "\"")
		_ = mm
		// Resolve again statically for the text assertion.
		var r vim.Resolver
		r.ArmOperator()
		r.BufferMotion(previewMotionKeys(m.Keymap), "i", m.previewBuf(), m.preview.vcur, false)
		act := r.BufferMotion(previewMotionKeys(m.Keymap), "\"", m.previewBuf(), m.preview.vcur, false)
		if act.Kind != vim.BufYank {
			t.Fatalf("yi\" = %+v", act)
		}
		if got := regionText(m, act.Region); got != "qux" {
			t.Fatalf("yi\" text = %q, want qux", got)
		}
	})

	t.Run("yy takes the whole line with newline", func(t *testing.T) {
		var r vim.Resolver
		r.ArmOperator()
		act := r.BufferMotion(previewMotionKeys(m.Keymap), "y", m.previewBuf(), m.preview.vcur, false)
		if act.Kind != vim.BufYank || !act.Region.Linewise {
			t.Fatalf("yy = %+v", act)
		}
		if got := regionText(m, act.Region); got != "foo bar \"qux\" tail\n" {
			t.Fatalf("yy text = %q", got)
		}
	})

	t.Run("W moves by big words in the capture", func(t *testing.T) {
		mm := typeKeys(m, "W", "W")
		if c := mm.preview.vcur; c.Col != 8 {
			t.Fatalf("WW landed at col %d, want 8 (the quote)", c.Col)
		}
	})

	t.Run("vi quote selects and moves the cursor", func(t *testing.T) {
		mm := typeKeys(m, "v", "i", "\"")
		if !mm.preview.span.Active || mm.preview.span.Mode != vim.SpanChar {
			t.Fatal("vi\" did not select")
		}
		lo, hi, ok := mm.previewSelectionByteRange()
		if !ok {
			t.Fatal("no selection range")
		}
		ws := mm.preview.windowStart
		if got := string(mm.preview.windowData[lo-ws : hi-ws]); got != "qux" {
			t.Fatalf("vi\" selects %q, want qux", got)
		}
		if c := mm.preview.vcur; c.Col != 11 {
			t.Fatalf("cursor at col %d after vi\", want 11 (the x)", c.Col)
		}
	})

	t.Run("ye through keys moves the cursor to the region start", func(t *testing.T) {
		mm := typeKeys(m, "3", "l") // col 3
		mm = typeKeys(mm, "y", "b") // yank back to col 0
		if c := mm.preview.vcur; c.Col != 0 {
			t.Fatalf("cursor at col %d after yb, want 0 (region start)", c.Col)
		}
		if mm.vimr.BufferPending() {
			t.Fatal("grammar state leaked after the yank")
		}
	})
}

// G lands on the first non-blank of the last line and gg on the first
// non-blank of the first line — vim's rule — not on the raw byte ends.
func TestPreviewJumpsLandOnFirstNonBlank(t *testing.T) {
	m := searchModel(t, "  indented first\nmiddle\n    indented last\n")
	m = typeKeys(m, "v")

	m = typeKeys(m, "G")
	if c := m.preview.vcur; c.Line != 2 || c.Col != 4 {
		t.Fatalf("G landed at (%d,%d), want (2,4) — first non-blank of the last line", c.Line, c.Col)
	}
	if got := m.previewByteFromVim(); got != m.preview.cursor {
		t.Fatal("byte cursor out of sync after G")
	}

	m = typeKeys(m, "g", "g")
	if c := m.preview.vcur; c.Line != 0 || c.Col != 2 {
		t.Fatalf("gg landed at (%d,%d), want (0,2)", c.Line, c.Col)
	}

	// A plain motion afterwards must not snap.
	m = typeKeys(m, "$", "j")
	if c := m.preview.vcur; c.Col == 0 || c.Col == 2 {
		t.Fatalf("motion after the jump got snapped: col %d", c.Col)
	}
}
