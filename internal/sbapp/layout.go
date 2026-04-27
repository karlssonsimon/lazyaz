package sbapp

import "github.com/karlssonsimon/lazyaz/internal/ui"

func (m *Model) resize() {
	if m.Width <= 0 || m.Height <= 0 {
		return
	}

	// hasParent=false at the topmost column lets focus absorb the
	// parent slot (focus ~80%); drilled levels keep three slots so
	// focus stays stable at ~60%.
	hasParent := m.focus > namespacesPane
	cols := ui.MillerLayout(m.Styles.Chrome.Pane, m.Width, hasParent, true)

	m.paneWidths = [6]int{}
	m.paneWidths[m.focus] = cols.Focused

	if parentIdx := m.parentPane(); parentIdx >= 0 {
		m.paneWidths[parentIdx] = cols.Parent
	}
	if childIdx := m.childPane(); childIdx >= 0 && childIdx <= messagePreviewPane {
		m.paneWidths[childIdx] = cols.Child
	}

	height := ui.AppBodyHeight(m.Height)
	if height < 10 {
		height = 10
	}
	m.paneHeight = height

	baseListHeight := ui.MillerListBodyHeight(height, true)
	rightmostPane := m.focus
	if childIdx := m.childPane(); childIdx >= 0 && m.paneWidths[childIdx] > 0 {
		rightmostPane = childIdx
	}
	contentWidth := func(pane int, w int) int {
		return ui.MillerContentWidth(ui.MillerColumnFrame{Width: w, RightRule: pane != rightmostPane})
	}

	if w := m.paneWidths[namespacesPane]; w > 0 {
		m.namespacesList.SetSize(contentWidth(namespacesPane, w), baseListHeight-m.inspectFooterHeight(namespacesPane))
	}
	if w := m.paneWidths[entitiesPane]; w > 0 {
		m.entitiesList.SetSize(contentWidth(entitiesPane, w), baseListHeight-m.inspectFooterHeight(entitiesPane))
	}
	if w := m.paneWidths[subscriptionsPane]; w > 0 {
		m.subscriptionsList.SetSize(contentWidth(subscriptionsPane, w), baseListHeight-m.inspectFooterHeight(subscriptionsPane))
	}
	if w := m.paneWidths[queueTypePane]; w > 0 {
		m.queueTypeList.SetSize(contentWidth(queueTypePane, w), baseListHeight-m.inspectFooterHeight(queueTypePane))
	}
	if w := m.paneWidths[messagesPane]; w > 0 {
		m.messageList.SetSize(contentWidth(messagesPane, w), baseListHeight-m.inspectFooterHeight(messagesPane))
		if len(m.peekedMessages) > 0 {
			m.refreshMessageItems()
		}
	}
	if w := m.paneWidths[messagePreviewPane]; w > 0 {
		// Reserve gutter width so JoinHorizontal(gutter, view) fits
		// the column. Gutter is rendered outside the viewport so
		// text selection / copy doesn't include line numbers.
		colWidth := contentWidth(messagePreviewPane, w)
		gutterW := ui.LineGutterWidth(m.messageViewport.TotalLineCount(), previewGutterMinDigits)
		vpWidth := colWidth - gutterW
		if vpWidth < 1 {
			vpWidth = 1
		}
		m.messageViewport.SetWidth(vpWidth)
		m.messageViewport.SetHeight(ui.MillerListBodyHeight(height, false) - 1)
	} else {
		m.messageViewport.SetWidth(0)
		m.messageViewport.SetHeight(0)
	}
}

// previewGutterMinDigits is the minimum digit width reserved for the
// line-number gutter beside message preview content.
const previewGutterMinDigits = 3

// parentPane returns the logical parent pane index for the current
// focus, skipping panes that aren't active. Returns -1 if none.
func (m Model) parentPane() int {
	panes := m.navigablePanes()
	for i, p := range panes {
		if p == m.focus && i > 0 {
			return panes[i-1]
		}
	}
	return -1
}

// childPane returns the logical child pane index for the current
// focus, skipping panes that aren't active. Returns -1 if none.
func (m Model) childPane() int {
	panes := m.navigablePanes()
	for i, p := range panes {
		if p == m.focus && i < len(panes)-1 {
			return panes[i+1]
		}
	}
	return -1
}
