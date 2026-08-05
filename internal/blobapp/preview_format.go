package blobapp

import (
	"context"
	"fmt"

	"github.com/karlssonsimon/lazyaz/internal/appshell"
	"github.com/karlssonsimon/lazyaz/internal/ui"
	"github.com/karlssonsimon/lazyaz/internal/vim"

	tea "charm.land/bubbletea/v2"
)

// The in-memory format view: = pretty-prints the blob without touching
// it. The trick is that formatted mode makes the window the whole
// document — windowStart 0, blobSize len(formatted) — so the window
// machinery quiesces by construction: slides never trigger, yank never
// fetches, and the cursor, visual mode and text objects operate on the
// formatted text unchanged. Only search needs an explicit branch,
// because the streamed scanner reads the real blob.

// previewRawStash is the raw view saved across a format toggle so = again
// restores it exactly, position included.
type previewRawStash struct {
	windowStart int64
	windowData  []byte
	lineStarts  []int
	rendered    string
	cursor      int64
	vcur        vim.Cursor
	blobSize    int64
	yoffset     int
	xoffset     int
}

// previewFormatFetchedMsg carries the whole blob fetched for formatting.
type previewFormatFetchedMsg struct {
	requestID int
	blobName  string
	data      []byte
	err       error
}

// togglePreviewFormat is =: format in memory, or restore the raw view
// when already formatted. Formatting needs the entire document — a
// window mid-JSON is not parseable — so blobs beyond the loaded window
// are fetched first, bounded by the format budget.
func (m Model) togglePreviewFormat() (Model, tea.Cmd) {
	if !m.preview.open {
		return m, nil
	}
	if m.preview.formatted {
		return m.restorePreviewRaw()
	}
	if m.preview.binary {
		m.Notify(appshell.LevelInfo, "Binary content cannot be formatted")
		return m, nil
	}
	if m.preview.blobSize > m.formatBudget {
		m.Notify(appshell.LevelWarn, fmt.Sprintf(
			"Blob is %s — over the format limit (%s)", humanSize(m.preview.blobSize), humanSize(m.formatBudget),
		))
		return m, nil
	}

	if m.preview.windowStart == 0 && int64(len(m.preview.windowData)) >= m.preview.blobSize {
		return m.applyPreviewFormat(m.preview.windowData)
	}

	if m.service == nil {
		m.Notify(appshell.LevelError, "Formatting needs the whole blob and no service is available")
		return m, nil
	}
	m.preview.requestID++
	m.StartLoading(previewPane, fmt.Sprintf("Fetching %s to format", m.preview.blobName))
	reader := serviceRangeReader{
		svc:       m.service,
		account:   m.currentAccount,
		container: m.containerName,
		blobName:  m.preview.blobName,
	}
	return m, tea.Batch(m.Spinner.Tick, formatFetchCmd(reader, m.preview.blobName, m.preview.blobSize, m.preview.requestID))
}

func formatFetchCmd(reader rangeReader, blobName string, size int64, requestID int) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), searchScanTimeout)
		defer cancel()
		data, err := reader.ReadBlobRange(ctx, 0, size)
		return previewFormatFetchedMsg{
			requestID: requestID,
			blobName:  blobName,
			data:      data,
			err:       err,
		}
	}
}

func (m Model) handlePreviewFormatFetched(msg previewFormatFetchedMsg) (Model, tea.Cmd) {
	if !m.preview.open || msg.blobName != m.preview.blobName || msg.requestID != m.preview.requestID {
		return m, nil
	}
	m.ClearLoading()
	if msg.err != nil {
		m.ResolveSpinner(m.LoadingSpinnerID, appshell.LevelError, fmt.Sprintf("Fetch for formatting failed: %s", msg.err.Error()))
		return m, nil
	}
	m.DismissSpinner(m.LoadingSpinnerID)
	return m.applyPreviewFormat(msg.data)
}

// applyPreviewFormat formats raw (the entire document) and swaps it in
// as the window. Cursor, search and selection reset — there is no
// honest mapping of a raw position into pretty-printed text — while the
// stash keeps the raw view for an exact restore.
func (m Model) applyPreviewFormat(raw []byte) (Model, tea.Cmd) {
	formatted, kind, ok := ui.FormatDocument(raw)
	if !ok {
		m.Notify(appshell.LevelInfo, "Content is neither valid JSON nor XML")
		return m, nil
	}

	m.preview.rawStash = &previewRawStash{
		windowStart: m.preview.windowStart,
		windowData:  m.preview.windowData,
		lineStarts:  m.preview.lineStarts,
		rendered:    m.preview.rendered,
		cursor:      m.preview.cursor,
		vcur:        m.preview.vcur,
		blobSize:    m.preview.blobSize,
		yoffset:     m.preview.viewport.YOffset(),
		xoffset:     m.preview.viewport.XOffset(),
	}

	m.preview.windowStart = 0
	m.preview.windowData = formatted
	m.preview.lineStarts = computeLineStarts(formatted)
	m.preview.blobSize = int64(len(formatted))
	m.preview.cursor = 0
	m.preview.vcur = vim.Cursor{}
	m.preview.span.Stop()
	m.preview.snapToLineStart = false
	m.clearPreviewSearch()
	m.preview.formatted = true
	m.preview.formatKind = kind
	m.preview.rendered = renderPreviewContent(formatted, m.preview.blobName, m.preview.contentType, false, m.Styles)
	m.preview.viewport.SetContent(m.preview.rendered)
	m.preview.viewport.SetYOffset(0)
	m.preview.viewport.SetXOffset(0)

	m.Notify(appshell.LevelSuccess, fmt.Sprintf("Formatted as %s (in-memory) — = to restore", kind))
	return m, nil
}

// restorePreviewRaw is = in formatted mode: put the stashed raw view
// back exactly as it was, position included.
func (m Model) restorePreviewRaw() (Model, tea.Cmd) {
	st := m.preview.rawStash
	m.preview.formatted = false
	m.preview.formatKind = ""
	m.preview.rawStash = nil
	if st == nil {
		return m, nil
	}

	m.preview.windowStart = st.windowStart
	m.preview.windowData = st.windowData
	m.preview.lineStarts = st.lineStarts
	m.preview.rendered = st.rendered
	m.preview.cursor = st.cursor
	m.preview.vcur = st.vcur
	m.preview.blobSize = st.blobSize
	m.preview.span.Stop()
	m.preview.snapToLineStart = false
	m.clearPreviewSearch()
	m.preview.viewport.SetContent(st.rendered)
	m.preview.viewport.SetYOffset(st.yoffset)
	m.preview.viewport.SetXOffset(st.xoffset)

	m.Notify(appshell.LevelInfo, "Raw view restored")
	return m, nil
}

// clearPreviewSearch drops the search across a format toggle: match
// offsets from one view are meaningless in the other, and a scan still
// in flight must not land — the requestID bump orphans it.
func (m *Model) clearPreviewSearch() {
	m.preview.search.bar.Clear()
	m.preview.search.resumeAt = -1
	m.preview.search.scanning = false
	m.preview.search.requestID++
}

// searchFormattedPreview replaces the streamed scan while formatted:
// the scanner reads the real blob, whose bytes no longer match what is
// on screen, and the whole formatted document is in memory anyway. Like
// the message body — and unlike the streamed path, whose deferred wrap
// exists because wrapping re-reads billed bytes — this wraps
// immediately.
func (m Model) searchFormattedPreview(pattern ui.SearchPattern, dir ui.SearchDirection, from int64) (Model, tea.Cmd) {
	body := m.preview.windowData
	at := int(clampInt64(from, 0, int64(len(body))))

	var match *ui.SearchMatch
	if dir == ui.SearchBackward {
		match = ui.SearchBufferBackward(pattern, body, at)
		if match == nil {
			match = ui.SearchBufferBackward(pattern, body, len(body))
			if match != nil {
				m.Notify(appshell.LevelInfo, "search hit TOP, continuing at BOTTOM")
			}
		}
	} else {
		match = ui.SearchBufferForward(pattern, body, at)
		if match == nil {
			match = ui.SearchBufferForward(pattern, body, 0)
			if match != nil {
				m.Notify(appshell.LevelInfo, "search hit BOTTOM, continuing at TOP")
			}
		}
	}

	if match == nil {
		m.Notify(appshell.LevelWarn, "Pattern not found: "+pattern.Query)
		return m, nil
	}

	m.preview.cursor = int64(match.Start)
	return m.ensurePreviewWindowAtCursor()
}
