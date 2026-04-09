package blobapp

import (
	"github.com/karlssonsimon/lazyaz/internal/ui"
)

func (m *Model) resize() {
	if m.Width <= 0 || m.Height <= 0 {
		return
	}

	numPanes := 3
	if m.preview.open {
		numPanes = 4
	}
	widths := ui.PaneLayout(m.Styles.Chrome.Pane, m.Width, numPanes)
	pane := m.Styles.Chrome.Pane
	m.paneWidths = [4]int{widths[0], widths[1], widths[2], 0}
	if m.preview.open {
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
	m.accountsList.SetSize(ui.PaneContentWidth(pane, widths[0]), baseListHeight-m.inspectFooterHeight(accountsPane))
	m.containersList.SetSize(ui.PaneContentWidth(pane, widths[1]), baseListHeight-m.inspectFooterHeight(containersPane))
	blobListHeight := baseListHeight - m.inspectFooterHeight(blobsPane)
	if m.filter.inputOpen {
		blobListHeight -= m.filterInputHeight()
	} else if m.hasActiveFilter() {
		blobListHeight -= 2 // filter banner + entry count
	}
	m.blobsList.SetSize(ui.PaneContentWidth(pane, widths[2]), blobListHeight)
	if m.preview.open {
		m.preview.viewport.SetWidth(ui.PaneContentWidth(pane, widths[3]))
		m.preview.viewport.SetHeight(baseListHeight)
	}
}
