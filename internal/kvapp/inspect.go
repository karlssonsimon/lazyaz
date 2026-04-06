package kvapp

import (
	"azure-storage/internal/ui"
)

// inspectFor returns the inspect title and field list for the given pane,
// based on its currently selected item. Returns ("", nil) if the pane has
// no inspectable selection.
func (m Model) inspectFor(pane int) (string, []ui.InspectField) {
	switch pane {
	case vaultsPane:
		item, ok := m.vaultsList.SelectedItem().(vaultItem)
		if !ok {
			return "Key Vault", nil
		}
		v := item.vault
		return "Key Vault", []ui.InspectField{
			{Label: "Name", Value: v.Name},
			{Label: "Subscription", Value: v.SubscriptionID},
			{Label: "Resource Group", Value: v.ResourceGroup},
			{Label: "Vault URI", Value: v.VaultURI},
		}
	case secretsPane:
		item, ok := m.secretsList.SelectedItem().(secretItem)
		if !ok {
			return "Secret", nil
		}
		s := item.secret
		enabled := "Yes"
		if !s.Enabled {
			enabled = "No"
		}
		return "Secret", []ui.InspectField{
			{Label: "Name", Value: s.Name},
			{Label: "Content Type", Value: ui.EmptyToDash(s.ContentType)},
			{Label: "Enabled", Value: enabled},
			{Label: "Created", Value: ui.FormatTime(s.CreatedOn)},
			{Label: "Updated", Value: ui.FormatTime(s.UpdatedOn)},
		}
	case versionsPane:
		item, ok := m.versionsList.SelectedItem().(versionItem)
		if !ok {
			return "Secret Version", nil
		}
		v := item.version
		enabled := "Yes"
		if !v.Enabled {
			enabled = "No"
		}
		return "Secret Version", []ui.InspectField{
			{Label: "Version", Value: v.Version},
			{Label: "Content Type", Value: ui.EmptyToDash(v.ContentType)},
			{Label: "Enabled", Value: enabled},
			{Label: "Created", Value: ui.FormatTime(v.CreatedOn)},
			{Label: "Updated", Value: ui.FormatTime(v.UpdatedOn)},
			{Label: "Expires", Value: ui.FormatTime(v.ExpiresOn)},
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
