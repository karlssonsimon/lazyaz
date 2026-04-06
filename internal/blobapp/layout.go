package blobapp

import (
	"github.com/karlssonsimon/lazyaz/internal/ui"
)

func (m *Model) resize() {
	if m.Width <= 0 || m.Height <= 0 {
		return
	}

	numPanes := 3
	if m.preview.open {
		numPanes = 4
	}
	widths := ui.PaneLayout(m.Styles.Chrome.Pane, m.Width, numPanes)
	pane := m.Styles.Chrome.Pane
	m.paneWidths = [4]int{widths[0], widths[1], widths[2], 0}
	if m.preview.open {
		m.paneWidths[3] = widths[3]
	}

	paneFrame := 2 // rounded border top + bottom
	height := m.Height - paneFrame - ui.StatusBarHeight - ui.SubscriptionBarHeight
	if height < 8 {
		height = 8
	}
	m.paneHeight = height

	baseListHeight := height - ui.PaneTitleHeight - ui.PaneHintHeight
	m.accountsList.SetSize(ui.PaneContentWidth(pane, widths[0]), baseListHeight-m.inspectFooterHeight(accountsPane))
	m.containersList.SetSize(ui.PaneContentWidth(pane, widths[1]), baseListHeight-m.inspectFooterHeight(containersPane))
	blobListHeight := baseListHeight - m.inspectFooterHeight(blobsPane)
	if m.search.active {
		blobListHeight -= searchInputHeight
	}
	m.blobsList.SetSize(ui.PaneContentWidth(pane, widths[2]), blobListHeight)
	if m.preview.open {
		m.preview.viewport.Width = ui.PaneContentWidth(pane, widths[3])
		m.preview.viewport.Height = baseListHeight
	}
}
