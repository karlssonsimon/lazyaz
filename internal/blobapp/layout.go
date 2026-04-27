package blobapp

import (
	"github.com/karlssonsimon/lazyaz/internal/azure/blob"
	"github.com/karlssonsimon/lazyaz/internal/ui"
)

func (m *Model) resize() {
	if m.Width <= 0 || m.Height <= 0 {
		return
	}

	pane := m.Styles.Chrome.Pane

	// hasParent=false at the topmost column lets focus absorb the
	// otherwise-empty parent slot (focus gets ~80%, child stays ~20%).
	// Once drilled, the layout reserves all three slots so focus stays
	// stable at ~60% as the user moves between drilled-in columns.
	hasParent := m.focus > accountsPane
	cols := ui.MillerLayout(pane, m.Width, hasParent, true)

	m.paneWidths = [4]int{} // reset all to 0
	m.paneWidths[m.focus] = cols.Focused
	if m.focus > accountsPane {
		m.paneWidths[m.focus-1] = cols.Parent
	}
	childIdx := m.focus + 1
	if m.focus == blobsPane {
		childIdx = previewPane
	}
	if childIdx <= previewPane {
		m.paneWidths[childIdx] = cols.Child
	}

	// Height.
	height := ui.AppBodyHeight(m.Height)
	if height < 10 {
		height = 10
	}
	m.paneHeight = height

	baseListHeight := ui.MillerListBodyHeight(height, true)
	rightmostPane := m.focus
	if childIdx <= previewPane && m.paneWidths[childIdx] > 0 {
		rightmostPane = childIdx
	}
	contentWidth := func(pane int, w int) int {
		return ui.MillerContentWidth(ui.MillerColumnFrame{Width: w, RightRule: pane != rightmostPane})
	}

	// Size each visible list to its pane width.
	if w := m.paneWidths[accountsPane]; w > 0 {
		m.accountsList.SetSize(contentWidth(accountsPane, w), baseListHeight-m.inspectFooterHeight(accountsPane))
	}
	if w := m.paneWidths[containersPane]; w > 0 {
		contentWidth := contentWidth(containersPane, w)
		m.containersList.SetSize(contentWidth, baseListHeight-m.inspectFooterHeight(containersPane))
		// Also size the parent blobs list to the same width (used when
		// inside a folder — the left column shows parent folder contents).
		if m.focus == blobsPane && m.prefix != "" {
			m.parentBlobsList.SetSize(contentWidth, baseListHeight)
			if items := m.parentBlobsList.Items(); len(items) > 0 {
				if bi, ok := items[0].(blobItem); ok && bi.contentWidth != contentWidth {
					entries := make([]blob.BlobEntry, 0, len(items))
					for _, item := range items {
						if bi, ok := item.(blobItem); ok {
							entries = append(entries, bi.blob)
						}
					}
					m.parentBlobsList.SetItems(blobsToItems(entries, parentPrefix(m.prefix), contentWidth))
				}
			}
		}
	}
	if w := m.paneWidths[blobsPane]; w > 0 {
		contentWidth := contentWidth(blobsPane, w)
		blobListHeight := baseListHeight - m.inspectFooterHeight(blobsPane)
		// When a filter is active (but overlay closed), a one-line banner
		// is shown as a pane prefix — subtract its height.
		if !m.filter.inputOpen && m.hasActiveFilter() {
			blobListHeight -= 2
		}
		m.blobsList.SetSize(contentWidth, blobListHeight)
		if items := m.blobsList.Items(); len(items) > 0 {
			if bi, ok := items[0].(blobItem); ok && bi.contentWidth != contentWidth {
				m.refreshItemsWithWidth(m.displayBlobs(), contentWidth)
			}
		}
	}
	if w := m.paneWidths[previewPane]; w > 0 {
		// Shrink viewport content area by the gutter width so that
		// gutter + viewport view = the column's content width once
		// JoinHorizontal'd in view.go. The gutter is rendered outside
		// the viewport so text selection / copy doesn't include
		// line-number characters.
		colWidth := contentWidth(previewPane, w)
		gutterW := ui.LineGutterWidth(m.preview.viewport.TotalLineCount(), previewGutterMinDigits)
		vpWidth := colWidth - gutterW
		if vpWidth < 1 {
			vpWidth = 1
		}
		m.preview.viewport.SetWidth(vpWidth)
		m.preview.viewport.SetHeight(ui.MillerListBodyHeight(height, false))
	}
}
