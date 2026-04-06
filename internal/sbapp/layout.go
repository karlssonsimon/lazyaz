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

	paneFrame := 2 // rounded border top + bottom
	height := m.Height - paneFrame - ui.StatusBarHeight - ui.SubscriptionBarHeight
	if height < 8 {
		height = 8
	}
	m.paneHeight = height

	baseListHeight := height - ui.PaneTitleHeight - ui.PaneHintHeight

	detailListHeight := baseListHeight - m.inspectFooterHeight(detailPane)
	if m.hasPeekTarget {
		detailListHeight -= dlqTabsHeight
	}

	if m.viewingMessage {
		m.detailList.SetSize(ui.PaneContentWidth(pane, widths[2]), detailListHeight)
		m.messageViewport.Width = ui.PaneContentWidth(pane, widths[3])
		m.messageViewport.Height = baseListHeight - 2
	} else {
		m.detailList.SetSize(ui.PaneContentWidth(pane, widths[2]), detailListHeight)
		m.messageViewport.Width = 0
		m.messageViewport.Height = 0
	}

	m.namespacesList.SetSize(ui.PaneContentWidth(pane, widths[0]), baseListHeight-m.inspectFooterHeight(namespacesPane))
	entitiesListHeight := baseListHeight - m.inspectFooterHeight(entitiesPane)
	if m.hasNamespace {
		entitiesListHeight -= entityTabsHeight
	}
	m.entitiesList.SetSize(ui.PaneContentWidth(pane, widths[1]), entitiesListHeight)
}
