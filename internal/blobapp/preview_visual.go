package blobapp

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/karlssonsimon/lazyaz/internal/appshell"
	"github.com/karlssonsimon/lazyaz/internal/ui"
	"github.com/karlssonsimon/lazyaz/internal/vim"

	tea "charm.land/bubbletea/v2"
)

// togglePreviewVisual is v and V. Starting anchors at the cursor;
// pressing the active mode's key again leaves visual; pressing the
// other switches mode in place, as in vim.
func (m Model) togglePreviewVisual(mode vim.SpanMode) (Model, tea.Cmd) {
	span := &m.preview.span
	if span.Active && span.Mode == mode {
		span.Stop()
		return m, nil
	}
	if span.Active {
		span.Mode = mode
		return m, nil
	}
	span.Start(m.preview.cursor, mode)
	return m, nil
}

// previewSelectionByteRange is the selection in absolute byte space,
// with charwise extended over the cursor-end rune and linewise extended
// to line bounds within the loaded window.
func (m Model) previewSelectionByteRange() (lo, hi int64, ok bool) {
	if !m.preview.span.Active {
		return 0, 0, false
	}
	lo, hi = m.preview.span.Range(m.preview.cursor)

	if m.preview.span.Mode == vim.SpanLine {
		lo, hi = m.extendToLineBounds(lo, hi)
		return lo, hi, true
	}

	// Charwise includes the rune under hi.
	hi += int64(m.runeLenAt(hi))
	return lo, hi, true
}

// runeLenAt is the byte length of the rune starting at the given
// absolute offset, 1 when it is outside the loaded window.
func (m Model) runeLenAt(off int64) int {
	local := off - m.preview.windowStart
	if local < 0 || local >= int64(len(m.preview.windowData)) {
		return 1
	}
	_, n := utf8.DecodeRune(m.preview.windowData[local:])
	if n < 1 {
		return 1
	}
	return n
}

// extendToLineBounds widens [lo, hi] to whole lines using the loaded
// window's line index; ends outside the window clamp to the window
// edges, which the yank path re-widens against fetched data.
func (m Model) extendToLineBounds(lo, hi int64) (int64, int64) {
	starts := m.preview.lineStarts
	if len(starts) == 0 {
		return lo, hi
	}
	winLo := m.preview.windowStart
	winHi := winLo + int64(len(m.preview.windowData))

	if lo >= winLo && lo < winHi {
		line := m.previewLineOf(int(lo - winLo))
		lo = winLo + int64(starts[line])
	}
	if hi >= winLo && hi < winHi {
		line := m.previewLineOf(int(hi - winLo))
		end := len(m.preview.windowData)
		if line+1 < len(starts) {
			end = starts[line+1]
		}
		hi = winLo + int64(end)
	}
	return lo, hi
}

// previewSelectionRanges maps the selection onto viewport rows and
// columns for rendering, translated through both scroll offsets.
func (m Model) previewSelectionRanges() map[int][]ui.ColumnRange {
	lo, hi, ok := m.previewSelectionByteRange()
	if !ok || len(m.preview.lineStarts) == 0 {
		return nil
	}
	vp := m.preview.viewport
	winLo := m.preview.windowStart
	buf := m.previewBuf()

	localLo := int(clampInt64(lo-winLo, 0, int64(len(m.preview.windowData))))
	localHi := int(clampInt64(hi-winLo, 0, int64(len(m.preview.windowData))))
	loLine := m.previewLineOf(localLo)
	hiLine := m.previewLineOf(maxInt(localHi-1, 0))

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
		if m.preview.span.Mode == vim.SpanChar {
			ls := m.preview.lineStarts[line]
			if line == loLine && localLo > ls {
				start = ui.PlainWidth([]byte(lineStr[:minInt(localLo-ls, len(lineStr))]))
			}
			if line == hiLine {
				end = ui.PlainWidth([]byte(lineStr[:minInt(localHi-ls, len(lineStr))]))
			}
		}
		if end == start {
			end = start + 1 // an empty line still shows a selected cell
		}

		start -= vp.XOffset()
		end -= vp.XOffset()
		if end <= 0 || start >= vp.Width() {
			continue
		}
		byRow[row] = []ui.ColumnRange{{
			Start: maxInt(start, 0),
			End:   minInt(end, vp.Width()),
			Style: m.Styles.SelectionHighlight,
		}}
	}
	return byRow
}

// yankDoneMsg carries a streamed yank back to Update.
type yankDoneMsg struct {
	text string
	err  error
}

// yankPreviewSelection is y: assemble the selected bytes and hand them
// to the clipboard. In-window selections cost nothing; anything wider
// streams through ReadBlobRange, bounded by the yank budget — over it,
// refuse outright rather than yank a silent partial.
func (m Model) yankPreviewSelection() (Model, tea.Cmd) {
	lo, hi, ok := m.previewSelectionByteRange()
	if !ok {
		m.Notify(appshell.LevelInfo, "Nothing selected — start with v or V")
		return m, nil
	}
	m.preview.span.Stop()

	if hi-lo > m.yankBudget {
		m.Notify(appshell.LevelWarn, fmt.Sprintf(
			"Selection is %s — over the yank limit (%s)", humanSize(hi-lo), humanSize(m.yankBudget),
		))
		return m, nil
	}

	winLo := m.preview.windowStart
	winHi := winLo + int64(len(m.preview.windowData))
	if lo >= winLo && hi <= winHi {
		text := string(m.preview.windowData[lo-winLo : hi-winLo])
		return m.copyYankedText(text)
	}

	if m.service == nil {
		m.Notify(appshell.LevelError, "Selection reaches outside the loaded window and no service is available")
		return m, nil
	}
	m.StartLoading(previewPane, fmt.Sprintf("Yanking %s", humanSize(hi-lo)))
	reader := serviceRangeReader{
		svc:       m.service,
		account:   m.currentAccount,
		container: m.containerName,
		blobName:  m.preview.blobName,
	}
	return m, tea.Batch(m.Spinner.Tick, yankRangeCmd(reader, lo, hi))
}

func yankRangeCmd(reader rangeReader, lo, hi int64) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), searchScanTimeout)
		defer cancel()
		data, err := reader.ReadBlobRange(ctx, lo, hi-lo)
		if err != nil {
			return yankDoneMsg{err: err}
		}
		return yankDoneMsg{text: string(data)}
	}
}

func (m Model) handleYankDone(msg yankDoneMsg) (Model, tea.Cmd) {
	m.ClearLoading()
	if msg.err != nil {
		m.ResolveSpinner(m.LoadingSpinnerID, appshell.LevelError, fmt.Sprintf("Yank failed: %s", msg.err.Error()))
		return m, nil
	}
	m.DismissSpinner(m.LoadingSpinnerID)
	return m.copyYankedText(msg.text)
}

// copyYankedText writes to the clipboard off the UI thread and reports
// what was yanked in vim's terms: lines for a linewise feel, bytes
// otherwise.
func (m Model) copyYankedText(text string) (Model, tea.Cmd) {
	desc := humanSize(int64(len(text)))
	if lines := strings.Count(text, "\n"); lines > 0 {
		desc = fmt.Sprintf("%d lines (%s)", lines, humanSize(int64(len(text))))
	}
	notifyText := desc
	return m, func() tea.Msg {
		if err := ui.WriteClipboard(text); err != nil {
			return previewClipboardMsg{err: err}
		}
		return previewClipboardMsg{desc: notifyText}
	}
}

type previewClipboardMsg struct {
	desc string
	err  error
}

func (m Model) handlePreviewClipboard(msg previewClipboardMsg) (Model, tea.Cmd) {
	if msg.err != nil {
		m.Notify(appshell.LevelError, fmt.Sprintf("Clipboard: %s", msg.err.Error()))
		return m, nil
	}
	m.Notify(appshell.LevelSuccess, "Yanked "+msg.desc)
	return m, nil
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
