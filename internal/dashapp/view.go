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

	bodyHeight := ui.AppBodyHeight(m.Height)
	if bodyHeight < 2 {
		bodyHeight = 2
	}

	// Group widgets by row so we can lay out multiple columns within
	// a single row. Per-row column count is taken from the highest
	// col index of widgets in that row, so a sparse grid (e.g. 1
	// widget in row 0, 2 in row 1) still tiles cleanly.
	rows, _ := gridDims(m.widgets)
	// Reserve a row between every pair of widget rows for the
	// horizontal separator rule.
	rowGutter := 0
	if rows > 1 {
		rowGutter = rows - 1
	}
	widgetHeights := bodyHeight - rowGutter
	if widgetHeights < rows {
		widgetHeights = rows // 1 row per widget minimum
	}
	heights := computeRowHeights(widgetHeights, rows)

	rowWidgets := make([][]int, rows)
	for i, w := range m.widgets {
		row, _ := w.Position()
		if row < 0 || row >= rows {
			continue
		}
		rowWidgets[row] = append(rowWidgets[row], i)
	}

	rowSlots := make([]string, rows)
	for r, idxs := range rowWidgets {
		if len(idxs) == 0 {
			continue
		}
		// Determine column count for this row from its widgets'
		// positions.
		maxCol := 0
		for _, i := range idxs {
			_, c := m.widgets[i].Position()
			if c > maxCol {
				maxCol = c
			}
		}
		cols := maxCol + 1
		colWidths := computeColWidths(m.Width, cols)
		colSlots := make([]string, cols)
		for _, i := range idxs {
			_, c := m.widgets[i].Position()
			if c < 0 || c >= cols {
				continue
			}
			// Only non-last widgets get a right rule — the rightmost
			// widget's right edge is the screen edge; a `│` there
			// would dangle against the bg.
			isLast := c == cols-1
			colSlots[c] = m.renderWidget(m.widgets[i], i, colWidths[c], heights[r], !isLast)
		}
		rowSlots[r] = lipgloss.JoinHorizontal(lipgloss.Top, colSlots...)
	}

	// Stitch widget rows together with a full-width horizontal rule
	// between them so stacked rows don't visually merge.
	bodyParts := make([]string, 0, 2*rows)
	for r, slot := range rowSlots {
		if slot == "" {
			continue
		}
		if len(bodyParts) > 0 {
			bodyParts = append(bodyParts, ui.RenderHorizontalRule(m.Width, m.Styles, nil))
		}
		bodyParts = append(bodyParts, slot)
		_ = r
	}
	body := lipgloss.JoinVertical(lipgloss.Left, bodyParts...)
	header := ui.RenderAppHeader(ui.HeaderConfig{
		Brand: "lazyaz",
		Path:  m.headerPath(),
		Meta:  ui.HeaderMeta(m.CurrentSub, m.HasSubscription, m.Styles),
	}, m.Styles, m.Width)
	statusBar := ui.RenderStatusLine(ui.StatusLineConfig{
		Mode:    "NORMAL",
		Actions: m.statusActions(),
	}, m.Styles, m.Width)
	// Dashboard's grid has multiple widgets per row with internal rules,
	// so computing per-row tick positions correctly is more involved.
	// Skip ticks for now — the rule still spans full width.
	topRule := ui.RenderHorizontalRule(m.Width, m.Styles, nil)
	bottomRule := ui.RenderHorizontalRuleBottom(m.Width, m.Styles, nil)
	view := ui.RenderCanvas(lipgloss.JoinVertical(lipgloss.Left, header, topRule, body, bottomRule, statusBar), m.Width, m.Height, m.Styles.Bg)

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

func (m Model) renderWidget(w Widget, idx, width, height int, rightRule bool) string {
	focused := idx == m.focusedIdx
	innerHeight := height - 2 // title and footer
	cursor := -1
	// Only show the highlight on the focused widget so it's clear
	// where keys land. Background cursor rows would compete visually.
	if focused {
		cursor = m.cursors[idx]
	}
	body := w.Render(&m, width, innerHeight, m.offsets[idx], cursor, m.viewStates[idx])
	return m.widgetFrame(m.widgetTitleFor(w, idx, focused), body, width, height, focused, rightRule)
}

func (m Model) headerPath() []string {
	// Dashboard's only "scope" is the active subscription — show it as
	// the single breadcrumb when set, otherwise nothing (the meta slot
	// already announces "no subscription").
	if m.HasSubscription {
		return []string{ui.SubscriptionDisplayName(m.CurrentSub)}
	}
	return nil
}

// compactDirections renders four h/j/k/l-style keys as a single
// compact hint. When all four share a modifier prefix (e.g.
// "ctrl+h", "ctrl+j", "ctrl+k", "ctrl+l") it collapses to
// `ctrl+hjkl`. Falls back to slash-separated `a/b/c/d` if the
// shapes don't match (custom keymaps, mixed modifiers).
func compactDirections(left, down, up, right string) string {
	parts := [4]string{left, down, up, right}
	prefix := commonModifierPrefix(parts[:])
	tails := [4]string{
		strings.TrimPrefix(left, prefix),
		strings.TrimPrefix(down, prefix),
		strings.TrimPrefix(up, prefix),
		strings.TrimPrefix(right, prefix),
	}
	allSingle := true
	for _, t := range tails {
		if len([]rune(t)) != 1 {
			allSingle = false
			break
		}
	}
	if prefix != "" && allSingle {
		return prefix + tails[0] + tails[1] + tails[2] + tails[3]
	}
	return left + "/" + down + "/" + up + "/" + right
}

// commonModifierPrefix returns the shared "<mod>+" prefix across the
// keys, e.g. "ctrl+" for ["ctrl+h","ctrl+j","ctrl+k","ctrl+l"].
// Returns "" if the keys don't all share the same single modifier.
func commonModifierPrefix(keys []string) string {
	if len(keys) == 0 {
		return ""
	}
	idx := strings.LastIndex(keys[0], "+")
	if idx < 0 {
		return ""
	}
	prefix := keys[0][:idx+1]
	for _, k := range keys[1:] {
		if !strings.HasPrefix(k, prefix) {
			return ""
		}
	}
	return prefix
}

func (m Model) statusActions() []ui.StatusAction {
	km := m.Keymap
	actions := []ui.StatusAction{
		{Key: compactDirections(km.WidgetLeft.Short(), km.WidgetDown.Short(), km.WidgetUp.Short(), km.WidgetRight.Short()), Label: "widgets"},
		{Key: km.WidgetScrollDown.Short() + "/" + km.WidgetScrollUp.Short(), Label: "rows"},
		{Key: km.ActionMenu.Short(), Label: "actions"},
		{Key: km.FilterInput.Short(), Label: "filter"},
		{Key: km.RefreshScope.Short(), Label: "refresh"},
		{Key: km.SubscriptionPicker.Short(), Label: "sub"},
		{Key: km.ToggleHelp.Short(), Label: "help"},
	}
	return actions
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

func (m Model) widgetFrame(title, body string, width, height int, focused, rightRule bool) string {
	km := m.Keymap
	footer := km.WidgetScrollDown.Short() + "/" + km.WidgetScrollUp.Short() + " rows"
	if m.refreshInFlight > 0 {
		footer = m.Styles.Chrome.Loading.Render(m.Spinner.View()) + " refreshing"
	}
	return ui.RenderMillerColumn(ui.MillerColumn{
		Title:  strings.ToUpper(title),
		Body:   body,
		Footer: footer,
		Frame:  ui.MillerColumnFrame{Width: width, Height: height, Focused: focused, RightRule: rightRule},
	}, m.Styles)
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
