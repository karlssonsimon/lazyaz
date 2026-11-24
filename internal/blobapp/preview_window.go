package blobapp

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/karlssonsimon/lazyaz/internal/appshell"
	"github.com/karlssonsimon/lazyaz/internal/azure/blob"
	"github.com/karlssonsimon/lazyaz/internal/ui"

	tea "charm.land/bubbletea/v2"
)

// The preview holds one window of the blob in memory: a byte range
// around the cursor, cut to whole lines. Moving inside the window costs
// nothing. When the view comes within a quarter of the window's lines of
// an edge the blob continues past, the window is fetched again around
// the cursor in the background, and swapped in with the view put back on
// the same line — so scrolling never waits on the network unless it
// outruns it.
//
// That only works while the window is many screens long, and a window is
// requested in bytes while a screen is measured in lines. So its size
// follows the lines it turns out to hold: previewWindowScreens screens
// of lines as long as the loaded ones, within a floor that keeps opening
// a blob quick and a ceiling that keeps a window cheap to highlight.

const (
	previewWindowBytes    = int64(512 * 1024)
	previewMaxWindowBytes = int64(2 * 1024 * 1024)
	previewWindowScreens  = 32

	// previewSlideFraction is how far the cursor must be from the point
	// the window was built around before a new window is worth fetching,
	// as a fraction of the window. Measured in bytes on purpose: a
	// window of a few very long lines is always "near an edge" by line
	// count, and without this every keypress there would fetch the same
	// window again.
	previewSlideFraction = 8

	// previewLongLineBytes is the most of a single line that is loaded
	// around the cursor. Every cursor motion and every frame costs time
	// in proportion to the length of the line it touches, so a line
	// longer than this — a minified document, typically — is shown one
	// piece at a time.
	previewLongLineBytes = 128 * 1024

	// previewLineCountBudget caps what is read just to number lines after
	// a jump. The reads are billed, and line numbers are not worth a
	// download the user did not ask for beyond this.
	previewLineCountBudget = int64(64 << 20)
)

// previewSource is the open blob as the preview reads it. Narrow so the
// windowing can be driven by a synthetic blob.
type previewSource interface {
	rangeReader
	Properties(ctx context.Context) (blob.BlobProperties, error)
}

// previewLineAnchor is a position in the blob whose line is known: line
// newlines precede offset.
type previewLineAnchor struct {
	offset int64
	line   int
	ok     bool
}

type previewLoadRequest struct {
	source      previewSource
	account     blob.Account
	container   string
	blobName    string
	blobSize    int64
	contentType string
	// cursor is the byte the window is built around, windowBytes its
	// size.
	cursor      int64
	windowBytes int64
	// decodeLimit is the largest blob that is fetched whole to be
	// decoded from UTF-16.
	decodeLimit int64
	styles      ui.Styles
	openSeq     int
	requestID   int
}

type previewWindowLoadedMsg struct {
	openSeq     int
	requestID   int
	account     blob.Account
	container   string
	blobName    string
	blobSize    int64
	contentType string
	// anchor is the cursor the window was built around.
	anchor      int64
	windowStart int64
	data        []byte
	lineStarts  []int
	rendered    string
	binary      bool
	// decoded means data is the whole blob converted from UTF-16, not a
	// byte range of it.
	decoded bool
	err     error
}

// previewLinesCountedMsg reports that line newlines precede offset.
type previewLinesCountedMsg struct {
	openSeq int
	offset  int64
	line    int
	err     error
}

// previewWindowAround returns the byte range of the window for a cursor:
// windowBytes centered on it, shifted to stay inside the blob.
func previewWindowAround(blobSize, cursor, windowBytes int64) (start, count int64) {
	if blobSize <= 0 {
		return 0, 0
	}
	count = windowBytes
	if count > blobSize {
		count = blobSize
	}
	start = clampInt64(cursor-count/2, 0, blobSize-count)
	return start, count
}

// trimWindowToLines drops the partial line at each end of a window that
// starts or stops inside the blob, so the window holds whole lines and
// never opens on half a line or half a rune. The line the cursor is on
// is kept even when it is cut: a line longer than the window has to be
// shown in pieces.
func trimWindowToLines(data []byte, start, blobSize, cursor int64) ([]byte, int64) {
	if start > 0 {
		if i := bytes.IndexByte(data, '\n'); i >= 0 && start+int64(i) < cursor {
			data = data[i+1:]
			start += int64(i + 1)
		}
	}
	if start+int64(len(data)) < blobSize {
		if i := bytes.LastIndexByte(data, '\n'); i >= 0 && start+int64(i) >= cursor {
			data = data[:i+1]
		}
	}
	return data, start
}

// cutWindowToLongLine reduces a window to previewLongLineBytes around
// the cursor when the cursor's line, as far as it was loaded, is longer
// than that.
func cutWindowToLongLine(data []byte, start, cursor int64) ([]byte, int64) {
	local := int(clampInt64(cursor-start, 0, int64(len(data))))
	lineStart := bytes.LastIndexByte(data[:local], '\n') + 1
	lineEnd := len(data)
	if i := bytes.IndexByte(data[local:], '\n'); i >= 0 {
		lineEnd = local + i + 1
	}
	if lineEnd-lineStart <= previewLongLineBytes {
		return data, start
	}

	from := local - previewLongLineBytes/2
	if from > len(data)-previewLongLineBytes {
		from = len(data) - previewLongLineBytes
	}
	if from < 0 {
		from = 0
	}
	to := from + previewLongLineBytes

	// The piece is cut between runes, not through one.
	for from < local && !utf8.RuneStart(data[from]) {
		from++
	}
	for to < len(data) && to > local+1 && !utf8.RuneStart(data[to]) {
		to--
	}
	return data[from:to], start + int64(from)
}

// loadPreviewWindowCmd fetches the window around req.cursor and prepares
// everything derived from its bytes. Highlighting a window takes long
// enough to drop frames, so it is done here rather than in Update.
func loadPreviewWindowCmd(req previewLoadRequest) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		msg := previewWindowLoadedMsg{
			openSeq:     req.openSeq,
			requestID:   req.requestID,
			account:     req.account,
			container:   req.container,
			blobName:    req.blobName,
			blobSize:    req.blobSize,
			contentType: req.contentType,
			anchor:      req.cursor,
		}

		if msg.blobSize <= 0 || strings.TrimSpace(msg.contentType) == "" {
			props, err := req.source.Properties(ctx)
			if err != nil {
				msg.err = err
				return msg
			}
			msg.blobSize = props.Size
			if strings.TrimSpace(msg.contentType) == "" {
				msg.contentType = props.ContentType
			}
		}

		start, count := previewWindowAround(msg.blobSize, req.cursor, req.windowBytes)
		var data []byte
		if count > 0 {
			var err error
			data, err = req.source.ReadBlobRange(ctx, start, count)
			if err != nil {
				msg.err = err
				return msg
			}
			if len(data) == 0 {
				msg.err = errors.New("the blob returned no data")
				return msg
			}
		}

		// UTF-16 has a NUL in every other byte and cannot be windowed by
		// line, so a blob small enough is fetched whole and decoded; the
		// preview then holds the entire text in memory.
		if start == 0 && ui.HasUTF16BOM(data) && msg.blobSize <= req.decodeLimit {
			whole := data
			if int64(len(whole)) < msg.blobSize {
				var err error
				whole, err = req.source.ReadBlobRange(ctx, 0, msg.blobSize)
				if err != nil {
					msg.err = err
					return msg
				}
			}
			if text, err := ui.DecodeUTF16(whole); err == nil {
				data = text
				msg.decoded = true
			}
		}
		if !msg.decoded {
			data, start = cutWindowToLongLine(data, start, req.cursor)
			data, start = trimWindowToLines(data, start, msg.blobSize, req.cursor)
		}

		msg.windowStart = start
		msg.data = data
		msg.binary = ui.IsProbablyBinary(data)
		msg.lineStarts = computeLineStarts(data)
		msg.rendered = renderPreviewContent(data, req.blobName, msg.contentType, msg.binary, req.styles)
		return msg
	}
}

// requestPreviewWindow asks for the window around the cursor, replacing
// any request still in flight.
func (m Model) requestPreviewWindow() (Model, tea.Cmd) {
	if m.preview.source == nil {
		return m, nil
	}

	m.preview.requestID++
	m.preview.loading = true
	windowBytes := m.previewWindowSize()
	start, count := previewWindowAround(m.preview.blobSize, m.preview.cursor, windowBytes)
	m.preview.loadStart = start
	m.preview.loadEnd = start + count
	if m.preview.blobSize <= 0 {
		// The size is looked up by the load itself; until it lands the
		// request stands for wherever the cursor goes.
		m.preview.loadEnd = math.MaxInt64
	}

	cmd := loadPreviewWindowCmd(previewLoadRequest{
		source:      m.preview.source,
		account:     m.currentAccount,
		container:   m.containerName,
		blobName:    m.preview.blobName,
		blobSize:    m.preview.blobSize,
		contentType: m.preview.contentType,
		cursor:      m.preview.cursor,
		windowBytes: windowBytes,
		decodeLimit: m.formatBudget,
		styles:      m.Styles,
		openSeq:     m.preview.openSeq,
		requestID:   m.preview.requestID,
	})

	// A window fetched ahead of the view is not something to wait for;
	// only a cursor with nothing loaded under it gets the spinner.
	if m.previewHoldsCursor() {
		m.preview.loadSpinner = 0
		return m, cmd
	}
	m.StartLoading(previewPane, fmt.Sprintf("Loading preview for %s", m.preview.blobName))
	m.preview.loadSpinner = m.LoadingSpinnerID
	return m, tea.Batch(m.Spinner.Tick, cmd)
}

// previewWindowSize is how many bytes the next window should span to
// hold previewWindowScreens screens, judged by the lines loaded now.
// Before anything is loaded there is nothing to judge by, and the floor
// is the guess.
func (m Model) previewWindowSize() int64 {
	lines := int64(len(m.preview.lineStarts))
	// A window of one line is a piece of a line too long to load whole;
	// no size would bring more lines into it.
	if lines <= 1 || m.preview.binary {
		return previewWindowBytes
	}
	bytesPerLine := int64(len(m.preview.windowData))/lines + 1
	screen := int64(max(1, m.preview.viewport.Height()))
	return clampInt64(bytesPerLine*screen*previewWindowScreens, previewWindowBytes, previewMaxWindowBytes)
}

// previewRunningAhead reports the browse-mode state where the position
// has moved past the top visible line: the view is stopped at the end of
// the loaded window, and the blob goes on below it.
func (m Model) previewRunningAhead() bool {
	if m.preview.vimMode {
		return false
	}
	top := m.preview.viewport.YOffset()
	lastTop := max(0, len(m.preview.lineStarts)-m.preview.viewport.Height())
	end := m.preview.windowStart + int64(len(m.preview.windowData))
	return top >= lastTop && end < m.preview.blobSize && m.preview.vcur.Line > top
}

// previewHoldsCursor reports whether the loaded window contains the
// cursor. It does not while a jump is waiting for its window.
func (m Model) previewHoldsCursor() bool {
	if len(m.preview.lineStarts) == 0 {
		return false
	}
	if m.preview.blobSize <= 0 {
		return true
	}
	start := m.preview.windowStart
	end := start + int64(len(m.preview.windowData))
	return m.preview.cursor >= start && m.preview.cursor < end
}

// previewNeedsWindow decides whether to fetch the window around the
// cursor: when the cursor has nothing loaded under it, or when the view
// is running out of loaded lines on a side where the blob continues.
func (m Model) previewNeedsWindow() bool {
	if m.preview.source == nil {
		return false
	}
	cursor := m.preview.cursor
	if m.preview.loading && cursor >= m.preview.loadStart && cursor < m.preview.loadEnd {
		// What is on its way already covers the cursor.
		return false
	}
	if !m.previewHoldsCursor() {
		return true
	}

	start := m.preview.windowStart
	end := start + int64(len(m.preview.windowData))
	lines := len(m.preview.lineStarts)
	height := max(1, m.preview.viewport.Height())
	margin := max(lines/4, height)
	top := m.preview.viewport.YOffset()

	nearTop := start > 0 && top < margin
	nearBottom := end < m.preview.blobSize && top+height > lines-margin
	if !nearTop && !nearBottom {
		return false
	}

	moved := cursor - m.preview.windowAnchor
	if moved < 0 {
		moved = -moved
	}
	return moved >= (end-start)/previewSlideFraction
}

// previewAtWindowEdge reports whether a move in the direction of delta
// from the given loaded line has no loaded line to go to, although the
// blob continues that way and no window is on its way. A window reaches
// far past the cursor on both sides and follows it, so this happens next
// to a line too long to be loaded whole, or inside one.
func (m Model) previewAtWindowEdge(line, delta int) bool {
	if m.preview.source == nil || m.preview.loading || !m.previewHoldsCursor() {
		return false
	}
	if delta > 0 {
		end := m.preview.windowStart + int64(len(m.preview.windowData))
		return line >= len(m.preview.lineStarts)-1 && end < m.preview.blobSize
	}
	return line <= 0 && m.preview.windowStart > 0
}

// stepPreviewPastWindow moves the cursor to the byte next to the loaded
// window, which brings the window around it. Going down that byte opens
// the next line, or the next piece of a line shown in pieces. Going up
// it ends the line or piece before, and the cursor is put at the start
// of that once it is loaded.
func (m Model) stepPreviewPastWindow(delta int) (Model, tea.Cmd) {
	if delta > 0 {
		m.preview.cursor = m.preview.windowStart + int64(len(m.preview.windowData))
	} else {
		m.preview.cursor = m.preview.windowStart - 1
		m.preview.snapToLineStart = true
	}
	return m.ensurePreviewWindowAtCursor()
}

// ensurePreviewWindowAtCursor is the tail of everything that moves the
// cursor: bring the view along when the cursor's line is loaded, and
// fetch a new window when it is not or soon will not be.
func (m Model) ensurePreviewWindowAtCursor() (Model, tea.Cmd) {
	if m.previewHoldsCursor() {
		m.syncPreviewVimFromByte()
		m.applyPendingSnap()
		m.followPreviewCursor()
	}
	if !m.previewNeedsWindow() {
		return m, nil
	}
	return m.requestPreviewWindow()
}

func (m Model) handlePreviewWindowLoaded(msg previewWindowLoadedMsg) (Model, tea.Cmd) {
	if !m.preview.open || !m.hasAccount || !m.hasContainer {
		return m, nil
	}
	if !sameAccount(m.currentAccount, msg.account) || m.containerName != msg.container || m.preview.blobName != msg.blobName {
		return m, nil
	}
	if msg.openSeq != m.preview.openSeq || msg.requestID != m.preview.requestID {
		return m, nil
	}

	m.preview.loading = false

	// Another operation may have taken over the pane's spinner while
	// this window loaded; only the load's own spinner is its to clear.
	spinner := m.preview.loadSpinner
	m.preview.loadSpinner = 0
	ownsSpinner := spinner != 0 && m.Loading && m.LoadingSpinnerID == spinner
	if ownsSpinner {
		m.ClearLoading()
	}

	if msg.err != nil {
		text := fmt.Sprintf("Failed to load preview for %s: %s", msg.blobName, msg.err.Error())
		if ownsSpinner {
			m.ResolveSpinner(spinner, appshell.LevelError, text)
		} else {
			m.Notify(appshell.LevelError, text)
		}
		// A jump whose window never arrived leaves the cursor pointing
		// at bytes that are not loaded. Put it back where the view is.
		if len(m.preview.lineStarts) > 0 && !m.previewHoldsCursor() {
			m.preview.snapToLineStart = false
			m.preview.cursor = m.previewByteFromVim()
		}
		return m, nil
	}
	if ownsSpinner {
		m.DismissSpinner(spinner)
	}

	docSize := msg.blobSize
	if msg.decoded {
		docSize = int64(len(msg.data))
	}
	start := msg.windowStart
	end := start + int64(len(msg.data))
	cursor := clampInt64(m.preview.cursor, 0, maxInt64(0, docSize-1))
	if docSize > 0 && (cursor < start || cursor >= end) {
		if msg.anchor != m.preview.cursor {
			// The cursor moved on while this window was loading.
			return m.ensurePreviewWindowAtCursor()
		}
		// The blob no longer has the bytes the cursor pointed at.
		cursor = clampInt64(cursor, start, maxInt64(start, end-1))
	}

	// The top visible line as a blob offset, so the view can be put back
	// on it in a window whose lines are indexed differently.
	first := len(m.preview.lineStarts) == 0
	topByte := int64(-1)
	if !first {
		top := min(m.preview.viewport.YOffset(), len(m.preview.lineStarts)-1)
		topByte = m.preview.windowStart + int64(m.preview.lineStarts[top])
	}
	ranAhead := m.previewRunningAhead()
	oldAnchor := m.previewWindowLineAnchor(start)

	m.preview.blobSize = docSize
	if strings.TrimSpace(msg.contentType) != "" {
		m.preview.contentType = msg.contentType
	}
	m.preview.cursor = cursor
	m.preview.windowStart = start
	m.preview.windowData = msg.data
	m.preview.windowAnchor = msg.anchor
	m.preview.lineStarts = msg.lineStarts
	m.preview.binary = msg.binary
	m.preview.decoded = msg.decoded
	m.preview.rendered = msg.rendered
	if m.preview.restyle {
		// The theme changed after the load captured its styles.
		m.preview.restyle = false
		m.preview.rendered = renderPreviewContent(msg.data, msg.blobName, m.preview.contentType, msg.binary, m.Styles)
	}
	m.preview.viewport.SetContent(m.preview.rendered)

	m.preview.lineBase = 0
	m.preview.lineBaseKnown = false
	m.preview.lineAnchor = betterLineAnchor(m.preview.lineAnchor, oldAnchor, start, end)
	m, lineCmd := m.resolvePreviewLineBase()

	// The gutter is as wide as its largest line number, and the viewport
	// gets what is left.
	m.resize()

	snapped := m.preview.snapToLineStart
	m.syncPreviewVimFromByte()
	m.applyPendingSnap()
	if snapped {
		// The snap is where the jump lands, not travel away from the
		// point the window was built around.
		m.preview.windowAnchor = m.preview.cursor
	}
	switch {
	case ranAhead:
		// The view was held back by the end of the old window; now it
		// can catch up with the position.
		m.preview.viewport.SetYOffset(m.preview.vcur.Line)
	case topByte >= start && topByte < end:
		m.preview.viewport.SetYOffset(m.previewLineOf(int(topByte - start)))
	default:
		m.centerPreviewOnCursor()
	}
	m.followPreviewCursor()

	if first {
		if m.preview.binary {
			m.Notify(appshell.LevelInfo, fmt.Sprintf("Binary preview for %s (%s)", msg.blobName, humanSize(msg.blobSize)))
		} else {
			m.Notify(appshell.LevelInfo, fmt.Sprintf("Previewing %s (%s)", msg.blobName, humanSize(msg.blobSize)))
		}
	}

	m, slideCmd := m.ensurePreviewWindowAtCursor()
	return m, tea.Batch(lineCmd, slideCmd)
}

// previewLineBase is the number of lines before the loaded window, and
// whether that is known. A window at the start of the blob has none
// before it by definition.
func (m Model) previewLineBase() (int, bool) {
	if m.preview.windowStart == 0 {
		return 0, true
	}
	return m.preview.lineBase, m.preview.lineBaseKnown
}

// previewLinesBefore is the number of newlines before a blob offset, when
// what is loaded or remembered can tell.
func (m Model) previewLinesBefore(offset int64) (int, bool) {
	if offset == 0 {
		return 0, true
	}
	if a := m.preview.lineAnchor; a.ok && a.offset == offset {
		return a.line, true
	}
	base, known := m.previewLineBase()
	start := m.preview.windowStart
	end := start + int64(len(m.preview.windowData))
	if !known || offset < start || offset > end {
		return 0, false
	}
	return base + bytes.Count(m.preview.windowData[:offset-start], newline), true
}

// previewWindowLineAnchor expresses what the loaded window knows about
// line numbers as a single position, picked to be useful to a window
// that will start at nextStart: nextStart itself when the two overlap,
// the loaded window's start otherwise.
func (m Model) previewWindowLineAnchor(nextStart int64) previewLineAnchor {
	base, known := m.previewLineBase()
	if !known || len(m.preview.lineStarts) == 0 {
		return previewLineAnchor{}
	}
	if line, ok := m.previewLinesBefore(nextStart); ok {
		return previewLineAnchor{offset: nextStart, line: line, ok: true}
	}
	return previewLineAnchor{offset: m.preview.windowStart, line: base, ok: true}
}

// betterLineAnchor picks the anchor that serves the window [start, end]
// best. One inside the window settles its numbering outright; failing
// that, the one that leaves fewer bytes to count up to the window.
// Counting only runs forward, so an anchor past the window is of no use.
func betterLineAnchor(a, b previewLineAnchor, start, end int64) previewLineAnchor {
	rank := func(x previewLineAnchor) int64 {
		if !x.ok || x.offset > end {
			return -1
		}
		if x.offset >= start {
			return math.MaxInt64
		}
		return x.offset
	}
	if rank(b) > rank(a) {
		return b
	}
	return a
}

// resolvePreviewLineBase works out the loaded window's line numbers from
// the anchor when it lies inside the window, and otherwise counts the
// newlines between the anchor and the window in the background — within
// a budget, since that reads bytes nobody asked to see.
func (m Model) resolvePreviewLineBase() (Model, tea.Cmd) {
	if _, known := m.previewLineBase(); known || len(m.preview.lineStarts) == 0 {
		return m, nil
	}

	start := m.preview.windowStart
	end := start + int64(len(m.preview.windowData))
	a := m.preview.lineAnchor
	if a.ok && a.offset >= start && a.offset <= end {
		m.preview.lineBase = a.line - bytes.Count(m.preview.windowData[:a.offset-start], newline)
		m.preview.lineBaseKnown = true
		m.resize()
		return m, nil
	}

	if m.preview.countingLines || m.preview.source == nil {
		return m, nil
	}
	from, fromLine := int64(0), 0
	if a.ok && a.offset <= start {
		from, fromLine = a.offset, a.line
	}
	if start-from > min(m.searchScanBudget, previewLineCountBudget) {
		return m, nil
	}
	m.preview.countingLines = true
	return m, countLinesCmd(m.preview.source, from, fromLine, start, m.preview.openSeq)
}

// countLinesCmd counts the newlines in [from, to), given that fromLine
// of them precede from.
func countLinesCmd(reader rangeReader, from int64, fromLine int, to int64, openSeq int) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), searchScanTimeout)
		defer cancel()

		line := fromLine
		for pos := from; pos < to; {
			count := to - pos
			if count > searchChunkBytes {
				count = searchChunkBytes
			}
			data, err := reader.ReadBlobRange(ctx, pos, count)
			if err != nil {
				return previewLinesCountedMsg{openSeq: openSeq, err: err}
			}
			if len(data) == 0 {
				return previewLinesCountedMsg{openSeq: openSeq, err: errors.New("the blob returned no data")}
			}
			line += bytes.Count(data, newline)
			pos += int64(len(data))
		}
		return previewLinesCountedMsg{openSeq: openSeq, offset: to, line: line}
	}
}

func (m Model) handlePreviewLinesCounted(msg previewLinesCountedMsg) (Model, tea.Cmd) {
	if !m.preview.open || msg.openSeq != m.preview.openSeq {
		return m, nil
	}
	m.preview.countingLines = false
	if msg.err != nil {
		// Line numbers are a courtesy; the gutter stays unnumbered.
		return m, nil
	}
	m.preview.lineAnchor = previewLineAnchor{offset: msg.offset, line: msg.line, ok: true}
	return m.resolvePreviewLineBase()
}

// previewGutterWidth is the width of the line-number gutter beside the
// loaded window.
func (m Model) previewGutterWidth() int {
	base, _ := m.previewLineBase()
	return ui.LineGutterWidth(base+m.preview.viewport.TotalLineCount(), previewGutterMinDigits)
}

// previewFirstLineNumber is the blob's line number for the window's
// first line, 0 while it is not known.
func (m Model) previewFirstLineNumber() int {
	base, known := m.previewLineBase()
	if !known {
		return 0
	}
	return base + 1
}

// computeLineStarts indexes the lines of a window. A trailing newline
// ends the last line; it does not begin another one.
func computeLineStarts(data []byte) []int {
	starts := []int{0}
	for i, b := range data {
		if b == '\n' && i+1 < len(data) {
			starts = append(starts, i+1)
		}
	}
	return starts
}

func renderPreviewContent(data []byte, blobName, contentType string, binary bool, styles ui.Styles) string {
	if binary {
		return styles.Warning.Render("Binary content preview is not supported.")
	}

	if len(data) == 0 {
		return styles.Muted.Render("(empty blob)")
	}

	// The final line ending is dropped so the viewport holds exactly the
	// lines computeLineStarts indexed, without an empty one after them.
	text := strings.TrimSuffix(string(data), "\n")
	text = strings.TrimSuffix(text, "\r")
	return styles.Syntax.Highlight(blobName, contentType, text)
}
