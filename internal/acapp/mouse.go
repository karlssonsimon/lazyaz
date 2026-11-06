package acapp

import (
	"github.com/karlssonsimon/lazyaz/internal/ui"

	tea "charm.land/bubbletea/v2"
)

// visiblePanes returns the on-screen pane layout left to right.
func (m Model) visiblePanes() []ui.VisiblePane {
	pw := m.paneWidths
	var panes []ui.VisiblePane
	cols := ui.MillerLayout(m.Styles.Chrome.Pane, m.Width, m.focus > storesPane, true)
	x := ui.MillerSideMargin(cols, m.Width)

	if m.focus > storesPane && pw[m.focus-1] > 0 {
		panes = append(panes, ui.VisiblePane{Index: m.focus - 1, X: x, Width: pw[m.focus-1]})
		x += pw[m.focus-1]
	}
	panes = append(panes, ui.VisiblePane{Index: m.focus, X: x, Width: pw[m.focus]})
	x += pw[m.focus]

	childIdx := m.focus + 1
	if childIdx <= valuePane && pw[childIdx] > 0 {
		panes = append(panes, ui.VisiblePane{Index: childIdx, X: x, Width: pw[childIdx]})
	}
	return panes
}

// listForPane returns the list for a pane, or nil for the value pane.
func (m *Model) listForPane(pane int) *ui.List {
	switch pane {
	case storesPane:
		return &m.storesList
	case settingsPane:
		return &m.settingsList
	default:
		return nil
	}
}

func (m Model) paneAreaY() int {
	y := ui.AppHeaderHeight
	if m.EmbeddedMode {
		y += ui.TabBarHeight
	}
	y++ // the rule between header and columns
	return y
}

// handleMouseClick focuses the clicked pane and selects the row under
// the cursor. Returns (consumed, doubleClick).
func (m *Model) handleMouseClick(msg tea.MouseClickMsg) (bool, bool) {
	if msg.Button != tea.MouseLeft {
		return false, false
	}
	doubleClick := m.clickTracker.Click(msg.X, msg.Y)

	areaY := m.paneAreaY()
	if msg.Y < areaY || msg.Y >= areaY+m.paneHeight {
		return false, false
	}
	vp := ui.PaneAtX(m.visiblePanes(), msg.X)
	if vp == nil {
		return false, false
	}
	if vp.Index == valuePane && !m.viewingValue {
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
			if vp.Index == settingsPane {
				m.syncValueToSelection()
			}
		}
	}
	return true, doubleClick
}
