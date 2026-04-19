package dashapp

import (
	"fmt"
	"strings"

	"github.com/karlssonsimon/lazyaz/internal/ui"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func (m Model) View() tea.View {
	if m.Width == 0 || m.Height == 0 {
		v := tea.NewView("loading...")
		v.AltScreen = true
		return v
	}

	subBar := ui.RenderSubscriptionBar(m.CurrentSub, m.HasSubscription, m.Styles, m.Width)
	statusBar := ui.RenderStatusBar(m.Styles, m.statusBarItems(), "", false, m.Width)

	bodyHeight := m.Height - lipgloss.Height(subBar) - lipgloss.Height(statusBar)
	if bodyHeight < 2 {
		bodyHeight = 2
	}

	rows, _ := gridDims(m.widgets)
	heights := computeRowHeights(bodyHeight, rows)

	rowSlots := make([]string, rows)
	for i, w := range m.widgets {
		row, _ := w.Position()
		if row >= rows {
			continue
		}
		rowSlots[row] = m.renderWidget(w, i, m.Width, heights[row])
	}

	body := lipgloss.JoinVertical(lipgloss.Left, rowSlots...)
	view := ui.RenderCanvas(lipgloss.JoinVertical(lipgloss.Left, subBar, body, statusBar), m.Width, m.Height, m.Styles.Bg)

	if m.sortOverlay.active {
		view = m.renderSortOverlay(view)
	}
	if m.actionMenu.active {
		view = m.renderActionMenu(view)
	}
	out := tea.NewView(m.RenderOverlays(view))
	out.AltScreen = true
	out.MouseMode = tea.MouseModeCellMotion
	return out
}

func (m Model) renderWidget(w Widget, idx, width, height int) string {
	focused := idx == m.focusedIdx
	innerHeight := height - 2 // pane border
	cursor := -1
	// Only show the highlight on the focused widget so it's clear
	// where keys land. Background cursor rows would compete visually.
	if focused {
		cursor = m.cursors[idx]
	}
	body := w.Render(&m, width, innerHeight, m.offsets[idx], cursor, m.viewStates[idx])
	return m.widgetFrame(m.widgetTitleFor(w, idx, focused), body, width, height, focused)
}

// widgetTitleFor returns the title shown in the pane header. While the
// filter input is open on the focused widget, the title carries the
// filter prompt so the user sees what they're typing without a
// separate input box.
func (m Model) widgetTitleFor(w Widget, idx int, focused bool) string {
	title := w.Title()
	view := m.viewStates[idx]
	if focused && m.filterInputActive {
		return title + "  /" + view.filter + "▎"
	}
	if view.filter != "" {
		return title + "  filter: " + view.filter
	}
	return title
}

func (m Model) statusBarItems() []ui.StatusBarItem {
	var items []ui.StatusBarItem
	items = append(items, ui.StatusBarItem{Label: "Namespaces:", Value: fmt.Sprintf("%d", len(m.namespaces))})
	if m.pendingFetches > 0 {
		items = append(items, ui.StatusBarItem{Label: "Loading:", Value: fmt.Sprintf("%d", m.pendingFetches)})
	}
	km := m.Keymap
	items = append(items,
		ui.StatusBarItem{Label: km.RefreshScope.Short(), Value: "refresh"},
		ui.StatusBarItem{Label: km.SubscriptionPicker.Short(), Value: "subscription"},
	)
	return items
}

// widgetFrame wraps content in a titled, bordered box sized to (width, height).
// While a refresh is in flight, the title carries an inline spinner glyph
// so the silent refresh is observable.
func (m Model) widgetFrame(title, body string, width, height int, focused bool) string {
	base := m.Styles.Chrome.Pane
	if focused {
		base = m.Styles.Chrome.FocusedPane
	}
	pane := base.
		Width(width - 2).
		Height(height - 2)
	titleLine := m.Styles.Chrome.Header.Render(title)
	if m.refreshInFlight > 0 {
		spin := m.Styles.Chrome.Meta.Render(" " + m.Spinner.View())
		titleLine = lipgloss.JoinHorizontal(lipgloss.Top, titleLine, spin)
	}
	return pane.Render(lipgloss.JoinVertical(lipgloss.Left, titleLine, body))
}

// loadingOrEmpty picks an empty-state hint. Avoids showing "no
// results" when an in-flight fetch could still bring data in.
func (m Model) loadingOrEmpty(empty string) string {
	if m.pendingFetches > 0 || m.Loading || m.refreshInFlight > 0 {
		return "Loading..."
	}
	return empty
}

// renderScrollableTable renders a table sliced by offset. cells[0] is
// always the header (rendered in place); cells[1:] are data rows — a
// window of up to maxDataRows starting at offset is shown. A "N more"
// hint is appended when rows are hidden above or below. cursorRow is
// the data row index to highlight (negative = no highlight).
func renderScrollableTable(cells [][]string, aligns []lipgloss.Position, styles ui.Styles, offset, maxDataRows, cursorRow int) string {
	if len(cells) == 0 {
		return ""
	}
	header := cells[0]
	data := cells[1:]

	if offset > len(data) {
		offset = len(data)
	}
	if offset < 0 {
		offset = 0
	}

	hidden := len(data) - (offset + maxDataRows)
	reserveHint := hidden > 0 || offset > 0
	rowBudget := maxDataRows
	if reserveHint {
		rowBudget = maxDataRows - 1
		if rowBudget < 1 {
			rowBudget = 1
		}
	}

	end := offset + rowBudget
	if end > len(data) {
		end = len(data)
	}
	window := [][]string{header}
	window = append(window, data[offset:end]...)
	// Cursor index inside the rendered window: header is row 0, data
	// starts at row 1. Pass -1 if cursor is outside the window.
	highlightInWindow := -1
	if cursorRow >= offset && cursorRow < end {
		highlightInWindow = (cursorRow - offset) + 1
	}
	table := renderTable(window, aligns, styles, highlightInWindow)

	if !reserveHint {
		return table
	}
	hiddenBelow := len(data) - end
	var hint string
	switch {
	case offset > 0 && hiddenBelow > 0:
		hint = fmt.Sprintf("↑ %d hidden above · %d below", offset, hiddenBelow)
	case offset > 0:
		hint = fmt.Sprintf("↑ %d hidden above", offset)
	case hiddenBelow > 0:
		hint = fmt.Sprintf("↓ %d more below", hiddenBelow)
	}
	return table + "\n" + styles.Chrome.Meta.Render(hint)
}

// renderTable lays out a simple padded table with per-column alignment.
// First row is the header (Meta style). Row at index highlightRow gets
// the SelectionHighlight style applied — that's the visual cursor.
// Pass -1 to skip the highlight.
func renderTable(cells [][]string, aligns []lipgloss.Position, styles ui.Styles, highlightRow int) string {
	if len(cells) == 0 {
		return ""
	}
	cols := len(cells[0])
	widths := make([]int, cols)
	for _, row := range cells {
		for i, c := range row {
			if i >= cols {
				break
			}
			if w := lipgloss.Width(c); w > widths[i] {
				widths[i] = w
			}
		}
	}

	var out strings.Builder
	for ri, row := range cells {
		parts := make([]string, 0, cols)
		for i, c := range row {
			if i >= cols {
				break
			}
			align := lipgloss.Left
			if i < len(aligns) {
				align = aligns[i]
			}
			cell := lipgloss.NewStyle().Width(widths[i]).Align(align).Render(c)
			parts = append(parts, cell)
		}
		line := strings.Join(parts, "  ")
		switch {
		case ri == 0:
			line = styles.Chrome.Meta.Render(line)
		case ri == highlightRow:
			line = styles.SelectionHighlight.Render(line)
		}
		out.WriteString(line)
		if ri < len(cells)-1 {
			out.WriteString("\n")
		}
	}
	return out.String()
}
