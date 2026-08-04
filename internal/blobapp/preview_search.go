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
}

// searchScanDoneMsg carries a finished scan back to Update.
type searchScanDoneMsg struct {
	requestID int
	blobName  string
	outcome   scanOutcome
	match     blobMatch
	resumeAt  int64
	bytesRead int64
	err       error
}

// serviceRangeReader adapts the blob service to the narrow rangeReader
// the scanner needs, capturing the scope so the scan itself stays free
// of account and container plumbing.
type serviceRangeReader struct {
	svc       *blob.Service
	account   blob.Account
	container string
	blobName  string
}

func (r serviceRangeReader) ReadBlobRange(ctx context.Context, offset, count int64) ([]byte, error) {
	return r.svc.ReadBlobRange(ctx, r.account, r.container, r.blobName, offset, count)
}

// runPreviewSearch starts a scan in the given direction from `from`.
func (m Model) runPreviewSearch(pattern ui.SearchPattern, dir ui.SearchDirection, from int64) (Model, tea.Cmd) {
	if m.service == nil || !m.preview.open {
		return m, nil
	}

	m.preview.search.requestID++
	m.preview.search.scanning = true
	m.StartLoading(previewPane, fmt.Sprintf("Searching %s for %s", m.preview.blobName, pattern.Query))

	reader := serviceRangeReader{
		svc:       m.service,
		account:   m.currentAccount,
		container: m.containerName,
		blobName:  m.preview.blobName,
	}

	cmd := searchScanCmd(
		reader,
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
			match:     res.match,
			resumeAt:  res.resumeAt,
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

	switch msg.outcome {
	case scanFound:
		m.preview.search.resumeAt = -1
		m.preview.cursor = msg.match.Start
		return m.ensurePreviewWindowAtCursor()

	case scanBudgetSpent:
		// Record where to pick up so the next n does not re-read what
		// this scan already paid for.
		m.preview.search.resumeAt = msg.resumeAt
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

	matches := ui.SearchBufferAll(pattern, m.preview.windowData)
	if len(matches) == 0 {
		return nil
	}

	top := m.preview.viewport.YOffset()
	height := m.preview.viewport.Height()
	cursorLocal := int(m.preview.cursor - m.preview.windowStart)

	byRow := make(map[int][]ui.ColumnRange)
	for _, match := range matches {
		line := m.previewLineOf(match.Start)
		row := line - top
		if row < 0 || row >= height {
			continue
		}

		lineStart := m.preview.lineStarts[line]
		if lineStart > match.Start {
			continue
		}
		// Columns are viewport-relative: the horizontal offset shifts
		// them, and ranges scrolled out of view are dropped.
		xoff := m.preview.viewport.XOffset()
		width := m.preview.viewport.Width()
		startCol := ui.PlainWidth(m.preview.windowData[lineStart:match.Start]) - xoff
		endCol := startCol + ui.PlainWidth(m.preview.windowData[match.Start:match.End])
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
		if match.Start == cursorLocal {
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
