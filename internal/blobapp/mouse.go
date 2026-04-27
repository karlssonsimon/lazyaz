package blobapp

import (
	"github.com/karlssonsimon/lazyaz/internal/ui"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
)

// visiblePanes returns the on-screen pane layout in left-to-right order,
// matching the render order in View(). Spacer columns are excluded.
func (m Model) visiblePanes() []ui.VisiblePane {
	pw := m.paneWidths
	var panes []ui.VisiblePane
	x := 0

	if m.focus > accountsPane && pw[m.focus-1] > 0 {
		panes = append(panes, ui.VisiblePane{Index: m.focus - 1, X: x, Width: pw[m.focus-1]})
		x += pw[m.focus-1]
	} else if m.focus == accountsPane {
		x += m.Width * 20 / 100
	}

	panes = append(panes, ui.VisiblePane{Index: m.focus, X: x, Width: pw[m.focus]})
	x += pw[m.focus]

	childIdx := m.focus + 1
	if m.focus == blobsPane {
		childIdx = previewPane
	}
	if childIdx <= previewPane && pw[childIdx] > 0 {
		panes = append(panes, ui.VisiblePane{Index: childIdx, X: x, Width: pw[childIdx]})
	}

	return panes
}

// listForPane returns the list.Model pointer for the given pane index, or nil.
func (m *Model) listForPane(pane int) *list.Model {
	switch pane {
	case accountsPane:
		return &m.accountsList
	case containersPane:
		return &m.containersList
	case blobsPane:
		return &m.blobsList
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
	// without it, mouse-click rows in the preview map one line below
	// where the click actually landed.
	y++
	return y
}

// handleMouseClick handles a left-click on a list pane: focuses the pane
// and selects the item under the cursor. On double-click, navigates into
// the selected item (same as enter). Returns (consumed, doubleClick).
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

	// Preview pane clicks are handled by text selection.
	if vp.Index == previewPane {
		return false, false
	}

	if vp.Index != m.focus {
		m.transitionTo(vp.Index, false)
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
