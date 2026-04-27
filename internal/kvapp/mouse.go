package kvapp

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

	if m.focus > vaultsPane && pw[m.focus-1] > 0 {
		panes = append(panes, ui.VisiblePane{Index: m.focus - 1, X: x, Width: pw[m.focus-1]})
		x += pw[m.focus-1]
	} else if m.focus == vaultsPane {
		x += m.Width * 20 / 100
	}

	panes = append(panes, ui.VisiblePane{Index: m.focus, X: x, Width: pw[m.focus]})
	x += pw[m.focus]

	childIdx := m.focus + 1
	if childIdx <= versionsPane && pw[childIdx] > 0 {
		panes = append(panes, ui.VisiblePane{Index: childIdx, X: x, Width: pw[childIdx]})
	}

	return panes
}

// listForPane returns the list.Model pointer for the given pane index, or nil.
func (m *Model) listForPane(pane int) *list.Model {
	switch pane {
	case vaultsPane:
		return &m.vaultsList
	case secretsPane:
		return &m.secretsList
	case versionsPane:
		return &m.versionsList
	default:
		return nil
	}
}

// paneAreaY returns the absolute screen Y where the pane area starts.
func (m Model) paneAreaY() int {
	y := ui.AppHeaderHeight
	if m.EmbeddedMode {
		y += ui.TabBarHeight
	}
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
