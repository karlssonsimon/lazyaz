package kvapp

import "azure-storage/internal/ui"

func (m *Model) resize() {
	if m.width <= 0 || m.height <= 0 {
		return
	}

	widths := ui.PaneLayout(m.styles.Chrome.Pane, m.width, 4)
	pane := m.styles.Chrome.Pane
	m.paneWidths = [4]int{widths[0], widths[1], widths[2], widths[3]}

	paneFrame := 2 // rounded border top + bottom
	height := m.height - paneFrame - ui.StatusBarHeight
	if height < 8 {
		height = 8
	}
	m.paneHeight = height

	m.subscriptionsList.SetSize(ui.PaneContentWidth(pane, widths[0]), height)
	m.vaultsList.SetSize(ui.PaneContentWidth(pane, widths[1]), height)
	m.secretsList.SetSize(ui.PaneContentWidth(pane, widths[2]), height)
	m.versionsList.SetSize(ui.PaneContentWidth(pane, widths[3]), height)
}
