package sbapp

import "azure-storage/internal/ui"

func (m *Model) resize() {
	if m.width <= 0 || m.height <= 0 {
		return
	}

	pane := m.styles.Chrome.Pane
	numPanes := 3
	if m.viewingMessage {
		numPanes = 4
	}
	widths := ui.PaneLayout(pane, m.width, numPanes)

	m.paneWidths = [4]int{widths[0], widths[1], widths[2], 0}
	if m.viewingMessage {
		m.paneWidths[3] = widths[3]
	}

	paneFrame := 2 // rounded border top + bottom
	height := m.height - paneFrame - ui.StatusBarHeight - ui.SubscriptionBarHeight
	if height < 8 {
		height = 8
	}
	m.paneHeight = height

	if m.viewingMessage {
		m.detailList.SetSize(ui.PaneContentWidth(pane, widths[2]), height)
		m.messageViewport.Width = ui.PaneContentWidth(pane, widths[3])
		m.messageViewport.Height = height - 2
	} else {
		m.detailList.SetSize(ui.PaneContentWidth(pane, widths[2]), height)
		m.messageViewport.Width = 0
		m.messageViewport.Height = 0
	}

	m.namespacesList.SetSize(ui.PaneContentWidth(pane, widths[0]), height)
	m.entitiesList.SetSize(ui.PaneContentWidth(pane, widths[1]), height)
}
