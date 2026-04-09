package sbapp

import "github.com/karlssonsimon/lazyaz/internal/ui"

func (m *Model) resize() {
	if m.Width <= 0 || m.Height <= 0 {
		return
	}

	pane := m.Styles.Chrome.Pane

	// Determine parent/child visibility based on focus.
	// Always reserve parent space — when namespaces is focused, a spacer
	// fills the left column to keep the focused pane centered.
	hasParent := true
	hasChild := false
	switch m.focus {
	case namespacesPane:
		hasChild = m.hasNamespace // show entities preview
	case entitiesPane:
		hasChild = m.hasPeekTarget // show detail preview
	case detailPane:
		hasChild = m.viewingMessage // show message preview
	}

	cols := ui.MillerLayout(pane, m.Width, hasParent, hasChild)

	// Map roles → pane indices. The focused pane is always center.
	// Parent is the pane before focus, child is the pane after.
	m.paneWidths = [4]int{} // reset all to 0
	m.paneWidths[m.focus] = cols.Focused
	if m.focus > namespacesPane {
		m.paneWidths[m.focus-1] = cols.Parent
	}
	if hasChild {
		childIdx := m.focus + 1
		if m.focus == detailPane {
			childIdx = messagePreviewPane
		}
		m.paneWidths[childIdx] = cols.Child
	}

	// paneHeight is the total block height of each pane (border + content),
	// i.e. the number of terminal rows the pane occupies. The view stacks
	// subscription bar + pane row + status bar to fill the window.
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
	if w := m.paneWidths[namespacesPane]; w > 0 {
		m.namespacesList.SetSize(ui.PaneContentWidth(pane, w), baseListHeight-m.inspectFooterHeight(namespacesPane))
	}
	if w := m.paneWidths[entitiesPane]; w > 0 {
		entitiesListHeight := baseListHeight - m.inspectFooterHeight(entitiesPane)
		if m.hasNamespace {
			entitiesListHeight -= entityTabsHeight
		}
		m.entitiesList.SetSize(ui.PaneContentWidth(pane, w), entitiesListHeight)
	}
	if w := m.paneWidths[detailPane]; w > 0 {
		detailListHeight := baseListHeight - m.inspectFooterHeight(detailPane)
		if m.hasPeekTarget {
			detailListHeight -= dlqTabsHeight
		}
		m.detailList.SetSize(ui.PaneContentWidth(pane, w), detailListHeight)
	}
	if w := m.paneWidths[messagePreviewPane]; w > 0 {
		m.messageViewport.SetWidth(ui.PaneContentWidth(pane, w))
		m.messageViewport.SetHeight(baseListHeight - 2)
	} else {
		m.messageViewport.SetWidth(0)
		m.messageViewport.SetHeight(0)
	}
}
