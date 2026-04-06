package blobapp

import (
	"fmt"

	"azure-storage/internal/ui"
)

func (m *Model) inspectFocusedItem() {
	switch m.focus {
	case accountsPane:
		item, ok := m.accountsList.SelectedItem().(accountItem)
		if !ok {
			return
		}
		a := item.account
		m.inspectTitle = "Storage Account"
		m.inspectFields = []ui.InspectField{
			{Label: "Name", Value: a.Name},
			{Label: "Subscription", Value: a.SubscriptionID},
			{Label: "Resource Group", Value: a.ResourceGroup},
			{Label: "Blob Endpoint", Value: a.BlobEndpoint},
		}
	case containersPane:
		item, ok := m.containersList.SelectedItem().(containerItem)
		if !ok {
			return
		}
		c := item.container
		m.inspectTitle = "Container"
		m.inspectFields = []ui.InspectField{
			{Label: "Name", Value: c.Name},
			{Label: "Last Modified", Value: ui.FormatTime(c.LastModified)},
		}
	case blobsPane:
		item, ok := m.blobsList.SelectedItem().(blobItem)
		if !ok {
			return
		}
		b := item.blob
		m.inspectTitle = "Blob"
		if b.IsPrefix {
			m.inspectTitle = "Directory"
			m.inspectFields = []ui.InspectField{
				{Label: "Path", Value: b.Name},
			}
			return
		}
		m.inspectFields = []ui.InspectField{
			{Label: "Name", Value: b.Name},
			{Label: "Size", Value: humanSize(b.Size)},
			{Label: "Content Type", Value: ui.EmptyToDash(b.ContentType)},
			{Label: "Last Modified", Value: ui.FormatTime(b.LastModified)},
			{Label: "Access Tier", Value: ui.EmptyToDash(b.AccessTier)},
			{Label: "Metadata", Value: fmt.Sprintf("%d entries", b.MetadataCount)},
		}
	}
}
