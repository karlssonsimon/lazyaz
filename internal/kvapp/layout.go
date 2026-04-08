package kvapp

import "github.com/karlssonsimon/lazyaz/internal/ui"

func (m *Model) resize() {
	if m.Width <= 0 || m.Height <= 0 {
		return
	}

	widths := ui.PaneLayout(m.Styles.Chrome.Pane, m.Width, 3)
	pane := m.Styles.Chrome.Pane
	m.paneWidths = [3]int{widths[0], widths[1], widths[2]}

	// paneHeight is the total block height of each pane (border + content),
	// i.e. the number of terminal rows the pane occupies.
	height := m.Height - ui.StatusBarHeight - ui.SubscriptionBarHeight
	if height < 10 {
		height = 10
	}
	m.paneHeight = height

	innerH := ui.PaneInnerHeight(pane, height)
	baseListHeight := innerH - ui.PaneTitleHeight - ui.PaneHintHeight
	if baseListHeight < 1 {
		baseListHeight = 1
	}
	m.vaultsList.SetSize(ui.PaneContentWidth(pane, widths[0]), baseListHeight-m.inspectFooterHeight(vaultsPane))
	m.secretsList.SetSize(ui.PaneContentWidth(pane, widths[1]), baseListHeight-m.inspectFooterHeight(secretsPane))
	m.versionsList.SetSize(ui.PaneContentWidth(pane, widths[2]), baseListHeight-m.inspectFooterHeight(versionsPane))
}
