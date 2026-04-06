package kvapp

import "azure-storage/internal/ui"

func (m *Model) resize() {
	if m.Width <= 0 || m.Height <= 0 {
		return
	}

	widths := ui.PaneLayout(m.Styles.Chrome.Pane, m.Width, 3)
	pane := m.Styles.Chrome.Pane
	m.paneWidths = [3]int{widths[0], widths[1], widths[2]}

	paneFrame := 2 // rounded border top + bottom
	height := m.Height - paneFrame - ui.StatusBarHeight - ui.SubscriptionBarHeight
	if height < 8 {
		height = 8
	}
	m.paneHeight = height

	baseListHeight := height - ui.PaneTitleHeight - ui.PaneHintHeight
	m.vaultsList.SetSize(ui.PaneContentWidth(pane, widths[0]), baseListHeight-m.inspectFooterHeight(vaultsPane))
	m.secretsList.SetSize(ui.PaneContentWidth(pane, widths[1]), baseListHeight-m.inspectFooterHeight(secretsPane))
	m.versionsList.SetSize(ui.PaneContentWidth(pane, widths[2]), baseListHeight-m.inspectFooterHeight(versionsPane))
}
