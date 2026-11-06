package acapp

import (
	"fmt"

	"github.com/karlssonsimon/lazyaz/internal/ui"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func (m Model) View() tea.View {
	if m.Width == 0 || m.Height == 0 {
		v := tea.NewView("loading...")
		v.AltScreen = true
		return v
	}

	pw := m.paneWidths
	h := m.paneHeight
	rightmost := m.focus
	if visible := m.visiblePanes(); len(visible) > 0 {
		rightmost = visible[len(visible)-1].Index
	}
	frame := func(pane int) ui.MillerColumnFrame {
		return ui.MillerColumnFrame{Width: pw[pane], Height: h, Focused: m.focus == pane, RightRule: pane != rightmost}
	}
	footer := func(pane int, l *ui.List) string {
		f := frame(pane)
		contentWidth := ui.MillerColumnContentWidth(f)
		base := m.columnFooter(pane)
		if inspect := m.inspectFooter(pane, contentWidth); inspect != "" {
			base = lipgloss.JoinVertical(lipgloss.Left, base, inspect)
		}
		if pane != m.focus || l == nil {
			return base
		}
		switch l.FilterState() {
		case list.Filtering:
			before, cv, after := ui.PrepareCursor(l.FilterInput.Value(), l.FilterInput.Position(), m.Cursor)
			return ui.RenderFilterLine(before, cv, after, l.FilterInput.Value(), m.Styles, contentWidth, true)
		case list.FilterApplied:
			return lipgloss.JoinVertical(lipgloss.Left,
				ui.RenderFilterLine("", "", "", l.FilterValue(), m.Styles, contentWidth, false),
				base)
		}
		return base
	}

	paneMap := map[int]string{}
	if pw[storesPane] > 0 {
		paneMap[storesPane] = ui.RenderMillerListColumn(ui.MillerListColumn{
			List:      &m.storesList,
			Title:     "STORES",
			TitleMeta: m.columnTitleMeta(storesPane),
			Footer:    footer(storesPane, &m.storesList),
			Frame:     frame(storesPane),
		}, m.Styles)
	}
	if pw[settingsPane] > 0 {
		paneMap[settingsPane] = ui.RenderMillerListColumn(ui.MillerListColumn{
			List:      &m.settingsList,
			Title:     "SETTINGS",
			TitleMeta: m.columnTitleMeta(settingsPane),
			Footer:    footer(settingsPane, &m.settingsList),
			Frame:     frame(settingsPane),
		}, m.Styles)
	}
	if pw[valuePane] > 0 {
		paneMap[valuePane] = m.renderValueColumn(frame(valuePane))
	}

	plainSpacer := func(width int) string {
		if width <= 0 {
			return ""
		}
		return lipgloss.NewStyle().Width(width).Height(h).Render("")
	}
	paneParts := make([]string, 0, 3)
	if m.focus > storesPane && pw[m.focus-1] > 0 {
		paneParts = append(paneParts, paneMap[m.focus-1])
	}
	paneParts = append(paneParts, paneMap[m.focus])
	if childIdx := m.focus + 1; childIdx <= valuePane && pw[childIdx] > 0 {
		if rendered, ok := paneMap[childIdx]; ok {
			paneParts = append(paneParts, rendered)
		} else {
			paneParts = append(paneParts, plainSpacer(pw[childIdx]))
		}
	}

	cols := ui.MillerLayout(m.Styles.Chrome.Pane, m.Width, m.focus > storesPane, true)
	if margin := ui.MillerSideMargin(cols, m.Width); margin > 0 {
		paneParts = append([]string{plainSpacer(margin)}, paneParts...)
	}
	panes := lipgloss.JoinHorizontal(lipgloss.Top, paneParts...)

	header := ui.RenderAppHeader(ui.HeaderConfig{
		Brand: "lazyaz",
		Path:  m.headerPath(),
		Meta:  ui.HeaderMeta(m.CurrentSub, m.HasSubscription, m.Styles),
	}, m.Styles, m.Width)
	statusBar := ui.RenderStatusLine(ui.StatusLineConfig{
		Mode:    m.inputMode().String(),
		Count:   m.vimr.PendingCount(),
		Actions: m.statusActions(),
	}, m.Styles, m.Width)
	ticks := m.columnTickPositions()
	topRule := ui.RenderHorizontalRule(m.Width, m.Styles, ticks)
	bottomRule := ui.RenderHorizontalRuleBottom(m.Width, m.Styles, ticks)

	view := ui.RenderCanvas(lipgloss.JoinVertical(lipgloss.Left, header, topRule, panes, bottomRule, statusBar), m.Width, m.Height, m.Styles.Bg)
	if m.copyOverlay.Active {
		view = ui.RenderCopyOverlay(m.copyOverlay, m.Keymap, m.Cursor, m.Styles, m.Width, m.Height, view)
	}
	out := tea.NewView(m.RenderOverlays(view))
	out.AltScreen = true
	out.MouseMode = tea.MouseModeCellMotion
	return out
}

// renderValueColumn is the value pane: a title line naming the setting,
// then the value with a line-number gutter. Before a value is opened it
// explains how to open one.
func (m Model) renderValueColumn(f ui.MillerColumnFrame) string {
	contentWidth := ui.MillerColumnContentWidth(f)
	var body string
	if m.viewingValue {
		title := m.Styles.Accent.Copy().Width(contentWidth).MaxWidth(contentWidth).Render(m.valueTitle())
		gutter := ui.RenderLineGutter(m.valueViewport, m.Styles, valueGutterMinDigits)
		body = lipgloss.JoinVertical(lipgloss.Left, title, lipgloss.JoinHorizontal(lipgloss.Top, gutter, m.valueViewport.View()))
	} else {
		body = m.Styles.Muted.Render(fmt.Sprintf("%s on a setting shows its value here", m.Keymap.OpenFocused.Short()))
	}
	return ui.RenderMillerColumn(ui.MillerColumn{
		Title:  "VALUE",
		Body:   body,
		Footer: m.valueStatusHint(),
		Frame:  f,
	}, m.Styles)
}

func (m Model) headerPath() []string {
	var path []string
	if m.HasSubscription {
		path = append(path, ui.SubscriptionDisplayName(m.CurrentSub))
	}
	if m.hasStore {
		path = append(path, m.currentStore.Name)
	}
	if m.viewingValue {
		path = append(path, m.currentSetting.DisplayKey())
	}
	return path
}

func (m Model) columnTickPositions() []int {
	pw := m.paneWidths
	parentVisible := m.focus > storesPane && pw[m.focus-1] > 0
	childIdx := m.focus + 1
	hasChild := childIdx <= valuePane && pw[childIdx] > 0

	cols := ui.MillerLayout(m.Styles.Chrome.Pane, m.Width, m.focus > storesPane, true)
	pos := ui.MillerSideMargin(cols, m.Width)
	var ticks []int
	if parentVisible {
		pos += pw[m.focus-1]
		ticks = append(ticks, pos-1)
	}
	pos += pw[m.focus]
	if hasChild {
		ticks = append(ticks, pos-1)
	}
	return ticks
}

func (m Model) statusActions() []ui.StatusAction {
	km := m.Keymap
	if m.focus == valuePane {
		return []ui.StatusAction{
			{Key: km.CursorDown.Short() + "/" + km.CursorUp.Short(), Label: "scroll"},
			{Key: km.YankSecret.Short(), Label: "yank"},
			{Key: km.CopyPalette.Short(), Label: "copy"},
			{Key: km.NavigateLeft.Short(), Label: "back"},
			{Key: km.ToggleHelp.Short(), Label: "help"},
		}
	}
	actions := []ui.StatusAction{
		{Key: km.CursorDown.Short() + "/" + km.CursorUp.Short(), Label: "move"},
		{Key: km.OpenFocusedAlt.Short(), Label: "open"},
		{Key: km.NavigateLeft.Short(), Label: "back"},
		{Key: km.FilterInput.Short(), Label: "filter"},
		{Key: km.RefreshScope.Short(), Label: "refresh"},
		{Key: km.ToggleHelp.Short(), Label: "help"},
	}
	if m.focus == settingsPane {
		actions = append(actions,
			ui.StatusAction{Key: km.YankSecret.Short(), Label: "yank"},
			ui.StatusAction{Key: km.Inspect.Short(), Label: "inspect"},
		)
	}
	return actions
}

func (m Model) columnFooter(pane int) string {
	l := m.listForPane(pane)
	if l == nil {
		return ""
	}
	idx := l.Index() + 1
	count := len(l.VisibleItems())
	if count == 0 {
		idx = 0
	}
	return fmt.Sprintf("%d of %d · ↕ j/k", idx, count)
}

func (m Model) columnTitleMeta(pane int) string {
	switch pane {
	case storesPane:
		if total := len(m.stores); total > 0 {
			return fmt.Sprintf("%d total", total)
		}
	case settingsPane:
		total := len(m.settings)
		if total == 0 {
			return ""
		}
		if shown := len(m.settingsList.VisibleItems()); shown != total {
			return fmt.Sprintf("%d / %d", shown, total)
		}
		return fmt.Sprintf("%d total", total)
	}
	return ""
}
