package kvapp

import "github.com/karlssonsimon/lazyaz/internal/ui"

func (m *Model) resize() {
	if m.Width <= 0 || m.Height <= 0 {
		return
	}

	// hasParent=false at the topmost column lets focus absorb the
	// parent slot (~80%); drilled levels keep three slots so focus
	// stays stable at ~60%.
	hasParent := m.focus > vaultsPane
	cols := ui.MillerLayout(m.Styles.Chrome.Pane, m.Width, hasParent, true)

	m.paneWidths = [3]int{}
	m.paneWidths[m.focus] = cols.Focused
	if m.focus > vaultsPane {
		m.paneWidths[m.focus-1] = cols.Parent
	}
	if m.focus < versionsPane {
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

	// Size each visible list to its pane width.
	if w := m.paneWidths[vaultsPane]; w > 0 {
		m.vaultsList.SetSize(ui.MillerContentWidth(ui.MillerColumnFrame{Width: w, RightRule: vaultsPane != rightmost}), baseListHeight-m.inspectFooterHeight(vaultsPane))
	}
	if w := m.paneWidths[secretsPane]; w > 0 {
		m.secretsList.SetSize(ui.MillerContentWidth(ui.MillerColumnFrame{Width: w, RightRule: secretsPane != rightmost}), baseListHeight-m.inspectFooterHeight(secretsPane))
	}
	if w := m.paneWidths[versionsPane]; w > 0 {
		m.versionsList.SetSize(ui.MillerContentWidth(ui.MillerColumnFrame{Width: w, RightRule: versionsPane != rightmost}), baseListHeight-m.inspectFooterHeight(versionsPane))
	}
}
