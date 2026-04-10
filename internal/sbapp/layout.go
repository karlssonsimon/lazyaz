package sbapp

import "github.com/karlssonsimon/lazyaz/internal/ui"

func (m *Model) resize() {
	if m.Width <= 0 || m.Height <= 0 {
		return
	}

	pane := m.Styles.Chrome.Pane

	hasParent := true
	hasChild := false
	switch m.focus {
	case namespacesPane:
		hasChild = m.hasNamespace
	case entitiesPane:
		hasChild = m.isTopicSelected() || m.hasPeekTarget
	case subscriptionsPane:
		hasChild = m.hasPeekTarget
	case queueTypePane:
		hasChild = len(m.peekedMessages) > 0
	case messagesPane:
		hasChild = m.viewingMessage
	}

	cols := ui.MillerLayout(pane, m.Width, hasParent, hasChild)

	m.paneWidths = [6]int{}
	m.paneWidths[m.focus] = cols.Focused

	if m.focus > namespacesPane {
		parentIdx := m.parentPane()
		if parentIdx >= 0 {
			m.paneWidths[parentIdx] = cols.Parent
		}
	}
	if hasChild {
		childIdx := m.childPane()
		if childIdx >= 0 && childIdx <= messagePreviewPane {
			m.paneWidths[childIdx] = cols.Child
		}
	}

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

	if w := m.paneWidths[namespacesPane]; w > 0 {
		m.namespacesList.SetSize(ui.PaneContentWidth(pane, w), baseListHeight-m.inspectFooterHeight(namespacesPane))
	}
	if w := m.paneWidths[entitiesPane]; w > 0 {
		m.entitiesList.SetSize(ui.PaneContentWidth(pane, w), baseListHeight-m.inspectFooterHeight(entitiesPane))
	}
	if w := m.paneWidths[subscriptionsPane]; w > 0 {
		m.subscriptionsList.SetSize(ui.PaneContentWidth(pane, w), baseListHeight-m.inspectFooterHeight(subscriptionsPane))
	}
	if w := m.paneWidths[queueTypePane]; w > 0 {
		m.queueTypeList.SetSize(ui.PaneContentWidth(pane, w), baseListHeight)
	}
	if w := m.paneWidths[messagesPane]; w > 0 {
		m.messageList.SetSize(ui.PaneContentWidth(pane, w), baseListHeight-m.inspectFooterHeight(messagesPane))
	}
	if w := m.paneWidths[messagePreviewPane]; w > 0 {
		m.messageViewport.SetWidth(ui.PaneContentWidth(pane, w))
		m.messageViewport.SetHeight(baseListHeight - 2)
	} else {
		m.messageViewport.SetWidth(0)
		m.messageViewport.SetHeight(0)
	}
}

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
