package sbapp

import (
	"strings"
	"unicode/utf8"

	"github.com/karlssonsimon/lazyaz/internal/appshell"
	"github.com/karlssonsimon/lazyaz/internal/keymap"
	"github.com/karlssonsimon/lazyaz/internal/ui"
	"github.com/karlssonsimon/lazyaz/internal/vim"

	tea "charm.land/bubbletea/v2"
)

// msgVimState is the message body's vim capture: the second consumer of
// the buffer engine. The body is one string in memory, so everything
// the blob preview does with windows and streaming is a plain slice
// here.
type msgVimState struct {
	active bool
	cur    vim.Cursor
	span   vim.Span
}

// msgBuffer adapts the message body to vim.TextBuffer.
type msgBuffer struct {
	lines  []string
	starts []int // byte offset of each line within the body
}

func newMsgBuffer(body string) msgBuffer {
	lines := strings.Split(body, "\n")
	starts := make([]int, len(lines))
	off := 0
	for i, l := range lines {
		starts[i] = off
		off += len(l) + 1
	}
	return msgBuffer{lines: lines, starts: starts}
}

func (b msgBuffer) LineCount() int { return len(b.lines) }
func (b msgBuffer) Line(i int) string {
	if i < 0 || i >= len(b.lines) {
		return ""
	}
	return b.lines[i]
}

func (m Model) msgBuf() msgBuffer {
	return newMsgBuffer(m.selectedMessage.FullBody)
}

func msgMotionKeys(km keymap.Keymap) vim.MotionKeys {
	return vim.MotionKeys{
		Left: km.MotionLeft, Right: km.MotionRight,
		Down: km.CursorDown, Up: km.CursorUp,
		WordForward: km.MotionWordForward, WordBack: km.MotionWordBack, WordEnd: km.MotionWordEnd,
		BigWordForward: km.MotionBigWord, BigWordBack: km.MotionBigWordBack, BigWordEnd: km.MotionBigWordEnd,
		LineStart: km.MotionLineStart, LineEnd: km.MotionLineEnd,
		FindChar: km.FindChar, FindCharBack: km.FindCharBack,
		TillChar: km.TillChar, TillCharBack: km.TillCharBack,
		RepeatFind: km.RepeatFind, RepeatFindBack: km.RepeatFindBack,
		ObjectInner: km.ObjectInner, ObjectAround: km.ObjectAround,
		FirstNonBlank: km.MotionFirstNonBlank, Underscore: km.MotionUnderscore,
		YankOp: km.PreviewYank,
	}
}

// msgByteAt is the byte offset of a buffer position; col may sit on the
// line-length boundary.
func (m Model) msgByteAt(line, col int) int64 {
	buf := m.msgBuf()
	if len(buf.starts) == 0 {
		return 0
	}
	if line >= len(buf.starts) {
		line = len(buf.starts) - 1
	}
	if line < 0 {
		line = 0
	}
	rs := []rune(buf.Line(line))
	if col > len(rs) {
		col = len(rs)
	}
	if col < 0 {
		col = 0
	}
	return int64(buf.starts[line] + len(string(rs[:col])))
}

// msgSyncFromByte places the cursor at a byte offset — search hits use
// it. Want is preserved.
func (m *Model) msgSyncFromByte(off int64) {
	buf := m.msgBuf()
	line := 0
	for i := range buf.starts {
		if int64(buf.starts[i]) <= off {
			line = i
		} else {
			break
		}
	}
	lineOff := int(off) - buf.starts[line]
	lineStr := buf.Line(line)
	if lineOff > len(lineStr) {
		lineOff = len(lineStr)
	}
	if lineOff < 0 {
		lineOff = 0
	}
	m.msgVim.cur.Line = line
	m.msgVim.cur.Col = len([]rune(lineStr[:lineOff]))
}

// msgFollowCursor scrolls the viewport after the cursor on both axes —
// vertical through ScrollWindow with scrolloff, horizontal with the
// same margin the blob preview uses.
func (m *Model) msgFollowCursor() {
	vp := &m.messageViewport
	buf := m.msgBuf()
	sw := ui.ScrollWindow{
		Cursor:    m.msgVim.cur.Line,
		Offset:    vp.YOffset(),
		Height:    vp.Height(),
		Count:     buf.LineCount(),
		Scrolloff: m.scrolloff,
	}
	vp.SetYOffset(sw.Normalize().Offset)

	rs := []rune(buf.Line(m.msgVim.cur.Line))
	col := m.msgVim.cur.Col
	if col > len(rs) {
		col = len(rs)
	}
	dcol := ui.PlainWidth([]byte(string(rs[:col])))
	width := vp.Width()
	if width <= 0 {
		return
	}
	const margin = 5
	xoff := vp.XOffset()
	if dcol < xoff+margin {
		xoff = dcol - margin
		if xoff < 0 {
			xoff = 0
		}
	}
	if dcol > xoff+width-1-margin {
		xoff = dcol - width + 1 + margin
	}
	vp.SetXOffset(xoff)
}

func (m Model) applyMsgCursor(nc vim.Cursor) (Model, tea.Cmd) {
	m.msgVim.cur = nc
	m.msgFollowCursor()
	return m, nil
}

// applyMsgBufferAction executes what the grammar resolved.
func (m Model) applyMsgBufferAction(act vim.BufferAction) (Model, tea.Cmd) {
	switch act.Kind {
	case vim.BufMoved:
		return m.applyMsgCursor(act.Cursor)
	case vim.BufYank:
		return m.msgYankRegion(act.Region)
	case vim.BufSelect:
		return m.msgSelectRegion(act.Region)
	default:
		return m, nil
	}
}

// msgYankRegion slices the region straight from the body — no windows,
// no budget — and lands the cursor on the region start as vim does.
func (m Model) msgYankRegion(reg vim.Region) (Model, tea.Cmd) {
	body := m.selectedMessage.FullBody
	var lo, hi int64
	if reg.Linewise {
		buf := m.msgBuf()
		lo = int64(buf.starts[reg.Start.Line])
		if reg.End.Line+1 < len(buf.starts) {
			hi = int64(buf.starts[reg.End.Line+1])
		} else {
			hi = int64(len(body))
		}
	} else {
		lo = m.msgByteAt(reg.Start.Line, reg.Start.Col)
		hi = m.msgByteAt(reg.End.Line, reg.End.Col)
	}
	if hi <= lo {
		return m, nil
	}

	target := reg.Start
	if reg.Linewise {
		target = m.msgVim.cur
		target.Line = reg.Start.Line
	}
	m2, _ := m.applyMsgCursor(target)
	return m2, yankTextCmd(body[lo:hi])
}

// msgSelectRegion is a text object resolving in visual mode.
func (m Model) msgSelectRegion(reg vim.Region) (Model, tea.Cmd) {
	m.msgVim.span.Start(m.msgByteAt(reg.Start.Line, reg.Start.Col), vim.SpanChar)
	last := reg.End
	if last.Col > 0 {
		last.Col--
	} else if last.Line > 0 {
		last.Line--
		last.Col = len([]rune(m.msgBuf().Line(last.Line)))
		if last.Col > 0 {
			last.Col--
		}
	}
	last.Want = last.Col
	return m.applyMsgCursor(last)
}

// msgSelectionByteRange is the active v/V selection in byte space.
func (m Model) msgSelectionByteRange() (lo, hi int64, ok bool) {
	if !m.msgVim.span.Active {
		return 0, 0, false
	}
	head := m.msgByteAt(m.msgVim.cur.Line, m.msgVim.cur.Col)
	lo, hi = m.msgVim.span.Range(head)

	body := m.selectedMessage.FullBody
	if m.msgVim.span.Mode == vim.SpanLine {
		buf := m.msgBuf()
		loLine, hiLine := 0, 0
		for i := range buf.starts {
			if int64(buf.starts[i]) <= lo {
				loLine = i
			}
			if int64(buf.starts[i]) <= hi {
				hiLine = i
			}
		}
		lo = int64(buf.starts[loLine])
		if hiLine+1 < len(buf.starts) {
			hi = int64(buf.starts[hiLine+1])
		} else {
			hi = int64(len(body))
		}
		return lo, hi, true
	}

	if int(hi) < len(body) {
		_, n := utf8.DecodeRuneInString(body[hi:])
		if n < 1 {
			n = 1
		}
		hi += int64(n)
	}
	return lo, hi, true
}

// msgYankSelection is y on an active selection.
func (m Model) msgYankSelection() (Model, tea.Cmd) {
	lo, hi, ok := m.msgSelectionByteRange()
	if !ok {
		m.Notify(appshell.LevelInfo, "Nothing selected — start with v or V")
		return m, nil
	}
	m.msgVim.span.Stop()
	body := m.selectedMessage.FullBody
	if hi > int64(len(body)) {
		hi = int64(len(body))
	}
	if hi <= lo {
		return m, nil
	}
	return m, yankTextCmd(body[lo:hi])
}

// yankTextCmd writes to the clipboard off the UI thread through the
// existing clipboardMsg flow.
func yankTextCmd(text string) tea.Cmd {
	return func() tea.Msg {
		if err := ui.WriteClipboard(text); err != nil {
			return clipboardMsg{err: err}
		}
		return clipboardMsg{text: text}
	}
}

// msgFirstNonBlank is vim's landing column for gg and G.
func msgFirstNonBlank(buf msgBuffer, line int) int {
	for i, r := range []rune(buf.Line(line)) {
		if r != ' ' && r != '\t' {
			return i
		}
	}
	return 0
}

func (m Model) msgJumpTop() (Model, tea.Cmd) {
	col := msgFirstNonBlank(m.msgBuf(), 0)
	return m.applyMsgCursor(vim.Cursor{Line: 0, Col: col, Want: col})
}

func (m Model) msgJumpBottom() (Model, tea.Cmd) {
	buf := m.msgBuf()
	line := buf.LineCount() - 1
	col := msgFirstNonBlank(buf, line)
	return m.applyMsgCursor(vim.Cursor{Line: line, Col: col, Want: col})
}

// msgYankToEnd is yG; msgYankToTop is ygg — the body is fully known, so
// both are plain slices.
func (m Model) msgYankToEnd() (Model, tea.Cmd) {
	lo := int64(m.msgBuf().starts[m.msgVim.cur.Line])
	body := m.selectedMessage.FullBody
	if int64(len(body)) <= lo {
		return m, nil
	}
	return m, yankTextCmd(body[lo:])
}

func (m Model) msgYankToTop() (Model, tea.Cmd) {
	buf := m.msgBuf()
	body := m.selectedMessage.FullBody
	var hi int64
	if m.msgVim.cur.Line+1 < len(buf.starts) {
		hi = int64(buf.starts[m.msgVim.cur.Line+1])
	} else {
		hi = int64(len(body))
	}
	if hi <= 0 {
		return m, nil
	}
	m2, _ := m.msgJumpTop()
	return m2, yankTextCmd(body[:hi])
}

// applyMsgScrollOp is zz/zt/zb.
func (m *Model) applyMsgScrollOp(op vim.ScrollOp) {
	vp := &m.messageViewport
	sw := ui.ScrollWindow{
		Cursor:    m.msgVim.cur.Line,
		Offset:    vp.YOffset(),
		Height:    vp.Height(),
		Count:     m.msgBuf().LineCount(),
		Scrolloff: m.scrolloff,
	}
	switch op {
	case vim.ScrollOpCenter:
		sw = sw.CenterOnCursor()
	case vim.ScrollOpTop:
		sw = sw.CursorToTop()
	case vim.ScrollOpBottom:
		sw = sw.CursorToBottom()
	}
	vp.SetYOffset(sw.Offset)
}

// enterMsgVimMode starts the capture with the cursor on the top visible
// line; withLineSelection is V from browse.
func (m Model) enterMsgVimMode(withLineSelection bool) (Model, tea.Cmd) {
	m.msgVim.active = true
	m.msgVim.cur = vim.Cursor{Line: m.messageViewport.YOffset()}
	if withLineSelection {
		m.msgVim.span.Start(m.msgByteAt(m.msgVim.cur.Line, 0), vim.SpanLine)
	}
	return m, nil
}

// handleMessageVimKey is the capture: only vim keys act until esc walks
// back to browse.
func (m Model) handleMessageVimKey(key string) (Model, tea.Cmd) {
	buf := m.msgBuf()

	// yG and ygg resolve here — same shape as the blob preview, minus
	// the windows.
	if m.vimr.OperatorPending() && !m.vimr.FindPending() && !m.vimr.ObjectPending() {
		switch m.vimr.GG(m.Keymap.JumpTopPrefix, key, true) {
		case vim.ChordFired:
			m.vimr.ConsumeOperator()
			m.vimr.ClearCount()
			return m.msgYankToTop()
		case vim.ChordArmed:
			return m, nil
		}
		if m.Keymap.JumpBottom.Matches(key) {
			m.vimr.ConsumeOperator()
			m.vimr.ClearCount()
			return m.msgYankToEnd()
		}
	}

	if m.vimr.BufferPending() {
		act := m.vimr.BufferMotion(msgMotionKeys(m.Keymap), key, buf, m.msgVim.cur, m.msgVim.span.Active)
		return m.applyMsgBufferAction(act)
	}

	switch res, op := m.vimr.Scroll(m.Keymap, key); res {
	case vim.ChordArmed:
		m.vimr.ClearCount()
		m.Notify(appshell.LevelInfo, vim.HintScroll)
		return m, nil
	case vim.ChordSwallowed:
		return m, nil
	case vim.ChordFired:
		m.applyMsgScrollOp(op)
		return m, nil
	}

	switch m.vimr.GG(m.Keymap.JumpTopPrefix, key, true) {
	case vim.ChordFired:
		return m.msgJumpTop()
	case vim.ChordArmed:
		m.Notify(appshell.LevelInfo, vim.HintGG)
		return m, nil
	}

	if m.vimr.Digit(key) {
		return m, nil
	}
	if !countedMsgVimKey(m.Keymap, key) {
		m.vimr.ClearCount()
	}

	if act := m.vimr.BufferMotion(msgMotionKeys(m.Keymap), key, buf, m.msgVim.cur, m.msgVim.span.Active); act.Kind != vim.BufNone {
		return m.applyMsgBufferAction(act)
	}

	switch {
	case m.Keymap.SearchForward.Matches(key):
		m.messageSearch.bar.Open(ui.SearchForward)
		return m, nil
	case m.Keymap.SearchBackward.Matches(key):
		m.messageSearch.bar.Open(ui.SearchBackward)
		return m, nil
	case m.Keymap.SearchNext.Matches(key):
		m.runMessageSearch(m.messageSearch.bar.Direction, false)
		return m, nil
	case m.Keymap.SearchPrev.Matches(key):
		m.runMessageSearch(m.messageSearch.bar.Direction.Opposite(), false)
		return m, nil
	case m.Keymap.VisualChar.Matches(key):
		return m.toggleMsgVisual(vim.SpanChar)
	case m.Keymap.ToggleVisualLine.Matches(key):
		return m.toggleMsgVisual(vim.SpanLine)
	case m.Keymap.PreviewYank.Matches(key):
		if m.msgVim.span.Active {
			return m.msgYankSelection()
		}
		m.vimr.ArmOperator()
		return m, nil
	case m.Keymap.JumpBottom.Matches(key):
		return m.msgJumpBottom()
	case m.Keymap.ScrollLineDown.Matches(key):
		return m.msgScrollView(m.vimr.TakeCount())
	case m.Keymap.ScrollLineUp.Matches(key):
		return m.msgScrollView(-m.vimr.TakeCount())
	case m.Keymap.HalfPageDown.Matches(key):
		step := maxIntSb(1, m.messageViewport.Height()/2)
		return m.applyMsgCursor(vim.MoveDown(buf, m.msgVim.cur, step*m.vimr.TakeCount()))
	case m.Keymap.HalfPageUp.Matches(key):
		step := maxIntSb(1, m.messageViewport.Height()/2)
		return m.applyMsgCursor(vim.MoveUp(buf, m.msgVim.cur, step*m.vimr.TakeCount()))
	case m.Keymap.FullPageDown.Matches(key):
		return m.applyMsgCursor(vim.MoveDown(buf, m.msgVim.cur, maxIntSb(1, m.messageViewport.Height())*m.vimr.TakeCount()))
	case m.Keymap.FullPageUp.Matches(key):
		return m.applyMsgCursor(vim.MoveUp(buf, m.msgVim.cur, maxIntSb(1, m.messageViewport.Height())*m.vimr.TakeCount()))
	// The esc ladder: search → visual → capture → browse.
	case key == "esc" && m.messageSearch.bar.Active():
		m.messageSearch.bar.Clear()
		return m, nil
	case m.msgVim.span.Active && m.Keymap.MessageBack.Matches(key) && key == "esc":
		m.msgVim.span.Stop()
		return m, nil
	case key == "esc" || key == "backspace":
		m.msgVim.active = false
		m.msgVim.span.Stop()
		m.vimr.Clear()
		return m, nil
	default:
		// Swallowed: the capture admits vim keys only.
		return m, nil
	}
}

// msgScrollView is ctrl+e/y: view scroll pushing the cursor only when
// needed.
func (m Model) msgScrollView(delta int) (Model, tea.Cmd) {
	vp := &m.messageViewport
	buf := m.msgBuf()
	sw := ui.ScrollWindow{
		Cursor:    m.msgVim.cur.Line,
		Offset:    vp.YOffset(),
		Height:    vp.Height(),
		Count:     buf.LineCount(),
		Scrolloff: m.scrolloff,
	}
	sw = sw.ScrollBy(delta)
	vp.SetYOffset(sw.Offset)
	nc := m.msgVim.cur
	if sw.Cursor != nc.Line {
		nc = vim.MoveDown(buf, nc, sw.Cursor-nc.Line)
	}
	return m.applyMsgCursor(nc)
}

func (m Model) toggleMsgVisual(mode vim.SpanMode) (Model, tea.Cmd) {
	span := &m.msgVim.span
	if span.Active && span.Mode == mode {
		span.Stop()
		return m, nil
	}
	if span.Active {
		span.Mode = mode
		return m, nil
	}
	if mode == vim.SpanLine {
		span.Start(m.msgByteAt(m.msgVim.cur.Line, 0), mode)
	} else {
		span.Start(m.msgByteAt(m.msgVim.cur.Line, m.msgVim.cur.Col), mode)
	}
	return m, nil
}

func countedMsgVimKey(km keymap.Keymap, key string) bool {
	return km.CursorDown.Matches(key) || km.CursorUp.Matches(key) ||
		km.MotionLeft.Matches(key) || km.MotionRight.Matches(key) ||
		km.MotionWordForward.Matches(key) || km.MotionWordBack.Matches(key) ||
		km.MotionWordEnd.Matches(key) || km.MotionLineEnd.Matches(key) ||
		km.MotionBigWord.Matches(key) || km.MotionBigWordBack.Matches(key) ||
		km.MotionBigWordEnd.Matches(key) ||
		km.FindChar.Matches(key) || km.FindCharBack.Matches(key) ||
		km.TillChar.Matches(key) || km.TillCharBack.Matches(key) ||
		km.RepeatFind.Matches(key) || km.RepeatFindBack.Matches(key) ||
		km.HalfPageDown.Matches(key) || km.HalfPageUp.Matches(key) ||
		km.FullPageDown.Matches(key) || km.FullPageUp.Matches(key) ||
		km.ScrollLineDown.Matches(key) || km.ScrollLineUp.Matches(key) ||
		km.PreviewYank.Matches(key) || km.MotionUnderscore.Matches(key)
}

// msgCursorHighlight is the cursor cell, viewport-relative, capture
// only.
func (m Model) msgCursorHighlight() (row int, rng ui.ColumnRange, ok bool) {
	if !m.msgVim.active {
		return 0, ui.ColumnRange{}, false
	}
	vp := m.messageViewport
	row = m.msgVim.cur.Line - vp.YOffset()
	if row < 0 || row >= vp.Height() {
		return 0, ui.ColumnRange{}, false
	}
	rs := []rune(m.msgBuf().Line(m.msgVim.cur.Line))
	col := m.msgVim.cur.Col
	cellW := 1
	if col >= 0 && col < len(rs) {
		cellW = ui.PlainWidth([]byte(string(rs[col : col+1])))
		if cellW < 1 {
			cellW = 1
		}
	}
	if col > len(rs) {
		col = len(rs)
	}
	start := ui.PlainWidth([]byte(string(rs[:minIntSb(col, len(rs))]))) - vp.XOffset()
	end := start + cellW
	if end <= 0 || start >= vp.Width() {
		return 0, ui.ColumnRange{}, false
	}
	if start < 0 {
		start = 0
	}
	if end > vp.Width() {
		end = vp.Width()
	}
	return row, ui.ColumnRange{Start: start, End: end, Style: m.Styles.CursorCell}, true
}

// msgSelectionRanges maps the active selection onto viewport rows.
func (m Model) msgSelectionRanges() map[int][]ui.ColumnRange {
	lo, hi, ok := m.msgSelectionByteRange()
	if !ok {
		return nil
	}
	buf := m.msgBuf()
	vp := m.messageViewport

	loLine, hiLine := 0, 0
	for i := range buf.starts {
		if int64(buf.starts[i]) <= lo {
			loLine = i
		}
		if int64(buf.starts[i]) < hi {
			hiLine = i
		}
	}

	byRow := make(map[int][]ui.ColumnRange)
	for line := loLine; line <= hiLine; line++ {
		row := line - vp.YOffset()
		if row < 0 || row >= vp.Height() {
			continue
		}
		lineStr := buf.Line(line)
		lineWidth := ui.PlainWidth([]byte(lineStr))

		start := 0
		end := lineWidth
		if m.msgVim.span.Mode == vim.SpanChar {
			ls := int64(buf.starts[line])
			if line == loLine && lo > ls {
				start = ui.PlainWidth([]byte(lineStr[:minIntSb(int(lo-ls), len(lineStr))]))
			}
			if line == hiLine {
				end = ui.PlainWidth([]byte(lineStr[:minIntSb(int(hi-ls), len(lineStr))]))
			}
		}
		if end == start {
			end = start + 1
		}

		start -= vp.XOffset()
		end -= vp.XOffset()
		if end <= 0 || start >= vp.Width() {
			continue
		}
		byRow[row] = []ui.ColumnRange{{
			Start: maxIntSb(start, 0),
			End:   minIntSb(end, vp.Width()),
			Style: m.Styles.SelectionHighlight,
		}}
	}
	return byRow
}

func minIntSb(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxIntSb(a, b int) int {
	if a > b {
		return a
	}
	return b
}
