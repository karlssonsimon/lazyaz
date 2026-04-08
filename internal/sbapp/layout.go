package sbapp

import "github.com/karlssonsimon/lazyaz/internal/ui"

func (m *Model) resize() {
	if m.Width <= 0 || m.Height <= 0 {
		return
	}

	pane := m.Styles.Chrome.Pane
	numPanes := 3
	if m.viewingMessage {
		numPanes = 4
	}
	widths := ui.PaneLayout(pane, m.Width, numPanes)

	m.paneWidths = [4]int{widths[0], widths[1], widths[2], 0}
	if m.viewingMessage {
		m.paneWidths[3] = widths[3]
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

	detailListHeight := baseListHeight - m.inspectFooterHeight(detailPane)
	if m.hasPeekTarget {
		detailListHeight -= dlqTabsHeight
	}

	if m.viewingMessage {
		m.detailList.SetSize(ui.PaneContentWidth(pane, widths[2]), detailListHeight)
		m.messageViewport.SetWidth(ui.PaneContentWidth(pane, widths[3]))
		m.messageViewport.SetHeight(baseListHeight - 2)
	} else {
		m.detailList.SetSize(ui.PaneContentWidth(pane, widths[2]), detailListHeight)
		m.messageViewport.SetWidth(0)
		m.messageViewport.SetHeight(0)
	}

	m.namespacesList.SetSize(ui.PaneContentWidth(pane, widths[0]), baseListHeight-m.inspectFooterHeight(namespacesPane))
	entitiesListHeight := baseListHeight - m.inspectFooterHeight(entitiesPane)
	if m.hasNamespace {
		entitiesListHeight -= entityTabsHeight
	}
	m.entitiesList.SetSize(ui.PaneContentWidth(pane, widths[1]), entitiesListHeight)
}
