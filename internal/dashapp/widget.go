package dashapp

import (
	"strings"

	"github.com/karlssonsimon/lazyaz/internal/ui"

	tea "charm.land/bubbletea/v2"
)

// widgetViewState is the per-widget UI state the dashboard tracks
// alongside cursor + offset. Lives on the Model in a slice parallel
// to widgets. Resets when the tab closes; not persisted to config.
type widgetViewState struct {
	sortField int    // index into Widget.SortFields(); 0 = widget default
	sortDesc  bool   // sort direction; widget supplies the natural default per field
	hasSort   bool   // false until the user picks one (avoids "fake default" surprise)
	filter    string // ephemeral substring filter; reserved for future filter feature
}

// SortField describes one column a widget supports sorting by. The
// sort overlay generates two entries per field — one ascending, one
// descending — so direction lives in the option, not the field.
type SortField struct {
	Label string
}

// Widget is the unit of dashboard composition. Each widget owns its
// title and body rendering; the dashboard owns the surrounding pane
// frame, focus state, and scroll offset. Widgets are stateless value
// types — anything that varies per-instance (scroll, filter, etc.)
// lives on the dashboard Model in slices keyed by widget index.
type Widget interface {
	// Title is shown in the widget's pane header.
	Title() string
	// Context is an optional one-line summary appended after the title
	// (e.g. "11 visible · 312 total" or "86 · 5,394 msgs"). Empty when
	// the widget has nothing meaningful to summarise. Rendered muted.
	Context(m *Model, view widgetViewState) string
	// Position returns the (row, col) grid cell the widget occupies.
	// Position drives both layout and spatial navigation. (0, 0) is
	// the top-left cell.
	Position() (row, col int)
	// RowCount is the total number of data rows the widget would
	// render with infinite vertical space, after applying the active
	// filter (if any). Drives scroll clamping. Must agree with the
	// number of rows Render actually emits.
	RowCount(m *Model, view widgetViewState) int
	// Render returns the widget body drawn between the Miller column
	// title and footer. offset is the current scroll position; cursor is
	// the data row index the user has selected; view carries the active
	// sort/filter for this widget.
	Render(m *Model, width, innerHeight, offset, cursor int, view widgetViewState) string
	// Actions returns the actions the widget exposes for the data
	// row at cursorRow. Each action has its own keybinding; pressing
	// the key while the widget is focused fires the action's Cmd.
	Actions(m *Model, cursorRow int) []Action
	// SortFields lists the columns this widget supports sorting by.
	// Empty = widget doesn't support sorting. Index in this slice is
	// what the sort overlay's options reference.
	SortFields() []SortField
}

// Action is a single keybound thing a widget can do for a data row.
// Cmd is what runs when the user presses Key — typically it returns a
// tea.Msg that the parent app routes (e.g. opening a new tab).
type Action struct {
	// Label is shown in help / future action menus.
	Label string
	// Key is the literal keypress that triggers the action (e.g. "o").
	// Single keys only — no chords for now.
	Key string
	// Cmd produces the tea.Msg that drives the action's effect.
	Cmd tea.Cmd
}

// openSortOverlayMsg asks the dashboard to open the sort picker for the
// focused widget. Emitted by every sortable widget's "Sort by..." action.
type openSortOverlayMsg struct{}

func openSortOverlayCmd() tea.Cmd {
	return func() tea.Msg { return openSortOverlayMsg{} }
}

// sortAction is the single "Sort by..." entry sortable widgets include
// in their action list. Replaces the old per-field numeric shortcuts —
// the dedicated overlay scales better as widgets gain more fields and
// keeps the action menu scannable.
func sortAction() Action {
	return Action{
		Label: "Sort by...",
		Key:   "s",
		Cmd:   openSortOverlayCmd(),
	}
}

func focusedWidgetView(m *Model) widgetViewState {
	if m == nil || m.focusedIdx < 0 || m.focusedIdx >= len(m.viewStates) {
		return widgetViewState{}
	}
	return m.viewStates[m.focusedIdx]
}

// matchesFilter is the case-insensitive substring check shared by all
// widgets' filter logic. Empty filter matches everything.
func matchesFilter(haystack, filter string) bool {
	if filter == "" {
		return true
	}
	return strings.Contains(strings.ToLower(haystack), strings.ToLower(filter))
}

// severityScale picks the warn/danger thresholds for a given numeric
// column. Counts (queues/topics) need much lower bands than message
// totals (active/dlq) because their natural magnitudes differ.
type severityScale struct{ warn, danger int64 }

var (
	// severityCounts is for entity counts (queues, topics): a few is
	// fine, dozens deserve a glance, hundreds are unusual.
	severityCounts = severityScale{warn: 50, danger: 200}
	// severityMessages is for live message backlogs: thousands warn,
	// >5k is the "go look at this" tier.
	severityMessages = severityScale{warn: 1000, danger: 5000}
	// severityDLQ is stricter — any DLQ build-up > 100 is concerning,
	// > 1000 is bad.
	severityDLQ = severityScale{warn: 100, danger: 1000}
)

// countCell formats n for a numeric table column: thousand-separated,
// muted at zero, amber at the warn threshold, pink at the danger
// threshold. Returns a styled string ready to drop into renderTable.
func countCell(n int64, styles ui.Styles, scale severityScale) string {
	text := formatThousands(n)
	switch {
	case n == 0:
		return styles.Muted.Render(text)
	case n >= scale.danger:
		return styles.DangerBold.Render(text)
	case n >= scale.warn:
		return styles.Warning.Render(text)
	}
	return text
}

// usageCountCell formats a "uses" counter scaled relative to the row
// max — the top entry pops pink, the rest fade through default → muted.
// Used by the most-used widgets where there's no absolute "danger" but
// users want to see what they touch most.
func usageCountCell(n, max int64, styles ui.Styles) string {
	text := formatThousands(n)
	if max <= 0 {
		return text
	}
	switch {
	case n == max:
		return styles.DangerBold.Render(text)
	case n*4 >= max*3: // top quartile
		return styles.Warning.Render(text)
	case n*4 < max:
		return styles.Muted.Render(text)
	}
	return text
}

// formatThousands formats n with non-breaking-space thousand separators
// (e.g. 6051 → "6 051"), matching the Swedish-style numerics in the
// dashboard mockups. Negative numbers preserve their sign.
func formatThousands(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [32]byte
	pos := len(buf)
	digits := 0
	for n > 0 {
		if digits == 3 {
			pos--
			buf[pos] = ' '
			digits = 0
		}
		pos--
		buf[pos] = byte('0' + n%10)
		n /= 10
		digits++
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}

// dashboardWidgets returns the widgets the dashboard renders, in
// stable order. Index in this slice doubles as the focus index and
// the offset slot. Adding a widget here is the only registration
// needed — layout, focus nav, and scroll all pick it up.
func dashboardWidgets() []Widget {
	return []Widget{
		namespaceCountsWidget{},
		dlqAlertsWidget{},
		usedSBWidget{},
		usedBlobWidget{},
	}
}

// computeRowHeights distributes bodyHeight across numRows cells.
// Remainder rows are given to the bottom cells so the layout matches
// the previous "topH = body/2, botH = body - topH" math.
func computeRowHeights(bodyHeight, numRows int) []int {
	if numRows <= 0 {
		return nil
	}
	base := bodyHeight / numRows
	rem := bodyHeight - base*numRows
	heights := make([]int, numRows)
	for i := range heights {
		heights[i] = base
		if i >= numRows-rem {
			heights[i]++
		}
	}
	return heights
}

// computeColWidths splits totalWidth across numCols cells. Remainder
// columns go to the right-most cells (parallel to computeRowHeights).
// Returns nil for numCols <= 0.
func computeColWidths(totalWidth, numCols int) []int {
	if numCols <= 0 {
		return nil
	}
	base := totalWidth / numCols
	rem := totalWidth - base*numCols
	widths := make([]int, numCols)
	for i := range widths {
		widths[i] = base
		if i >= numCols-rem {
			widths[i]++
		}
	}
	return widths
}

// gridDims returns the number of rows and columns occupied by widgets.
func gridDims(widgets []Widget) (rows, cols int) {
	for _, w := range widgets {
		r, c := w.Position()
		if r+1 > rows {
			rows = r + 1
		}
		if c+1 > cols {
			cols = c + 1
		}
	}
	return
}

// findWidgetIdx returns the index of the widget at (row, col), or -1.
func findWidgetIdx(widgets []Widget, row, col int) int {
	for i, w := range widgets {
		r, c := w.Position()
		if r == row && c == col {
			return i
		}
	}
	return -1
}
