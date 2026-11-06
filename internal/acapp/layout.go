package acapp

import "github.com/karlssonsimon/lazyaz/internal/ui"

func (m *Model) resize() {
	if m.Width <= 0 || m.Height <= 0 {
		return
	}

	hasParent := m.focus > storesPane
	cols := ui.MillerLayout(m.Styles.Chrome.Pane, m.Width, hasParent, true)

	m.paneWidths = [3]int{}
	m.paneWidths[m.focus] = cols.Focused
	if m.focus > storesPane {
		m.paneWidths[m.focus-1] = cols.Parent
	}
	if m.focus < valuePane {
		m.paneWidths[m.focus+1] = cols.Child
	}

	height := ui.AppBodyHeight(m.Height)
	if height < 10 {
		height = 10
	}
	m.paneHeight = height

	baseListHeight := ui.MillerListBodyHeight(height, true)
	rightmost := m.focus
	if visible := m.visiblePanes(); len(visible) > 0 {
		rightmost = visible[len(visible)-1].Index
	}
	contentWidth := func(pane, w int) int {
		return ui.MillerColumnContentWidth(ui.MillerColumnFrame{Width: w, RightRule: pane != rightmost})
	}

	if w := m.paneWidths[storesPane]; w > 0 {
		m.storesList.SetSize(contentWidth(storesPane, w), baseListHeight-m.inspectFooterHeight(storesPane))
	}
	if w := m.paneWidths[settingsPane]; w > 0 {
		m.settingsList.SetSize(contentWidth(settingsPane, w), baseListHeight-m.inspectFooterHeight(settingsPane))
	}
	if w := m.paneWidths[valuePane]; w > 0 {
		// The gutter sits outside the viewport so copying text never
		// picks up line numbers.
		colWidth := contentWidth(valuePane, w)
		gutterW := ui.LineGutterWidth(m.valueViewport.TotalLineCount(), valueGutterMinDigits)
		vpWidth := colWidth - gutterW
		if vpWidth < 1 {
			vpWidth = 1
		}
		m.valueViewport.SetWidth(vpWidth)
		m.valueViewport.SetHeight(ui.MillerListBodyHeight(height, false) - 1)
	} else {
		m.valueViewport.SetWidth(0)
		m.valueViewport.SetHeight(0)
	}
}
