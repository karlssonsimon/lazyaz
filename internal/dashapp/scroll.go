package dashapp

// focusedWidgetDims returns (totalRows, visibleRows) for the focused
// widget. Reads heights stashed by recomputeWidgetHeights so the math
// matches the renderer.
func (m Model) focusedWidgetDims() (total, visible int) {
	if m.focusedIdx < 0 || m.focusedIdx >= len(m.widgets) {
		return 0, 1
	}
	w := m.widgets[m.focusedIdx]
	row, _ := w.Position()
	widgetH := 0
	if row >= 0 && row < len(m.rowHeights) {
		widgetH = m.rowHeights[row]
	}
	visible = innerHeightToVisibleData(widgetH - 2) // pane border is 2 rows
	total = w.RowCount(&m)
	return
}

// innerHeightToVisibleData converts a widget's inner content area
// (height excluding pane border) to the number of data rows that fit:
// inner has 1 row of title + 1 row of header + N data rows.
func innerHeightToVisibleData(innerHeight int) int {
	visible := innerHeight - 2 // 1 title + 1 column header
	if visible < 1 {
		visible = 1
	}
	return visible
}

// scrollFocused shifts the focused widget's offset by delta rows,
// clamped so the last row is reachable. When the list overflows we
// reserve one row for the "N hidden" hint, so usable budget is
// visible-1; max offset reflects that.
func (m *Model) scrollFocused(delta int) {
	if m.focusedIdx < 0 || m.focusedIdx >= len(m.offsets) {
		return
	}
	maxOffset := m.maxFocusedOffset()
	m.offsets[m.focusedIdx] = clampInt(m.offsets[m.focusedIdx]+delta, 0, maxOffset)
}

func (m *Model) scrollFocusedToTop() {
	if m.focusedIdx < 0 || m.focusedIdx >= len(m.offsets) {
		return
	}
	m.offsets[m.focusedIdx] = 0
}

func (m *Model) scrollFocusedToBottom() {
	if m.focusedIdx < 0 || m.focusedIdx >= len(m.offsets) {
		return
	}
	m.offsets[m.focusedIdx] = m.maxFocusedOffset()
}

// maxFocusedOffset is the largest valid offset for the focused widget.
// When the list overflows, the renderer reserves one row for the "N
// hidden" hint, so reaching the last row needs offset = total - (visible-1).
func (m Model) maxFocusedOffset() int {
	total, visible := m.focusedWidgetDims()
	if total <= visible {
		return 0
	}
	return total - (visible - 1)
}

func (m Model) halfPageStep() int {
	_, visible := m.focusedWidgetDims()
	step := visible / 2
	if step < 1 {
		step = 1
	}
	return step
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
