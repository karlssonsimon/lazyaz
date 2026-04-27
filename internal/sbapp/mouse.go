package sbapp

import (
	"github.com/karlssonsimon/lazyaz/internal/ui"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
)

// visiblePanes returns the on-screen pane layout in left-to-right order.
func (m Model) visiblePanes() []ui.VisiblePane {
	pw := m.paneWidths
	var panes []ui.VisiblePane
	x := 0

	parentIdx := m.parentPane()
	if parentIdx >= 0 && pw[parentIdx] > 0 {
		panes = append(panes, ui.VisiblePane{Index: parentIdx, X: x, Width: pw[parentIdx]})
		x += pw[parentIdx]
	} else if m.focus == namespacesPane {
		x += m.Width * 20 / 100
	}

	panes = append(panes, ui.VisiblePane{Index: m.focus, X: x, Width: pw[m.focus]})
	x += pw[m.focus]

	childIdx := m.childPane()
	if childIdx >= 0 && pw[childIdx] > 0 {
		panes = append(panes, ui.VisiblePane{Index: childIdx, X: x, Width: pw[childIdx]})
	}

	return panes
}

// listForPane returns the list.Model pointer for the given pane index, or nil.
func (m *Model) listForPane(pane int) *list.Model {
	switch pane {
	case namespacesPane:
		return &m.namespacesList
	case entitiesPane:
		return &m.entitiesList
	case subscriptionsPane:
		return &m.subscriptionsList
	case queueTypePane:
		return &m.queueTypeList
	case messagesPane:
		return &m.messageList
	default:
		return nil
	}
}

// paneAreaY returns the absolute screen Y where the pane area starts.
// Accounts for the tab bar (when embedded), app header, and the
// full-width horizontal rule rendered between the header and columns.
func (m Model) paneAreaY() int {
	y := ui.AppHeaderHeight
	if m.EmbeddedMode {
		y += ui.TabBarHeight
	}
	// +1 for the horizontal rule between the header and the columns.
	y++
	return y
}

// handleMouseClick handles a left-click on a list pane: focuses the pane
// and selects the item under the cursor. Returns (consumed, doubleClick).
func (m *Model) handleMouseClick(msg tea.MouseClickMsg) (bool, bool) {
	if msg.Button != tea.MouseLeft {
		return false, false
	}

	doubleClick := m.clickTracker.Click(msg.X, msg.Y)

	areaY := m.paneAreaY()
	areaBottom := areaY + m.paneHeight
	if msg.Y < areaY || msg.Y >= areaBottom {
		return false, false
	}

	vp := ui.PaneAtX(m.visiblePanes(), msg.X)
	if vp == nil {
		return false, false
	}

	if vp.Index == messagePreviewPane {
		return false, false
	}

	if vp.Index != m.focus {
		m.transitionTo(vp.Index)
	}

	contentY := ui.MillerColumnContentYStart(areaY)
	localY := msg.Y - contentY
	itemH := m.Styles.Delegate.Height() + m.Styles.Delegate.Spacing()
	if l := m.listForPane(vp.Index); l != nil && localY >= 0 {
		if idx := ui.ListItemAtY(l, localY, itemH); idx >= 0 {
			l.Select(idx)
		}
	}

	return true, doubleClick
}
