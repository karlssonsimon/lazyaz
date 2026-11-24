package blobapp

import (
	"context"
	"fmt"
	"time"

	"github.com/karlssonsimon/lazyaz/internal/appshell"
	"github.com/karlssonsimon/lazyaz/internal/azure/blob"
	"github.com/karlssonsimon/lazyaz/internal/ui"

	tea "charm.land/bubbletea/v2"
)

// searchScanTimeout bounds one scan command. The budget bounds how much
// it reads; this bounds how long a stalled connection can hold it open.
const searchScanTimeout = 5 * time.Minute

// previewSearchState is the preview's half of the search: the bar owns
// the query, this owns where the last scan stopped.
type previewSearchState struct {
	bar ui.SearchBar
	// resumeAt is where a budget-exhausted scan stopped, so the next n
	// continues instead of paying for the same bytes again. Negative
	// means there is nothing to resume.
	resumeAt int64
	// scanning is true while a scan command is in flight.
	scanning bool
	// requestID guards against a scan landing after the user has moved
	// on, matching how preview window loads are guarded.
	requestID int
	// fromLine is the number of lines before the offset the scan in
	// flight started at, when that was known. The scan counts the
	// newlines it reads on its way, so a hit then arrives with its line
	// number for free.
	fromLine      int
	fromLineKnown bool
}

// searchScanDoneMsg carries a finished scan back to Update.
type searchScanDoneMsg struct {
	requestID int
	blobName  string
	outcome   scanOutcome
	dir       ui.SearchDirection
	match     blobMatch
	resumeAt  int64
	newlines  int
	bytesRead int64
	err       error
}

// serviceRangeReader adapts the blob service to the narrow reader the
// preview needs, capturing the scope so the scan and the windowing stay
// free of account and container plumbing.
type serviceRangeReader struct {
	svc       *blob.Service
	account   blob.Account
	container string
	blobName  string
}

func (r serviceRangeReader) ReadBlobRange(ctx context.Context, offset, count int64) ([]byte, error) {
	return r.svc.ReadBlobRange(ctx, r.account, r.container, r.blobName, offset, count)
}

func (r serviceRangeReader) Properties(ctx context.Context) (blob.BlobProperties, error) {
	return r.svc.GetBlobProperties(ctx, r.account, r.container, r.blobName)
}

// runPreviewSearch starts a scan in the given direction from `from`.
func (m Model) runPreviewSearch(pattern ui.SearchPattern, dir ui.SearchDirection, from int64) (Model, tea.Cmd) {
	if !m.preview.open {
		return m, nil
	}
	// A formatted or decoded document is searched in memory: the scanner
	// reads the real blob, whose bytes are not what is on screen.
	if m.preview.formatted || m.preview.decoded {
		return m.searchFormattedPreview(pattern, dir, from)
	}
	if m.preview.source == nil {
		return m, nil
	}

	m.preview.search.requestID++
	m.preview.search.scanning = true
	m.preview.search.fromLine, m.preview.search.fromLineKnown = m.previewLinesBefore(from)
	m.StartLoading(previewPane, fmt.Sprintf("Searching %s for %s", m.preview.blobName, pattern.Query))

	cmd := searchScanCmd(
		m.preview.source,
		pattern,
		dir,
		from,
		m.preview.blobSize,
		m.searchScanBudget,
		m.preview.blobName,
		m.preview.search.requestID,
	)
	return m, tea.Batch(m.Spinner.Tick, cmd)
}

func searchScanCmd(
	reader rangeReader,
	pattern ui.SearchPattern,
	dir ui.SearchDirection,
	from, blobSize, budget int64,
	blobName string,
	requestID int,
) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), searchScanTimeout)
		defer cancel()

		scanner := blobScanner{reader: reader}

		var (
			res scanResult
			err error
		)
		if dir == ui.SearchBackward {
			res, err = scanner.backward(ctx, pattern, from, budget)
		} else {
			res, err = scanner.forward(ctx, pattern, from, blobSize, budget)
		}

		return searchScanDoneMsg{
			requestID: requestID,
			blobName:  blobName,
			outcome:   res.outcome,
			dir:       dir,
			match:     res.match,
			resumeAt:  res.resumeAt,
			newlines:  res.newlines,
			bytesRead: res.bytesRead,
			err:       err,
		}
	}
}

func (m Model) handleSearchScanDone(msg searchScanDoneMsg) (Model, tea.Cmd) {
	// Stale result: the user searched again, or left this blob.
	if !m.preview.open || msg.requestID != m.preview.search.requestID || msg.blobName != m.preview.blobName {
		return m, nil
	}

	m.preview.search.scanning = false
	m.ClearLoading()

	if msg.err != nil {
		m.ResolveSpinner(m.LoadingSpinnerID, appshell.LevelError, fmt.Sprintf("Search failed: %s", msg.err.Error()))
		return m, nil
	}
	m.DismissSpinner(m.LoadingSpinnerID)

	next := m.Keymap.SearchNext.Short()

	// The scan counted the newlines between where it started and where
	// it stopped, which gives the stopping point its line number.
	stoppedAtLine := m.preview.search.fromLine + msg.newlines
	if msg.dir == ui.SearchBackward {
		stoppedAtLine = m.preview.search.fromLine - msg.newlines
	}

	switch msg.outcome {
	case scanFound:
		m.preview.search.resumeAt = -1
		m.preview.snapToLineStart = false
		m.preview.cursor = msg.match.Start
		if m.preview.search.fromLineKnown {
			m.preview.lineAnchor = previewLineAnchor{offset: msg.match.Start, line: stoppedAtLine, ok: true}
		}
		next, cmd := m.ensurePreviewWindowAtCursor()
		if next.previewHoldsCursor() {
			// The hit is in the loaded window, so the anchor can number
			// it now. Otherwise the window on its way does that when it
			// lands.
			var lineCmd tea.Cmd
			next, lineCmd = next.resolvePreviewLineBase()
			cmd = tea.Batch(cmd, lineCmd)
		}
		return next, cmd

	case scanBudgetSpent:
		// Record where to pick up so the next n does not re-read what
		// this scan already paid for.
		m.preview.search.resumeAt = msg.resumeAt
		if m.preview.search.fromLineKnown {
			m.preview.lineAnchor = previewLineAnchor{offset: msg.resumeAt, line: stoppedAtLine, ok: true}
		}
		m.Notify(appshell.LevelWarn, fmt.Sprintf(
			"Searched %s, no match yet — %s to continue", humanSize(msg.bytesRead), next,
		))
		return m, nil

	default:
		m.preview.search.resumeAt = -1
		edge := "BOTTOM"
		if m.preview.search.bar.Direction == ui.SearchBackward {
			edge = "TOP"
		}
		m.Notify(appshell.LevelInfo, fmt.Sprintf("search hit %s — %s again to wrap", edge, next))
		return m, nil
	}
}

// startPreviewSearch runs the query that was just typed.
func (m Model) startPreviewSearch() (Model, tea.Cmd) {
	pattern := m.preview.search.bar.Pattern
	if !pattern.Valid() {
		return m, nil
	}
	m.preview.search.resumeAt = -1
	dir := m.preview.search.bar.Direction
	return m.runPreviewSearch(pattern, dir, m.searchStartOffset(dir))
}

// repeatPreviewSearch is n / N. A budget-exhausted scan resumes where it
// stopped; anything else continues from the cursor. At an edge the
// resume offset is cleared, so the next press wraps.
func (m Model) repeatPreviewSearch(dir ui.SearchDirection) (Model, tea.Cmd) {
	pattern := m.preview.search.bar.Pattern
	if !pattern.Valid() {
		m.Notify(appshell.LevelInfo, "No previous search")
		return m, nil
	}

	from := m.searchStartOffset(dir)
	if m.preview.search.resumeAt >= 0 && dir == m.preview.search.bar.Direction {
		from = m.preview.search.resumeAt
	} else if m.atSearchEdge(dir) {
		// Wrap: forward restarts at the top, backward at the bottom.
		from = 0
		if dir == ui.SearchBackward {
			from = m.preview.blobSize
		}
	}
	return m.runPreviewSearch(pattern, dir, from)
}

// atSearchEdge reports whether the cursor is already at the end the
// given direction runs towards, which is when n should wrap.
func (m Model) atSearchEdge(dir ui.SearchDirection) bool {
	if dir == ui.SearchBackward {
		return m.preview.cursor <= 0
	}
	return m.preview.cursor >= m.preview.blobSize-1
}

// searchStartOffset is where a search begins so repeating it advances
// instead of re-finding the match the cursor is already on.
func (m Model) searchStartOffset(dir ui.SearchDirection) int64 {
	if dir == ui.SearchBackward {
		return m.preview.cursor
	}
	return m.preview.cursor + 1
}

// previewMatchRanges maps the matches inside the loaded window onto
// viewport rows and display columns, ready for ui.HighlightLines.
//
// Byte offsets cannot serve as columns: the rendered lines carry
// syntax-highlighting escapes and may hold multibyte runes, so a match's
// column is the display width of the plain text before it on its line.
func (m Model) previewMatchRanges() map[int][]ui.ColumnRange {
	pattern := m.preview.search.bar.Pattern
	if !pattern.Valid() || len(m.preview.windowData) == 0 || len(m.preview.lineStarts) == 0 {
		return nil
	}

	// Only the lines on screen are searched. This runs on every frame,
	// and the window is hundreds of screens long.
	top := m.preview.viewport.YOffset()
	height := m.preview.viewport.Height()
	if top >= len(m.preview.lineStarts) || height <= 0 {
		return nil
	}
	visibleStart := m.preview.lineStarts[top]
	visibleEnd := len(m.preview.windowData)
	if bottom := top + height; bottom < len(m.preview.lineStarts) {
		visibleEnd = m.preview.lineStarts[bottom]
	}

	matches := ui.SearchBufferAll(pattern, m.preview.windowData[visibleStart:visibleEnd])
	if len(matches) == 0 {
		return nil
	}

	cursorLocal := int(m.preview.cursor - m.preview.windowStart)
	// Columns are viewport-relative: the horizontal offset shifts them,
	// and ranges scrolled out of view are dropped.
	xoff := m.preview.viewport.XOffset()
	width := m.preview.viewport.Width()

	byRow := make(map[int][]ui.ColumnRange)
	for _, match := range matches {
		matchStart := visibleStart + match.Start
		matchEnd := visibleStart + match.End
		line := m.previewLineOf(matchStart)
		row := line - top
		if row < 0 || row >= height {
			continue
		}

		lineStart := m.preview.lineStarts[line]
		startCol := ui.PlainWidth(m.preview.windowData[lineStart:matchStart]) - xoff
		endCol := startCol + ui.PlainWidth(m.preview.windowData[matchStart:matchEnd])
		if endCol <= 0 || startCol >= width {
			continue
		}
		if startCol < 0 {
			startCol = 0
		}
		if endCol > width {
			endCol = width
		}

		style := m.Styles.SearchMatch
		if matchStart == cursorLocal {
			style = m.Styles.SearchMatchCurrent
		}
		byRow[row] = append(byRow[row], ui.ColumnRange{Start: startCol, End: endCol, Style: style})
	}
	return byRow
}

// previewLineOf returns the index in lineStarts of the line holding the
// given window-relative byte offset.
func (m Model) previewLineOf(offset int) int {
	starts := m.preview.lineStarts
	if len(starts) == 0 {
		return 0
	}
	lo, hi := 0, len(starts)-1
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if starts[mid] <= offset {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	return lo
}

// previewHasFooter reports whether the search bar occupies the preview
// pane's footer row. Layout needs this so the viewport is sized to the
// space that is actually left for it.
func (m Model) previewHasFooter() bool {
	return m.preview.search.bar.InputOpen || m.preview.search.bar.Active()
}
