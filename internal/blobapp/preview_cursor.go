package blobapp

import (
	"github.com/karlssonsimon/lazyaz/internal/keymap"
	"github.com/karlssonsimon/lazyaz/internal/ui"
	"github.com/karlssonsimon/lazyaz/internal/vim"

	tea "charm.land/bubbletea/v2"
)

// previewBuffer adapts the loaded window to vim.TextBuffer: raw bytes as
// lines, no ANSI. The motion engine works window-local; the absolute
// byte cursor remains the source of truth for windowing and search.
type previewBuffer struct {
	data   []byte
	starts []int
}

func (b previewBuffer) LineCount() int {
	return len(b.starts)
}

func (b previewBuffer) Line(i int) string {
	if i < 0 || i >= len(b.starts) {
		return ""
	}
	lo := b.starts[i]
	hi := len(b.data)
	if i+1 < len(b.starts) {
		hi = b.starts[i+1]
	}
	s := b.data[lo:hi]
	for len(s) > 0 && (s[len(s)-1] == '\n' || s[len(s)-1] == '\r') {
		s = s[:len(s)-1]
	}
	return string(s)
}

func (m *Model) previewBuf() previewBuffer {
	return previewBuffer{data: m.preview.windowData, starts: m.preview.lineStarts}
}

// previewMotionKeys maps the preview's bindings onto the engine's
// binding set. j/k are the existing PreviewDown/Up keys.
func previewMotionKeys(km keymap.Keymap) vim.MotionKeys {
	return vim.MotionKeys{
		Left: km.MotionLeft, Right: km.MotionRight,
		Down: km.PreviewDown, Up: km.PreviewUp,
		WordForward: km.MotionWordForward, WordBack: km.MotionWordBack, WordEnd: km.MotionWordEnd,
		LineStart: km.MotionLineStart, LineEnd: km.MotionLineEnd,
		FindChar: km.FindChar, FindCharBack: km.FindCharBack,
		TillChar: km.TillChar, TillCharBack: km.TillCharBack,
		RepeatFind: km.RepeatFind, RepeatFindBack: km.RepeatFindBack,
		BigWordForward: km.MotionBigWord, BigWordBack: km.MotionBigWordBack, BigWordEnd: km.MotionBigWordEnd,
		ObjectInner: km.ObjectInner, ObjectAround: km.ObjectAround,
		FirstNonBlank: km.MotionFirstNonBlank, Underscore: km.MotionUnderscore,
		YankOp: km.PreviewYank,
	}
}

// previewByteAt is the absolute byte offset of a buffer position. The
// column may sit on the line-length boundary (before the newline);
// unlike the cursor mapping this is not clamped to blobSize-1, so it
// can express a region's exclusive end.
func (m Model) previewByteAt(line, col int) int64 {
	if len(m.preview.lineStarts) == 0 {
		return m.preview.cursor
	}
	if line >= len(m.preview.lineStarts) {
		line = len(m.preview.lineStarts) - 1
	}
	if line < 0 {
		line = 0
	}
	rs := []rune(m.previewBuf().Line(line))
	if col > len(rs) {
		col = len(rs)
	}
	if col < 0 {
		col = 0
	}
	return m.preview.windowStart + int64(m.preview.lineStarts[line]+len(string(rs[:col])))
}

// syncPreviewVimFromByte recomputes the window-local (line, col) cursor
// from the absolute byte cursor — after a window load or a byte-path
// jump (gg, G, search hit). Want survives, so j after $ keeps riding
// line ends across window slides.
func (m *Model) syncPreviewVimFromByte() {
	if len(m.preview.lineStarts) == 0 {
		m.preview.vcur.Line = 0
		m.preview.vcur.Col = 0
		return
	}
	local := int(clampInt64(m.preview.cursor-m.preview.windowStart, 0, int64(len(m.preview.windowData))))
	line := m.previewLineOf(local)
	lineOff := local - m.preview.lineStarts[line]
	lineStr := m.previewBuf().Line(line)
	if lineOff > len(lineStr) {
		lineOff = len(lineStr)
	}
	if lineOff < 0 {
		lineOff = 0
	}
	col := len([]rune(lineStr[:lineOff]))
	m.preview.vcur.Line = line
	m.preview.vcur.Col = col
}

// previewByteFromVim is the reverse mapping: the absolute byte offset of
// the window-local cursor.
func (m Model) previewByteFromVim() int64 {
	if len(m.preview.lineStarts) == 0 {
		return m.preview.cursor
	}
	line := m.preview.vcur.Line
	if line >= len(m.preview.lineStarts) {
		line = len(m.preview.lineStarts) - 1
	}
	if line < 0 {
		line = 0
	}
	lineStr := m.previewBuf().Line(line)
	rs := []rune(lineStr)
	col := m.preview.vcur.Col
	if col > len(rs) {
		col = len(rs)
	}
	if col < 0 {
		col = 0
	}
	byteOff := len(string(rs[:col]))
	off := m.preview.windowStart + int64(m.preview.lineStarts[line]+byteOff)
	if m.preview.blobSize > 0 {
		off = clampInt64(off, 0, m.preview.blobSize-1)
	}
	return off
}

// applyPreviewCursor lands a motion result: the vim cursor becomes
// authoritative for Want, the byte cursor is re-derived, and the window
// machinery takes over — its tail re-syncs and scrolls the view.
func (m Model) applyPreviewCursor(nc vim.Cursor) (Model, tea.Cmd) {
	// A cursor waiting for its window has nothing loaded to move
	// relative to; the jump lands first.
	if !m.previewHoldsCursor() {
		return m, nil
	}
	m.preview.vcur = nc
	m.preview.cursor = m.previewByteFromVim()
	return m.ensurePreviewWindowAtCursor()
}

// horizontalFollowMargin is how many columns of context the view keeps
// between the cursor and the pane's side edges.
const horizontalFollowMargin = 5

// followPreviewCursor scrolls the viewport so the cursor stays in view:
// vertically through ScrollWindow (which brings scrolloff), horizontally
// with a fixed margin.
func (m *Model) followPreviewCursor() {
	vp := &m.preview.viewport
	// Browse mode tracks the top visible line as its position — or a
	// line below it while the view waits at the end of the window — so
	// applying scrolloff there would push the view back on every
	// scroll. Anywhere else the cursor was put there by a jump or a
	// search hit, and gets its context rows like in the vim capture.
	so := m.scrolloff
	scrollPosition := !m.preview.vimMode && m.preview.vcur.Line == vp.YOffset()
	if scrollPosition || m.previewRunningAhead() {
		so = 0
	}
	sw := ui.ScrollWindow{
		Cursor:    m.preview.vcur.Line,
		Offset:    vp.YOffset(),
		Height:    vp.Height(),
		Count:     len(m.preview.lineStarts),
		Scrolloff: so,
	}
	// A cursor that landed a screen or more away shares nothing with
	// the lines on screen, so it is centered, as vim does; a nearer one
	// scrolls the view just far enough.
	next := sw.Normalize()
	if vp.Height() > 0 && absInt(next.Offset-sw.Offset) >= vp.Height() {
		next = sw.CenterOnCursor()
	}
	vp.SetYOffset(next.Offset)

	width := vp.Width()
	if width <= 0 {
		return
	}
	// The end is brought into view first and the start second, so when
	// the span is wider than the pane it is the start that shows.
	start, end := m.previewCursorSpan()
	xoff := vp.XOffset()
	if end+horizontalFollowMargin > xoff+width {
		xoff = end + horizontalFollowMargin - width
	}
	if start-horizontalFollowMargin < xoff {
		xoff = start - horizontalFollowMargin
		if xoff < 0 {
			xoff = 0
		}
	}
	vp.SetXOffset(xoff)
}

// centerPreviewOnCursor puts the cursor line in the middle of the view,
// as far as the ends of the window allow.
func (m *Model) centerPreviewOnCursor() {
	vp := &m.preview.viewport
	sw := ui.ScrollWindow{
		Cursor: m.preview.vcur.Line,
		Offset: vp.YOffset(),
		Height: vp.Height(),
		Count:  len(m.preview.lineStarts),
	}
	vp.SetYOffset(sw.CenterOnCursor().Offset)
}

// previewCursorSpan is the range of display columns the horizontal
// follow keeps in view: the cursor cell, widened to the whole match when
// the cursor sits at the start of one. A search hit far along a line is
// then shown whole, not as its first few columns at the pane's edge.
func (m Model) previewCursorSpan() (start, end int) {
	start = m.previewCursorDisplayCol()
	end = start + 1

	pattern := m.preview.search.bar.Pattern
	if !pattern.Valid() || len(m.preview.lineStarts) == 0 {
		return start, end
	}
	local := m.preview.cursor - m.preview.windowStart
	if local < 0 || local >= int64(len(m.preview.windowData)) {
		return start, end
	}

	lineEnd := len(m.preview.windowData)
	if next := m.preview.vcur.Line + 1; next < len(m.preview.lineStarts) {
		lineEnd = m.preview.lineStarts[next]
	}
	if int(local) >= lineEnd {
		return start, end
	}
	match := ui.SearchBufferForward(pattern, m.preview.windowData[:lineEnd], int(local))
	if match == nil || match.Start != int(local) {
		return start, end
	}
	width := ui.PlainWidth(m.preview.windowData[match.Start:match.End])
	if width > 1 {
		end = start + width
	}
	return start, end
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// previewCursorDisplayCol is the cursor's display column on its line —
// rune index converted through display width, which differ as soon as
// the text is not ASCII.
func (m Model) previewCursorDisplayCol() int {
	lineStr := m.previewBuf().Line(m.preview.vcur.Line)
	rs := []rune(lineStr)
	col := m.preview.vcur.Col
	if col > len(rs) {
		col = len(rs)
	}
	if col < 0 {
		col = 0
	}
	return ui.PlainWidth([]byte(string(rs[:col])))
}

// previewCursorHighlight returns the cursor cell as a viewport-relative
// row and column range, ok=false when the cursor is scrolled out of
// view.
func (m Model) previewCursorHighlight() (row int, rng ui.ColumnRange, ok bool) {
	if !m.preview.vimMode || len(m.preview.lineStarts) == 0 {
		return 0, ui.ColumnRange{}, false
	}
	vp := m.preview.viewport
	row = m.preview.vcur.Line - vp.YOffset()
	if row < 0 || row >= vp.Height() {
		return 0, ui.ColumnRange{}, false
	}

	lineStr := m.previewBuf().Line(m.preview.vcur.Line)
	rs := []rune(lineStr)
	col := m.preview.vcur.Col
	cellW := 1
	if col >= 0 && col < len(rs) {
		cellW = ui.PlainWidth([]byte(string(rs[col : col+1])))
		if cellW < 1 {
			cellW = 1
		}
	}
	start := m.previewCursorDisplayCol() - vp.XOffset()
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

// applyPendingSnap lands a gg/G jump on the line's first non-blank,
// vim's rule for both. One-shot: motions and search hits sync through
// the same path and must not be snapped.
func (m *Model) applyPendingSnap() {
	if !m.preview.snapToLineStart {
		return
	}
	m.preview.snapToLineStart = false
	rs := []rune(m.previewBuf().Line(m.preview.vcur.Line))
	col := 0
	for i, r := range rs {
		if r != ' ' && r != '\t' {
			col = i
			break
		}
	}
	m.preview.vcur.Col = col
	m.preview.vcur.Want = col
	m.preview.cursor = m.previewByteFromVim()
}

// applyPreviewScrollOp is zz/zt/zb in the capture: the cursor line
// stays put and the view moves around it, through the same ScrollWindow
// arithmetic the lists use — scrolloff included.
func (m *Model) applyPreviewScrollOp(op vim.ScrollOp) {
	vp := &m.preview.viewport
	sw := ui.ScrollWindow{
		Cursor:    m.preview.vcur.Line,
		Offset:    vp.YOffset(),
		Height:    vp.Height(),
		Count:     len(m.preview.lineStarts),
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
