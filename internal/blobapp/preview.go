package blobapp

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/karlssonsimon/lazyaz/internal/appshell"
	"github.com/karlssonsimon/lazyaz/internal/azure/blob"
	"github.com/karlssonsimon/lazyaz/internal/keymap"
	"github.com/karlssonsimon/lazyaz/internal/ui"
	"github.com/karlssonsimon/lazyaz/internal/vim"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
)

const (
	previewBufferViewports = 10
	previewApproxLineBytes = 96
	previewMinWindowBytes  = int64(64 * 1024)
	previewMaxWindowBytes  = int64(2 * 1024 * 1024)
)

type previewState struct {
	open        bool
	blobName    string
	blobSize    int64
	contentType string
	binary      bool
	cursor      int64
	windowStart int64
	windowData  []byte
	lineStarts  []int
	rendered    string
	requestID   int
	viewport    viewport.Model
	search      previewSearchState
	// vcur is the window-local vim cursor; preview.cursor (the absolute
	// byte offset) is re-derived from it after every motion.
	vcur vim.Cursor
	// span is the v/V selection: a byte-offset anchor with the cursor
	// as the moving end.
	span vim.Span
	// snapToLineStart is a one-shot request from gg/G: once the window
	// holding the target is loaded and the cursor synced, snap to the
	// line's first non-blank — vim's landing rule for both jumps. The
	// byte target alone cannot express it, since the line's content is
	// only known after the window loads.
	snapToLineStart bool
	// vimMode gates the cursor. The preview opens in browse mode —
	// h backs out and j/k scroll, like every other pane — and v enters
	// the vim capture, where only vim keys work until esc.
	vimMode bool
	// formatted means = swapped the window for the pretty-printed
	// document; rawStash holds the raw view for the toggle back.
	formatted  bool
	formatKind string
	rawStash   *previewRawStash
}

func newPreviewState() previewState {
	vp := viewport.New()
	vp.SetWidth(40)
	vp.SetHeight(10)
	vp.SetContent("")
	return previewState{viewport: vp, search: previewSearchState{resumeAt: -1}}
}

// previewGutterMinDigits is the minimum digit width reserved for the
// line-number gutter beside the preview content. 3 gives a stable
// gutter for any file ≤ 999 lines; widens automatically beyond that.
const previewGutterMinDigits = 3

func (m *Model) resetPreviewState() {
	m.preview = newPreviewState()
	m.vimr.Clear()
	if m.focus == previewPane {
		m.transitionTo(blobsPane, false)
	}
}

func (m Model) openPreview(b blob.BlobEntry) (Model, tea.Cmd) {
	if b.IsPrefix {
		m.Notify(appshell.LevelInfo, "Open a blob file to preview")
		return m, nil
	}

	if !m.preview.open {
		m.preview.open = true
	}
	m.preview.blobName = b.Name
	m.preview.blobSize = b.Size
	m.preview.contentType = b.ContentType
	m.preview.binary = false
	m.preview.cursor = 0
	m.preview.vcur = vim.Cursor{}
	m.preview.span = vim.Span{}
	m.preview.vimMode = false
	m.preview.windowStart = 0
	m.preview.windowData = nil
	m.preview.lineStarts = nil
	m.preview.formatted = false
	m.preview.formatKind = ""
	m.preview.rawStash = nil
	m.preview.rendered = m.Styles.Muted.Render("Loading preview...")
	m.preview.viewport.SetContent(m.preview.rendered)
	m.preview.requestID++
	m.transitionTo(previewPane, false)
	m.StartLoading(previewPane, fmt.Sprintf("Loading preview for %s", b.Name))
	m.resize()

	cmd := loadPreviewWindowCmd(
		m.service,
		m.currentAccount,
		m.containerName,
		b.Name,
		0,
		b.Size,
		b.ContentType,
		max(1, m.preview.viewport.Height()),
		m.preview.requestID,
	)
	return m, tea.Batch(m.Spinner.Tick, cmd)
}

func (m Model) handlePreviewWindowLoaded(msg previewWindowLoadedMsg) (Model, tea.Cmd) {
	if !m.preview.open || !m.hasAccount || !m.hasContainer {
		return m, nil
	}
	if !sameAccount(m.currentAccount, msg.account) || m.containerName != msg.container || m.preview.blobName != msg.blobName {
		return m, nil
	}
	if msg.requestID != m.preview.requestID {
		return m, nil
	}

	m.ClearLoading()
	if msg.err != nil {
		m.ResolveSpinner(m.LoadingSpinnerID, appshell.LevelError, fmt.Sprintf("Failed to load preview for %s: %s", msg.blobName, msg.err.Error()))
		return m, nil
	}

	m.DismissSpinner(m.LoadingSpinnerID)
	m.preview.blobSize = msg.blobSize
	if strings.TrimSpace(msg.contentType) != "" {
		m.preview.contentType = msg.contentType
	}
	m.preview.windowStart = msg.windowStart
	m.preview.windowData = msg.data
	m.preview.cursor = clampInt64(msg.cursor, 0, maxInt64(0, msg.blobSize-1))
	m.preview.binary = ui.IsProbablyBinary(msg.data)
	m.preview.lineStarts = computeLineStarts(msg.data)
	m.preview.rendered = renderPreviewContent(msg.data, msg.blobName, m.preview.contentType, m.preview.binary, m.Styles)
	m.preview.viewport.SetContent(m.preview.rendered)
	m.syncPreviewVimFromByte()
	m.applyPendingSnap()
	m.followPreviewCursor()

	if m.preview.binary {
		m.Notify(appshell.LevelInfo, fmt.Sprintf("Binary preview for %s (%s)", msg.blobName, humanSize(msg.blobSize)))
	} else {
		m.Notify(appshell.LevelInfo, fmt.Sprintf("Previewing %s (%s)", msg.blobName, humanSize(msg.blobSize)))
	}

	return m, nil
}

func (m Model) handlePreviewKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	key := msg.String()

	// While the / prompt is open it owns every key, in both modes.
	if m.preview.search.bar.InputOpen {
		consumed, submitted := m.preview.search.bar.HandleKey(key, m.Keymap)
		if submitted {
			if err := m.preview.search.bar.Accept(); err != nil {
				m.Notify(appshell.LevelError, err.Error())
				return m, nil
			}
			return m.startPreviewSearch()
		}
		if consumed {
			return m, nil
		}
		return m, nil
	}

	if !m.preview.vimMode {
		return m.handlePreviewBrowseKey(key)
	}
	return m.handlePreviewVimKey(key)
}

// handlePreviewBrowseKey is the preview's default mode: it navigates
// like every other pane. h backs out, j/k scroll the view, no cursor is
// shown, and v enters the vim capture.
func (m Model) handlePreviewBrowseKey(key string) (Model, tea.Cmd) {
	switch m.vimr.GG(m.Keymap.JumpTopPrefix, key, true) {
	case vim.ChordFired:
		return m.jumpPreviewToTop()
	case vim.ChordArmed:
		m.Notify(appshell.LevelInfo, vim.HintGG)
		return m, nil
	}

	if m.vimr.Digit(key) {
		return m, nil
	}
	if !countedPreviewKey(m.Keymap, key) {
		m.vimr.ClearCount()
	}

	switch {
	case ui.ShouldQuit(key, m.Keymap.Quit, false):
		return m, tea.Quit
	case m.Keymap.SearchForward.Matches(key):
		m.preview.search.bar.Open(ui.SearchForward)
		return m, nil
	case m.Keymap.SearchBackward.Matches(key):
		m.preview.search.bar.Open(ui.SearchBackward)
		return m, nil
	case m.Keymap.SearchNext.Matches(key):
		return m.repeatPreviewSearch(m.preview.search.bar.Direction)
	case m.Keymap.SearchPrev.Matches(key):
		return m.repeatPreviewSearch(m.preview.search.bar.Direction.Opposite())
	case m.Keymap.FormatPreview.Matches(key):
		return m.togglePreviewFormat()
	// VisualChar is checked before ToggleVisualLine: the stock binding
	// for the latter is ["v","V"], so v must resolve first. V enters
	// the capture with a linewise selection already started.
	case m.Keymap.VisualChar.Matches(key):
		return m.enterPreviewVimMode(false)
	case m.Keymap.ToggleVisualLine.Matches(key):
		return m.enterPreviewVimMode(true)
	// h and left back out of the preview here, restoring the Miller
	// column muscle memory; inside the vim capture they are motions.
	// Esc alone dismisses an active search first — the first rung of
	// the ladder — while the navigation keys leave immediately.
	case m.Keymap.PreviewBack.Matches(key), m.Keymap.MotionLeft.Matches(key):
		if key == "esc" && m.preview.search.bar.Active() {
			m.preview.search.bar.Clear()
			return m, nil
		}
		m.transitionTo(blobsPane, false)
		return m, nil
	case m.Keymap.PreviewNextFocus.Matches(key):
		m.nextFocus()
		return m, nil
	case m.Keymap.PreviewPreviousFocus.Matches(key):
		m.previousFocus()
		return m, nil
	case m.Keymap.PreviewDown.Matches(key):
		return m.browseScrollPreview(m.vimr.TakeCount())
	case m.Keymap.PreviewUp.Matches(key):
		return m.browseScrollPreview(-m.vimr.TakeCount())
	case m.Keymap.HalfPageDown.Matches(key):
		step := max(1, m.preview.viewport.Height()/2)
		return m.browseScrollPreview(step * m.vimr.TakeCount())
	case m.Keymap.HalfPageUp.Matches(key):
		step := max(1, m.preview.viewport.Height()/2)
		return m.browseScrollPreview(-step * m.vimr.TakeCount())
	case m.Keymap.FullPageDown.Matches(key):
		return m.browseScrollPreview(max(1, m.preview.viewport.Height()) * m.vimr.TakeCount())
	case m.Keymap.FullPageUp.Matches(key):
		return m.browseScrollPreview(-max(1, m.preview.viewport.Height()) * m.vimr.TakeCount())
	case m.Keymap.ScrollLineDown.Matches(key):
		return m.browseScrollPreview(m.vimr.TakeCount())
	case m.Keymap.ScrollLineUp.Matches(key):
		return m.browseScrollPreview(-m.vimr.TakeCount())
	case m.Keymap.JumpBottom.Matches(key):
		return m.jumpPreviewToBottom()
	default:
		return m, nil
	}
}

// handlePreviewVimKey is the capture: only vim keys act, everything
// else is swallowed until esc walks back to browse mode. The app
// forwards every key here while the capture is on (IsTextInputActive),
// so tab switching and the other chrome shortcuts are blocked too.
func (m Model) handlePreviewVimKey(key string) (Model, tea.Cmd) {
	// yG and ygg are the two operator motions the engine cannot see:
	// they reach beyond the loaded window, so the preview resolves them
	// in byte space — the streamed yank and its budget already know how
	// to pay for the rest. Guarded so f-g still finds g and yi-g still
	// cancels as an unknown object.
	if m.vimr.OperatorPending() && !m.vimr.FindPending() && !m.vimr.ObjectPending() {
		switch m.vimr.GG(m.Keymap.JumpTopPrefix, key, true) {
		case vim.ChordFired:
			m.vimr.ConsumeOperator()
			m.vimr.ClearCount()
			return m.yankToTop()
		case vim.ChordArmed:
			return m, nil
		}
		if m.Keymap.JumpBottom.Matches(key) {
			m.vimr.ConsumeOperator()
			m.vimr.ClearCount()
			return m.yankToEnd()
		}
	}

	// Armed grammar state — a find target, the y operator, an i/a
	// object — owns the next key outright: it must be consumed before
	// the chords and before digit handling (f3 finds the character 3).
	if m.vimr.BufferPending() {
		act := m.vimr.BufferMotion(previewMotionKeys(m.Keymap), key, m.previewBuf(), m.preview.vcur, m.preview.span.Active)
		return m.applyBufferAction(act)
	}

	// The z chord repositions the view around the cursor, same resolver
	// state the lists use. It sits after the grammar so an armed
	// operator still owns z (and cancels on it), and before gg so an
	// armed z swallows its continuation.
	switch res, op := m.vimr.Scroll(m.Keymap, key); res {
	case vim.ChordArmed:
		m.vimr.ClearCount()
		m.Notify(appshell.LevelInfo, vim.HintScroll)
		return m, nil
	case vim.ChordSwallowed:
		return m, nil
	case vim.ChordFired:
		m.applyPreviewScrollOp(op)
		return m, nil
	}

	switch m.vimr.GG(m.Keymap.JumpTopPrefix, key, true) {
	case vim.ChordFired:
		return m.jumpPreviewToTop()
	case vim.ChordArmed:
		m.Notify(appshell.LevelInfo, vim.HintGG)
		return m, nil
	}

	if m.vimr.Digit(key) {
		return m, nil
	}
	if !countedPreviewKey(m.Keymap, key) {
		m.vimr.ClearCount()
	}

	// The buffer grammar resolves in one place — vim returns an
	// instruction, the preview applies it. A new motion or object added
	// to the engine reaches here without this file changing.
	if act := m.vimr.BufferMotion(previewMotionKeys(m.Keymap), key, m.previewBuf(), m.preview.vcur, m.preview.span.Active); act.Kind != vim.BufNone {
		return m.applyBufferAction(act)
	}

	switch {
	case m.Keymap.SearchForward.Matches(key):
		m.preview.search.bar.Open(ui.SearchForward)
		return m, nil
	case m.Keymap.SearchBackward.Matches(key):
		m.preview.search.bar.Open(ui.SearchBackward)
		return m, nil
	case m.Keymap.SearchNext.Matches(key):
		return m.repeatPreviewSearch(m.preview.search.bar.Direction)
	case m.Keymap.SearchPrev.Matches(key):
		return m.repeatPreviewSearch(m.preview.search.bar.Direction.Opposite())
	// VisualChar before ToggleVisualLine: the stock binding for the
	// latter is ["v","V"].
	case m.Keymap.VisualChar.Matches(key):
		return m.togglePreviewVisual(vim.SpanChar)
	case m.Keymap.ToggleVisualLine.Matches(key):
		return m.togglePreviewVisual(vim.SpanLine)
	case m.Keymap.PreviewYank.Matches(key):
		if m.preview.span.Active {
			return m.yankPreviewSelection()
		}
		m.vimr.ArmOperator()
		return m, nil
	case m.Keymap.FormatPreview.Matches(key):
		return m.togglePreviewFormat()
	// The esc ladder: search → visual → vim normal → browse. Leaving
	// the preview itself needs one more esc from browse mode.
	case key == "esc" && m.preview.search.bar.Active():
		m.preview.search.bar.Clear()
		return m, nil
	case m.preview.span.Active && m.Keymap.PreviewBack.Matches(key):
		m.preview.span.Stop()
		return m, nil
	case m.Keymap.PreviewBack.Matches(key):
		m.preview.vimMode = false
		m.preview.span.Stop()
		m.vimr.Clear()
		return m, nil
	case m.Keymap.HalfPageDown.Matches(key):
		step := max(1, m.preview.viewport.Height()/2)
		return m.applyPreviewCursor(vim.MoveDown(m.previewBuf(), m.preview.vcur, step*m.vimr.TakeCount()))
	case m.Keymap.HalfPageUp.Matches(key):
		step := max(1, m.preview.viewport.Height()/2)
		return m.applyPreviewCursor(vim.MoveUp(m.previewBuf(), m.preview.vcur, step*m.vimr.TakeCount()))
	case m.Keymap.FullPageDown.Matches(key):
		return m.applyPreviewCursor(vim.MoveDown(m.previewBuf(), m.preview.vcur, max(1, m.preview.viewport.Height())*m.vimr.TakeCount()))
	case m.Keymap.FullPageUp.Matches(key):
		return m.applyPreviewCursor(vim.MoveUp(m.previewBuf(), m.preview.vcur, max(1, m.preview.viewport.Height())*m.vimr.TakeCount()))
	// ctrl+e / ctrl+y scroll the view a line; the cursor is only pushed
	// when the view would leave it behind.
	case m.Keymap.ScrollLineDown.Matches(key):
		return m.scrollPreviewView(m.vimr.TakeCount())
	case m.Keymap.ScrollLineUp.Matches(key):
		return m.scrollPreviewView(-m.vimr.TakeCount())
	case m.Keymap.JumpBottom.Matches(key):
		return m.jumpPreviewToBottom()
	default:
		// Swallowed: the capture admits vim keys only.
		return m, nil
	}
}

// enterPreviewVimMode starts the capture with the cursor on the top
// visible line. withLineSelection also starts a linewise span, so V
// from browse mode lands selecting.
func (m Model) enterPreviewVimMode(withLineSelection bool) (Model, tea.Cmd) {
	m.preview.vimMode = true
	m.preview.vcur = vim.Cursor{Line: m.preview.viewport.YOffset()}
	m.preview.cursor = m.previewByteFromVim()
	if withLineSelection {
		m.preview.span.Start(m.preview.cursor, vim.SpanLine)
	}
	return m, nil
}

// browseScrollPreview scrolls the view without a cursor, keeping the
// byte cursor on the top visible line so search and vim-mode entry
// start where the user is looking, and sliding the window near edges.
func (m Model) browseScrollPreview(deltaLines int) (Model, tea.Cmd) {
	if !m.preview.open || deltaLines == 0 {
		return m, nil
	}
	vp := &m.preview.viewport
	if deltaLines > 0 {
		vp.ScrollDown(deltaLines)
	} else {
		vp.ScrollUp(-deltaLines)
	}
	m.preview.vcur = vim.Cursor{Line: vp.YOffset()}
	m.preview.cursor = m.previewByteFromVim()
	return m.ensurePreviewWindowAtCursor()
}

func (m Model) jumpPreviewToTop() (Model, tea.Cmd) {
	m.preview.cursor = 0
	m.preview.snapToLineStart = true
	return m.ensurePreviewWindowAtCursor()
}

func (m Model) jumpPreviewToBottom() (Model, tea.Cmd) {
	if m.preview.blobSize <= 0 {
		m.preview.cursor = 0
	} else {
		m.preview.cursor = m.preview.blobSize - 1
	}
	m.preview.snapToLineStart = true
	return m.ensurePreviewWindowAtCursor()
}

func (m Model) scrollPreviewView(delta int) (Model, tea.Cmd) {
	if !m.preview.open || delta == 0 {
		return m, nil
	}
	vp := &m.preview.viewport
	sw := ui.ScrollWindow{
		Cursor:    m.preview.vcur.Line,
		Offset:    vp.YOffset(),
		Height:    vp.Height(),
		Count:     len(m.preview.lineStarts),
		Scrolloff: m.scrolloff,
	}
	sw = sw.ScrollBy(delta)
	vp.SetYOffset(sw.Offset)
	nc := m.preview.vcur
	if sw.Cursor != nc.Line {
		nc = vim.MoveDown(m.previewBuf(), nc, sw.Cursor-nc.Line)
	}
	return m.applyPreviewCursor(nc)
}

func (m Model) ensurePreviewWindowAtCursor() (Model, tea.Cmd) {
	windowEnd := m.preview.windowStart + int64(len(m.preview.windowData))
	needLoad := false

	if len(m.preview.windowData) == 0 || m.preview.cursor < m.preview.windowStart || m.preview.cursor >= windowEnd {
		needLoad = true
	}

	if !needLoad && len(m.preview.lineStarts) > 0 {
		visible := max(1, m.preview.viewport.Height())
		local := m.previewLocalLine()
		if m.preview.windowStart > 0 && local < visible*previewBufferViewports {
			needLoad = true
		}
		if windowEnd < m.preview.blobSize && local > len(m.preview.lineStarts)-visible*(previewBufferViewports+1) {
			needLoad = true
		}
	}

	if needLoad {
		m.preview.requestID++
		m.StartLoading(previewPane, fmt.Sprintf("Loading preview window for %s", m.preview.blobName))
		cmd := loadPreviewWindowCmd(
			m.service,
			m.currentAccount,
			m.containerName,
			m.preview.blobName,
			m.preview.cursor,
			m.preview.blobSize,
			m.preview.contentType,
			max(1, m.preview.viewport.Height()),
			m.preview.requestID,
		)
		return m, tea.Batch(m.Spinner.Tick, cmd)
	}

	m.syncPreviewVimFromByte()
	m.applyPendingSnap()
	m.followPreviewCursor()
	return m, nil
}

func (m Model) previewLocalLine() int {
	if len(m.preview.lineStarts) == 0 {
		return 0
	}
	localOffset := int(clampInt64(m.preview.cursor-m.preview.windowStart, 0, int64(len(m.preview.windowData))))
	idx := sort.Search(len(m.preview.lineStarts), func(i int) bool {
		return m.preview.lineStarts[i] > localOffset
	})
	if idx == 0 {
		return 0
	}
	line := idx - 1
	if line >= len(m.preview.lineStarts) {
		return len(m.preview.lineStarts) - 1
	}
	return line
}

func computeLineStarts(data []byte) []int {
	if len(data) == 0 {
		return []int{0}
	}
	starts := []int{0}
	for i, b := range data {
		if b == '\n' && i+1 <= len(data) {
			starts = append(starts, i+1)
		}
	}
	return starts
}

func loadPreviewWindowCmd(
	svc *blob.Service,
	account blob.Account,
	containerName string,
	blobName string,
	cursor int64,
	knownSize int64,
	knownContentType string,
	visibleLines int,
	requestID int,
) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		size := knownSize
		contentType := knownContentType
		if size <= 0 || strings.TrimSpace(contentType) == "" {
			props, err := svc.GetBlobProperties(ctx, account, containerName, blobName)
			if err != nil {
				return previewWindowLoadedMsg{
					requestID: requestID,
					account:   account,
					container: containerName,
					blobName:  blobName,
					err:       err,
				}
			}
			size = props.Size
			if strings.TrimSpace(contentType) == "" {
				contentType = props.ContentType
			}
		}

		windowStart, windowCount := computePreviewWindow(size, cursor, visibleLines)
		data, err := svc.ReadBlobRange(ctx, account, containerName, blobName, windowStart, windowCount)
		return previewWindowLoadedMsg{
			requestID:   requestID,
			account:     account,
			container:   containerName,
			blobName:    blobName,
			blobSize:    size,
			contentType: contentType,
			windowStart: windowStart,
			cursor:      cursor,
			data:        data,
			err:         err,
		}
	}
}

func computePreviewWindow(totalSize, cursor int64, visibleLines int) (int64, int64) {
	if totalSize <= 0 {
		return 0, 0
	}

	visibleBytes := int64(max(1, visibleLines) * previewApproxLineBytes)
	bufferBytes := visibleBytes * previewBufferViewports
	windowSize := visibleBytes + 2*bufferBytes
	if windowSize < previewMinWindowBytes {
		windowSize = previewMinWindowBytes
	}
	if windowSize > previewMaxWindowBytes {
		windowSize = previewMaxWindowBytes
	}
	if windowSize > totalSize {
		windowSize = totalSize
	}

	anchored := clampInt64(cursor, 0, maxInt64(0, totalSize-1))
	start := anchored - bufferBytes
	if start < 0 {
		start = 0
	}
	if start+windowSize > totalSize {
		start = maxInt64(0, totalSize-windowSize)
	}

	return start, windowSize
}

// previewViewportRegion returns the screen bounds of the preview
// viewport so mouse coordinates can be translated to content positions.
// X starts AFTER the line-number gutter so a click on a number doesn't
// register as content selection.
func (m Model) previewViewportRegion() ui.ViewportRegion {
	previewX := 0
	for i := 0; i < previewPane; i++ {
		previewX += m.paneWidths[i]
	}
	gutterW := ui.LineGutterWidth(m.preview.viewport.TotalLineCount(), previewGutterMinDigits)

	// Flat Miller column viewport starts one row below the title.
	return ui.ViewportRegion{
		X:      previewX + gutterW,
		Y:      m.paneAreaY() + 1,
		Width:  m.preview.viewport.Width(),
		Height: m.preview.viewport.Height(),
	}
}

func renderPreviewContent(data []byte, blobName, contentType string, binary bool, styles ui.Styles) string {
	if binary {
		return styles.Warning.Render("Binary content preview is not supported.")
	}

	if len(data) == 0 {
		return styles.Muted.Render("(empty blob)")
	}

	text := string(data)
	return styles.Syntax.Highlight(blobName, contentType, text)
}

func clampInt64(v, minVal, maxVal int64) int64 {
	if v < minVal {
		return minVal
	}
	if v > maxVal {
		return maxVal
	}
	return v
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

// countedPreviewKey reports whether key is a motion that consumes a
// pending count in preview focus. Everything else drops the count.
// G is deliberately not counted here: absolute line N of a windowed
// gigabyte blob is unknowable without scanning it.
func countedPreviewKey(km keymap.Keymap, key string) bool {
	return km.PreviewDown.Matches(key) || km.PreviewUp.Matches(key) ||
		km.HalfPageDown.Matches(key) || km.HalfPageUp.Matches(key) ||
		km.FullPageDown.Matches(key) || km.FullPageUp.Matches(key) ||
		km.ScrollLineDown.Matches(key) || km.ScrollLineUp.Matches(key) ||
		km.MotionLeft.Matches(key) || km.MotionRight.Matches(key) ||
		km.MotionWordForward.Matches(key) || km.MotionWordBack.Matches(key) ||
		km.MotionWordEnd.Matches(key) || km.MotionLineEnd.Matches(key) ||
		km.FindChar.Matches(key) || km.FindCharBack.Matches(key) ||
		km.TillChar.Matches(key) || km.TillCharBack.Matches(key) ||
		km.RepeatFind.Matches(key) || km.RepeatFindBack.Matches(key) ||
		km.MotionBigWord.Matches(key) || km.MotionBigWordBack.Matches(key) ||
		km.MotionBigWordEnd.Matches(key) || km.PreviewYank.Matches(key) ||
		km.MotionUnderscore.Matches(key)
}
