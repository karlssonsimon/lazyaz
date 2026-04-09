package kvapp

import "github.com/karlssonsimon/lazyaz/internal/ui"

func (m *Model) resize() {
	if m.Width <= 0 || m.Height <= 0 {
		return
	}

	pane := m.Styles.Chrome.Pane

	// Determine parent/child visibility based on focus.
	// Always reserve parent space — when vaults is focused, a spacer
	// fills the left column to keep the focused pane centered.
	hasParent := true
	hasChild := false
	switch m.focus {
	case vaultsPane:
		hasChild = m.hasVault // show secrets preview
	case secretsPane:
		hasChild = m.hasSecret // show versions preview
	case versionsPane:
		hasChild = false // rightmost pane, no child
	}

	cols := ui.MillerLayout(pane, m.Width, hasParent, hasChild)

	// Map roles → pane indices. The focused pane is always center.
	// Parent is the pane before focus, child is the pane after.
	m.paneWidths = [3]int{} // reset all to 0
	m.paneWidths[m.focus] = cols.Focused
	if m.focus > vaultsPane {
		m.paneWidths[m.focus-1] = cols.Parent
	}
	if hasChild {
		m.paneWidths[m.focus+1] = cols.Child
	}

	// Height.
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

	// Size each visible list to its pane width.
	if w := m.paneWidths[vaultsPane]; w > 0 {
		m.vaultsList.SetSize(ui.PaneContentWidth(pane, w), baseListHeight-m.inspectFooterHeight(vaultsPane))
	}
	if w := m.paneWidths[secretsPane]; w > 0 {
		m.secretsList.SetSize(ui.PaneContentWidth(pane, w), baseListHeight-m.inspectFooterHeight(secretsPane))
	}
	if w := m.paneWidths[versionsPane]; w > 0 {
		m.versionsList.SetSize(ui.PaneContentWidth(pane, w), baseListHeight-m.inspectFooterHeight(versionsPane))
	}
}
