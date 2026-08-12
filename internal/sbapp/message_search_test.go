package sbapp

import (
	"fmt"
	"strings"
	"testing"

	"github.com/karlssonsimon/lazyaz/internal/azure/servicebus"
	"github.com/karlssonsimon/lazyaz/internal/ui"
	"github.com/karlssonsimon/lazyaz/internal/vim"

	tea "charm.land/bubbletea/v2"

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

// Counted scroll in the message body: 3ctrl+e moves three lines.
func TestMessageBodyCountedScroll(t *testing.T) {
	var sb strings.Builder
	for i := 0; i < 100; i++ {
		sb.WriteString("line\n")
	}
	m := messageSearchModel(t, sb.String())

	for _, key := range []string{"3", "ctrl+e"} {
		m2, _ := m.handleViewingMessageKey(tea.KeyPressMsg{}, key)
		m = m2
	}
	if got := m.messageViewport.YOffset(); got != 3 {
		t.Errorf("YOffset = %d after 3 ctrl+e, want 3", got)
	}

	// Counted j scrolls too, and the count is consumed each time.
	for _, key := range []string{"2", "j"} {
		m2, _ := m.handleViewingMessageKey(tea.KeyPressMsg{}, key)
		m = m2
	}
	if got := m.messageViewport.YOffset(); got != 5 {
		t.Errorf("YOffset = %d after 2j, want 5", got)
	}
	if got := m.vimr.PendingCount(); got != 0 {
		t.Errorf("count = %d after motions, want 0", got)
	}
}

func msgVimModel(t *testing.T, body string) Model {
	t.Helper()
	m := messageSearchModel(t, body)
	m2, _ := m.handleViewingMessageKey(tea.KeyPressMsg{}, "v")
	m = m2
	if !m.msgVim.active {
		t.Fatal("v did not enter the capture")
	}
	return m
}

func msgKeys(t *testing.T, m Model, keys ...string) Model {
	t.Helper()
	for _, k := range keys {
		m2, _ := m.handleViewingMessageKey(tea.KeyPressMsg{}, k)
		m = m2
	}
	return m
}

// The message body is the second TextBuffer consumer: motions, the
// operator grammar and objects all work in its capture.
func TestMessageVimCapture(t *testing.T) {
	m := msgVimModel(t, "foo bar \"qux\" tail\nsecond line\n")

	if got := m.inputMode().String(); got != "VIM" {
		t.Fatalf("mode = %q, want VIM", got)
	}
	if !m.IsTextInputActive() {
		t.Fatal("capture must claim the keyboard")
	}

	m = msgKeys(t, m, "w", "e")
	if c := m.msgVim.cur; c.Line != 0 || c.Col != 6 {
		t.Fatalf("we landed at (%d,%d), want (0,6)", c.Line, c.Col)
	}

	// $ is sticky through j.
	m = msgKeys(t, m, "$", "j")
	if c := m.msgVim.cur; c.Line != 1 || c.Col != 10 {
		t.Fatalf("$j landed at (%d,%d), want (1,10)", c.Line, c.Col)
	}

	// yi" resolves against the body.
	var r vim.Resolver
	r.ArmOperator()
	r.BufferMotion(msgMotionKeys(m.Keymap), "i", m.msgBuf(), vim.Cursor{Line: 0, Col: 10}, false)
	act := r.BufferMotion(msgMotionKeys(m.Keymap), "\"", m.msgBuf(), vim.Cursor{Line: 0, Col: 10}, false)
	if act.Kind != vim.BufYank {
		t.Fatalf("yi\" = %+v", act)
	}
	lo := m.msgByteAt(act.Region.Start.Line, act.Region.Start.Col)
	hi := m.msgByteAt(act.Region.End.Line, act.Region.End.Col)
	if got := m.selectedMessage.FullBody[lo:hi]; got != "qux" {
		t.Fatalf("yi\" text = %q, want qux", got)
	}
}

// vi" selects in the capture and y yanks the selection.
func TestMessageVimVisualObject(t *testing.T) {
	m := msgVimModel(t, "say \"hello\" end\n")

	m = msgKeys(t, m, "v", "i", "\"")
	if !m.msgVim.span.Active {
		t.Fatal("vi\" did not select")
	}
	lo, hi, ok := m.msgSelectionByteRange()
	if !ok {
		t.Fatal("no selection range")
	}
	if got := m.selectedMessage.FullBody[lo:hi]; got != "hello" {
		t.Fatalf("vi\" selects %q, want hello", got)
	}

	m2, cmd := m.msgYankSelection()
	if cmd == nil {
		t.Fatal("y produced no clipboard command")
	}
	if m2.msgVim.span.Active {
		t.Fatal("span still active after yank")
	}
}

// Esc walks: visual → capture → browse; browse esc backs out to the
// list as before.
func TestMessageVimEscLadder(t *testing.T) {
	m := msgVimModel(t, "alpha\nbeta\n")
	m = msgKeys(t, m, "v")
	if got := m.inputMode().String(); got != "VISUAL" {
		t.Fatalf("setup: mode %q", got)
	}

	m = msgKeys(t, m, "esc")
	if got := m.inputMode().String(); got != "VIM" {
		t.Fatalf("first esc: mode %q, want VIM", got)
	}
	m = msgKeys(t, m, "esc")
	if m.msgVim.active {
		t.Fatal("second esc did not leave the capture")
	}
	if !m.viewingMessage {
		t.Fatal("second esc left the message view; it should only leave the capture")
	}
}

// zz centers the cursor line in the capture.
func TestMessageVimScrollOps(t *testing.T) {
	var sb strings.Builder
	for i := 0; i < 100; i++ {
		fmt.Fprintf(&sb, "line %02d\n", i)
	}
	m := msgVimModel(t, sb.String())
	m = msgKeys(t, m, "2", "5", "j")
	if got := m.msgVim.cur.Line; got != 25 {
		t.Fatalf("setup: line %d, want 25", got)
	}
	h := m.messageViewport.Height()
	m = msgKeys(t, m, "z", "z")
	if got, want := m.messageViewport.YOffset(), 25-(h-1)/2; got != want {
		t.Fatalf("zz: YOffset %d, want %d", got, want)
	}
}

// Keys reach the capture through the mode dispatch: once v flips the
// input mode to VIM/VISUAL/V-LINE, handleKey must still route to the
// message handler instead of falling through and swallowing everything.
func TestMessageVimKeysRouteThroughHandleKey(t *testing.T) {
	m := messageSearchModel(t, "alpha\nbeta\ngamma\n")
	m.focus = messagePreviewPane

	press := func(m Model, code rune, text string) Model {
		t.Helper()
		m2, _ := m.handleKey(tea.KeyPressMsg{Code: code, Text: text})
		return m2
	}

	m = press(m, 'v', "v")
	if !m.msgVim.active {
		t.Fatal("v did not enter the capture")
	}

	m = press(m, 'j', "j")
	if got := m.msgVim.cur.Line; got != 1 {
		t.Fatalf("j fell through the mode dispatch: line %d, want 1", got)
	}

	m = press(m, 'v', "v")
	if got := m.inputMode().String(); got != "VISUAL" {
		t.Fatalf("v did not start a selection: mode %q", got)
	}
	m = press(m, 'j', "j")
	if got := m.msgVim.cur.Line; got != 2 {
		t.Fatalf("j in visual fell through: line %d, want 2", got)
	}

	m2, _ := m.handleKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	m2, _ = m2.handleKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m2.msgVim.active {
		t.Fatal("esc fell through the mode dispatch; capture never exits")
	}
}

// Browse mode is untouched: y still yanks the whole body, h still
// backs out.
func TestMessageBrowseUnchanged(t *testing.T) {
	m := messageSearchModel(t, "the body\n")

	m2, cmd := m.handleViewingMessageKey(tea.KeyPressMsg{}, "y")
	if cmd == nil {
		t.Fatal("browse y no longer yanks the body")
	}
	if m2.msgVim.active {
		t.Fatal("browse y entered the capture")
	}
}
