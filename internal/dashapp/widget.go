package dashapp

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
	// render with infinite vertical space. Drives scroll clamping.
	RowCount(m *Model) int
	// Render returns the widget body — the inner content drawn
	// inside the pane frame. height is the inner content area
	// (already excluding the pane chrome). offset is the current
	// scroll position.
	Render(m *Model, width, innerHeight int, offset int) string
}

// dashboardWidgets returns the widgets the dashboard renders, in
// stable order. Index in this slice doubles as the focus index and
// the offset slot. Adding a widget here is the only registration
// needed — layout, focus nav, and scroll all pick it up.
func dashboardWidgets() []Widget {
	return []Widget{
		namespaceCountsWidget{},
		dlqAlertsWidget{},
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
