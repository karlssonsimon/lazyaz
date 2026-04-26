package blobapp

import (
	"github.com/karlssonsimon/lazyaz/internal/ui"
)

func (m *Model) resize() {
	if m.Width <= 0 || m.Height <= 0 {
		return
	}

	pane := m.Styles.Chrome.Pane

	// Determine parent/child visibility based on focus.
	// Always reserve parent space — when accounts is focused, a spacer
	// fills the left column to keep the focused pane centered.
	hasParent := true
	hasChild := false
	switch m.focus {
	case accountsPane:
		hasChild = m.hasAccount // show containers preview
	case containersPane:
		hasChild = m.hasContainer // show blobs preview
	case blobsPane:
		hasChild = m.preview.open // show blob content preview
	}

	cols := ui.MillerLayout(pane, m.Width, hasParent, hasChild)

	// Map roles → pane indices. The focused pane is always center.
	// Parent is the pane before focus, child is the pane after.
	m.paneWidths = [4]int{} // reset all to 0
	m.paneWidths[m.focus] = cols.Focused
	if m.focus > accountsPane {
		m.paneWidths[m.focus-1] = cols.Parent
	}
	if hasChild {
		childIdx := m.focus + 1
		if m.focus == blobsPane {
			childIdx = previewPane
		}
		m.paneWidths[childIdx] = cols.Child
	}

	// Height.
	height := m.Height - ui.StatusBarHeight - ui.SubscriptionBarHeight
	if height < 10 {
		height = 10
	}
	m.paneHeight = height

	baseListHeight := ui.PaneListBodyHeight(pane, height, ui.PaneListChrome{Title: true, Hints: true})

	// Size each visible list to its pane width.
	if w := m.paneWidths[accountsPane]; w > 0 {
		m.accountsList.SetSize(ui.PaneContentWidth(pane, w), baseListHeight-m.inspectFooterHeight(accountsPane))
	}
	if w := m.paneWidths[containersPane]; w > 0 {
		m.containersList.SetSize(ui.PaneContentWidth(pane, w), baseListHeight-m.inspectFooterHeight(containersPane))
		// Also size the parent blobs list to the same width (used when
		// inside a folder — the left column shows parent folder contents).
		if m.focus == blobsPane && m.prefix != "" {
			m.parentBlobsList.SetSize(ui.PaneContentWidth(pane, w), baseListHeight)
		}
	}
	if w := m.paneWidths[blobsPane]; w > 0 {
		blobListHeight := baseListHeight - m.inspectFooterHeight(blobsPane)
		// When a filter is active (but overlay closed), a one-line banner
		// is shown as a pane prefix — subtract its height.
		if !m.filter.inputOpen && m.hasActiveFilter() {
			blobListHeight -= 2
		}
		m.blobsList.SetSize(ui.PaneContentWidth(pane, w), blobListHeight)
	}
	if w := m.paneWidths[previewPane]; w > 0 {
		m.preview.viewport.SetWidth(ui.PaneContentWidth(pane, w))
		m.preview.viewport.SetHeight(baseListHeight)
	}
}
