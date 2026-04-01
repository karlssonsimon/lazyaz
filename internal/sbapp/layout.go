package sbapp

import "azure-storage/internal/ui"

func (m *Model) resize() {
	if m.width <= 0 || m.height <= 0 {
		return
	}

	widths := ui.PaneLayout(m.styles.Chrome.Pane, m.width, 4)
	pane := m.styles.Chrome.Pane
	m.paneWidths = [4]int{widths[0], widths[1], widths[2], widths[3]}

	height := m.height - 10
	if height < 8 {
		height = 8
	}
	m.paneHeight = height

	detContent := ui.PaneContentWidth(pane, widths[3])
	if m.viewingMessage {
		detHalf := detContent / 3
		if detHalf < 30 {
			detHalf = 30
		}
		previewW := detContent - detHalf - 3
		if previewW < 30 {
			previewW = 30
		}
		m.detailList.SetSize(detHalf, height)
		m.messageViewport.Width = previewW
		m.messageViewport.Height = height - 2
	} else {
		m.detailList.SetSize(detContent, height)
		m.messageViewport.Width = 0
		m.messageViewport.Height = 0
	}

	m.subscriptionsList.SetSize(ui.PaneContentWidth(pane, widths[0]), height)
	m.namespacesList.SetSize(ui.PaneContentWidth(pane, widths[1]), height)
	m.entitiesList.SetSize(ui.PaneContentWidth(pane, widths[2]), height)
}
