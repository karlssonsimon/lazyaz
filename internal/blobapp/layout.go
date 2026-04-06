package blobapp

import (
	"azure-storage/internal/ui"
)

func (m *Model) resize() {
	if m.width <= 0 || m.height <= 0 {
		return
	}

	numPanes := 3
	if m.preview.open {
		numPanes = 4
	}
	widths := ui.PaneLayout(m.styles.Chrome.Pane, m.width, numPanes)
	pane := m.styles.Chrome.Pane
	m.paneWidths = [4]int{widths[0], widths[1], widths[2], 0}
	if m.preview.open {
		m.paneWidths[3] = widths[3]
	}

	paneFrame := 2 // rounded border top + bottom
	height := m.height - paneFrame - ui.StatusBarHeight - ui.SubscriptionBarHeight
	if height < 8 {
		height = 8
	}
	m.paneHeight = height

	listHeight := height - ui.PaneHintHeight
	m.accountsList.SetSize(ui.PaneContentWidth(pane, widths[0]), listHeight)
	m.containersList.SetSize(ui.PaneContentWidth(pane, widths[1]), listHeight)
	blobListHeight := listHeight
	if m.search.active {
		blobListHeight -= searchInputHeight
	}
	m.blobsList.SetSize(ui.PaneContentWidth(pane, widths[2]), blobListHeight)
	if m.preview.open {
		m.preview.viewport.Width = ui.PaneContentWidth(pane, widths[3])
		m.preview.viewport.Height = listHeight
	}
}
