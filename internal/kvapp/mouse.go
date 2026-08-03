package kvapp

import (
	"github.com/karlssonsimon/lazyaz/internal/ui"

	tea "charm.land/bubbletea/v2"
)

// visiblePanes returns the on-screen pane layout in left-to-right order,
// matching the render order in View(): centering margin, then parent,
// focused, child.
func (m Model) visiblePanes() []ui.VisiblePane {
	pw := m.paneWidths
	var panes []ui.VisiblePane
	cols := ui.MillerLayout(m.Styles.Chrome.Pane, m.Width, m.focus > vaultsPane, true)
	x := ui.MillerSideMargin(cols, m.Width)

	if m.focus > vaultsPane && pw[m.focus-1] > 0 {
		panes = append(panes, ui.VisiblePane{Index: m.focus - 1, X: x, Width: pw[m.focus-1]})
		x += pw[m.focus-1]
	}

	panes = append(panes, ui.VisiblePane{Index: m.focus, X: x, Width: pw[m.focus]})
	x += pw[m.focus]

	childIdx := m.focus + 1
	if childIdx <= versionsPane && pw[childIdx] > 0 {
		panes = append(panes, ui.VisiblePane{Index: childIdx, X: x, Width: pw[childIdx]})
	}

	return panes
}

// listForPane returns the ui.List pointer for the given pane index, or nil.
func (m *Model) listForPane(pane int) *ui.List {
	switch pane {
	case vaultsPane:
		return &m.vaultsList
	case kindPane:
		return &m.kindList
	case secretsPane:
		return &m.secretsList
	case versionsPane:
		return &m.versionsList
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
	// +1 for the horizontal rule between the header and the columns;
	// without it, mouse-click rows map one line below where the click
	// actually landed.
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
