package kvapp

import (
	"azure-storage/internal/ui"
)

func (m *Model) inspectFocusedItem() {
	switch m.focus {
	case vaultsPane:
		item, ok := m.vaultsList.SelectedItem().(vaultItem)
		if !ok {
			return
		}
		v := item.vault
		m.inspectTitle = "Key Vault"
		m.inspectFields = []ui.InspectField{
			{Label: "Name", Value: v.Name},
			{Label: "Subscription", Value: v.SubscriptionID},
			{Label: "Resource Group", Value: v.ResourceGroup},
			{Label: "Vault URI", Value: v.VaultURI},
		}
	case secretsPane:
		item, ok := m.secretsList.SelectedItem().(secretItem)
		if !ok {
			return
		}
		s := item.secret
		enabled := "Yes"
		if !s.Enabled {
			enabled = "No"
		}
		m.inspectTitle = "Secret"
		m.inspectFields = []ui.InspectField{
			{Label: "Name", Value: s.Name},
			{Label: "Content Type", Value: ui.EmptyToDash(s.ContentType)},
			{Label: "Enabled", Value: enabled},
			{Label: "Created", Value: ui.FormatTime(s.CreatedOn)},
			{Label: "Updated", Value: ui.FormatTime(s.UpdatedOn)},
		}
	case versionsPane:
		item, ok := m.versionsList.SelectedItem().(versionItem)
		if !ok {
			return
		}
		v := item.version
		enabled := "Yes"
		if !v.Enabled {
			enabled = "No"
		}
		m.inspectTitle = "Secret Version"
		m.inspectFields = []ui.InspectField{
			{Label: "Version", Value: v.Version},
			{Label: "Content Type", Value: ui.EmptyToDash(v.ContentType)},
			{Label: "Enabled", Value: enabled},
			{Label: "Created", Value: ui.FormatTime(v.CreatedOn)},
			{Label: "Updated", Value: ui.FormatTime(v.UpdatedOn)},
			{Label: "Expires", Value: ui.FormatTime(v.ExpiresOn)},
		}
	}
}
