package dashapp

import (
	"strings"

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
