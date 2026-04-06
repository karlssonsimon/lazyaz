package blobapp

import (
	"fmt"

	"azure-storage/internal/ui"
)

// inspectFor returns the inspect title and field list for the given pane,
// based on its currently selected item. Returns ("", nil) if the pane has
// no inspectable selection.
func (m Model) inspectFor(pane int) (string, []ui.InspectField) {
	switch pane {
	case accountsPane:
		item, ok := m.accountsList.SelectedItem().(accountItem)
		if !ok {
			return "Storage Account", nil
		}
		a := item.account
		return "Storage Account", []ui.InspectField{
			{Label: "Name", Value: a.Name},
			{Label: "Subscription", Value: a.SubscriptionID},
			{Label: "Resource Group", Value: a.ResourceGroup},
			{Label: "Blob Endpoint", Value: a.BlobEndpoint},
		}
	case containersPane:
		item, ok := m.containersList.SelectedItem().(containerItem)
		if !ok {
			return "Container", nil
		}
		c := item.container
		return "Container", []ui.InspectField{
			{Label: "Name", Value: c.Name},
			{Label: "Last Modified", Value: ui.FormatTime(c.LastModified)},
		}
	case blobsPane:
		item, ok := m.blobsList.SelectedItem().(blobItem)
		if !ok {
			return "Blob", nil
		}
		b := item.blob
		if b.IsPrefix {
			return "Directory", []ui.InspectField{
				{Label: "Path", Value: b.Name},
			}
		}
		return "Blob", []ui.InspectField{
			{Label: "Name", Value: b.Name},
			{Label: "Size", Value: humanSize(b.Size)},
			{Label: "Content Type", Value: ui.EmptyToDash(b.ContentType)},
			{Label: "Last Modified", Value: ui.FormatTime(b.LastModified)},
			{Label: "Access Tier", Value: ui.EmptyToDash(b.AccessTier)},
			{Label: "Metadata", Value: fmt.Sprintf("%d entries", b.MetadataCount)},
		}
	}
	return "", nil
}

// inspectFooterHeight returns the rendered row count of the inspect strip
// for the given pane (when toggled on), or 0 when off. Used by resize() to
// shrink the list height to make room for the strip.
func (m Model) inspectFooterHeight(pane int) int {
	if !m.inspectPanes[pane] {
		return 0
	}
	_, fields := m.inspectFor(pane)
	return ui.InspectStripHeight(fields)
}

// inspectFooter returns the rendered inspect strip for the pane (or "" when
// the toggle is off). Called from View() to populate ListPane.Footer.
func (m Model) inspectFooter(pane, contentWidth int) string {
	if !m.inspectPanes[pane] {
		return ""
	}
	title, fields := m.inspectFor(pane)
	return ui.RenderInspectStrip(title, fields, m.Styles, contentWidth)
}

// toggleInspect flips the inspect strip on/off for the focused pane.
func (m *Model) toggleInspect() {
	if m.inspectPanes == nil {
		m.inspectPanes = make(map[int]bool)
	}
	m.inspectPanes[m.focus] = !m.inspectPanes[m.focus]
	m.resize()
}
