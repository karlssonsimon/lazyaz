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
	visible = innerHeightToVisibleData(widgetH - 2) // Miller title + footer are 2 rows
	total = w.RowCount(&m, m.viewStates[m.focusedIdx])
	return
}

// innerHeightToVisibleData converts a Miller column body height to the
// number of data rows the table renderer can budget. The widget title
// and footer are outside this body; only the table header is inside it.
func innerHeightToVisibleData(innerHeight int) int {
	visible := innerHeight - 1 // 1 column header
	if visible < 1 {
		visible = 1
	}
	return visible
}

// moveCursorFocused shifts the focused widget's cursor by delta rows,
// clamped to [0, total-1], then nudges the scroll offset so the cursor
// stays visible. j/k/ctrl+d/ctrl+u all funnel through here.
func (m *Model) moveCursorFocused(delta int) {
	if m.focusedIdx < 0 || m.focusedIdx >= len(m.cursors) {
		return
	}
	total, _ := m.focusedWidgetDims()
	if total <= 0 {
		m.cursors[m.focusedIdx] = 0
		m.offsets[m.focusedIdx] = 0
		return
	}
	m.cursors[m.focusedIdx] = clampInt(m.cursors[m.focusedIdx]+delta, 0, total-1)
	m.scrollToKeepCursorVisible()
}

func (m *Model) cursorToTop() {
	if m.focusedIdx < 0 || m.focusedIdx >= len(m.cursors) {
		return
	}
	m.cursors[m.focusedIdx] = 0
	m.scrollToKeepCursorVisible()
}

func (m *Model) cursorToBottom() {
	if m.focusedIdx < 0 || m.focusedIdx >= len(m.cursors) {
		return
	}
	total, _ := m.focusedWidgetDims()
	if total <= 0 {
		return
	}
	m.cursors[m.focusedIdx] = total - 1
	m.scrollToKeepCursorVisible()
}

// scrollToKeepCursorVisible nudges the focused widget's offset so the
// cursor row stays inside the visible window. Vim-style: cursor at the
// top edge → offset shrinks; cursor at the bottom edge → offset grows.
// Accounts for the "N hidden" hint row that the renderer reserves
// when the list overflows.
func (m *Model) scrollToKeepCursorVisible() {
	total, visible := m.focusedWidgetDims()
	if total <= visible {
		m.offsets[m.focusedIdx] = 0
		return
	}
	// When the list overflows we lose one row to the hint, so the
	// cursor must fit in [offset, offset + visible-1).
	visibleData := visible - 1
	if visibleData < 1 {
		visibleData = 1
	}
	cursor := m.cursors[m.focusedIdx]
	offset := m.offsets[m.focusedIdx]
	if cursor < offset {
		offset = cursor
	} else if cursor >= offset+visibleData {
		offset = cursor - visibleData + 1
	}
	maxOffset := total - visibleData
	if maxOffset < 0 {
		maxOffset = 0
	}
	m.offsets[m.focusedIdx] = clampInt(offset, 0, maxOffset)
}

// clampCursorsToData runs after data updates to keep cursors valid
// when rows disappear (e.g. a refresh removes a namespace). Without
// this, the cursor could point past the end and actions would target
// the wrong row.
func (m *Model) clampCursorsToData() {
	for i, w := range m.widgets {
		total := w.RowCount(m, m.viewStates[i])
		if total <= 0 {
			m.cursors[i] = 0
			m.offsets[i] = 0
			continue
		}
		if m.cursors[i] >= total {
			m.cursors[i] = total - 1
		}
		if m.cursors[i] < 0 {
			m.cursors[i] = 0
		}
	}
	// Re-run scroll-follows-cursor for the focused widget so the new
	// cursor position is in view after data updates.
	m.scrollToKeepCursorVisible()
}

func (m Model) halfPageStep() int {
	_, visible := m.focusedWidgetDims()
	step := visible / 2
	if step < 1 {
		step = 1
	}
	return step
}

// focusedCursor returns the cursor row of the focused widget, or 0 if
// indices are out of range. Convenience for action handlers.
func (m Model) focusedCursor() int {
	if m.focusedIdx < 0 || m.focusedIdx >= len(m.cursors) {
		return 0
	}
	return m.cursors[m.focusedIdx]
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
