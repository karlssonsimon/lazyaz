package kvapp

import (
	"github.com/karlssonsimon/lazyaz/internal/azure/keyvault"
	"github.com/karlssonsimon/lazyaz/internal/ui"
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
	case kindPane:
		// The kind row carries no inspectable Azure metadata; just label
		// the choice so the strip isn't empty.
		ki, ok := m.kindList.SelectedItem().(kindItem)
		if !ok {
			return "Kind", nil
		}
		return "Kind", []ui.InspectField{
			{Label: "Selection", Value: ki.Title()},
		}
	case secretsPane:
		switch v := m.secretsList.SelectedItem().(type) {
		case secretItem:
			return secretInspect(v.secret, m.revealedSecrets[v.secret.Name])
		case certItem:
			return certInspect(v.cert)
		case keyItem:
			return keyInspect(v.key)
		}
		return middleColumnInspectFallback(m.kvKind), nil
	case versionsPane:
		switch v := m.versionsList.SelectedItem().(type) {
		case versionItem:
			return secretVersionInspect(v.version, m.revealedVersions[revealVersionKey(m.currentSecret.Name, v.version.Version)])
		case certVersionItem:
			return certVersionInspect(v.version)
		case keyVersionItem:
			return keyVersionInspect(v.version)
		}
		return middleColumnInspectFallback(m.kvKind) + " Version", nil
	}
	return "", nil
}

func middleColumnInspectFallback(kind kvKind) string {
	switch kind {
	case kvKindCertificates:
		return "Certificate"
	case kvKindKeys:
		return "Key"
	default:
		return "Secret"
	}
}

func secretInspect(s keyvault.Secret, revealed string) (string, []ui.InspectField) {
	return "Secret", []ui.InspectField{
		{Label: "Name", Value: s.Name},
		{Label: "Value", Value: revealedValueOrMask(revealed)},
		{Label: "Content Type", Value: ui.EmptyToDash(s.ContentType)},
		{Label: "Enabled", Value: yesNo(s.Enabled)},
		{Label: "Created", Value: ui.FormatTime(s.CreatedOn)},
		{Label: "Updated", Value: ui.FormatTime(s.UpdatedOn)},
	}
}

func secretVersionInspect(v keyvault.SecretVersion, revealed string) (string, []ui.InspectField) {
	return "Secret Version", []ui.InspectField{
		{Label: "Version", Value: v.Version},
		{Label: "Value", Value: revealedValueOrMask(revealed)},
		{Label: "Content Type", Value: ui.EmptyToDash(v.ContentType)},
		{Label: "Enabled", Value: yesNo(v.Enabled)},
		{Label: "Created", Value: ui.FormatTime(v.CreatedOn)},
		{Label: "Updated", Value: ui.FormatTime(v.UpdatedOn)},
		{Label: "Expires", Value: ui.FormatTime(v.ExpiresOn)},
	}
}

func certInspect(c keyvault.Certificate) (string, []ui.InspectField) {
	return "Certificate", []ui.InspectField{
		{Label: "Name", Value: c.Name},
		{Label: "Thumbprint", Value: ui.EmptyToDash(c.Thumbprint)},
		{Label: "Enabled", Value: yesNo(c.Enabled)},
		{Label: "Created", Value: ui.FormatTime(c.CreatedOn)},
		{Label: "Updated", Value: ui.FormatTime(c.UpdatedOn)},
		{Label: "Not Before", Value: ui.FormatTime(c.NotBefore)},
		{Label: "Expires", Value: ui.FormatTime(c.Expires)},
	}
}

func certVersionInspect(v keyvault.CertificateVersion) (string, []ui.InspectField) {
	return "Certificate Version", []ui.InspectField{
		{Label: "Version", Value: v.Version},
		{Label: "Thumbprint", Value: ui.EmptyToDash(v.Thumbprint)},
		{Label: "Enabled", Value: yesNo(v.Enabled)},
		{Label: "Created", Value: ui.FormatTime(v.CreatedOn)},
		{Label: "Updated", Value: ui.FormatTime(v.UpdatedOn)},
		{Label: "Not Before", Value: ui.FormatTime(v.NotBefore)},
		{Label: "Expires", Value: ui.FormatTime(v.Expires)},
	}
}

func keyInspect(k keyvault.Key) (string, []ui.InspectField) {
	managed := "No"
	if k.Managed {
		managed = "Yes"
	}
	return "Key", []ui.InspectField{
		{Label: "Name", Value: k.Name},
		{Label: "Enabled", Value: yesNo(k.Enabled)},
		{Label: "Managed", Value: managed},
		{Label: "Created", Value: ui.FormatTime(k.CreatedOn)},
		{Label: "Updated", Value: ui.FormatTime(k.UpdatedOn)},
		{Label: "Not Before", Value: ui.FormatTime(k.NotBefore)},
		{Label: "Expires", Value: ui.FormatTime(k.Expires)},
	}
}

func keyVersionInspect(v keyvault.KeyVersion) (string, []ui.InspectField) {
	return "Key Version", []ui.InspectField{
		{Label: "Version", Value: v.Version},
		{Label: "Enabled", Value: yesNo(v.Enabled)},
		{Label: "Created", Value: ui.FormatTime(v.CreatedOn)},
		{Label: "Updated", Value: ui.FormatTime(v.UpdatedOn)},
		{Label: "Not Before", Value: ui.FormatTime(v.NotBefore)},
		{Label: "Expires", Value: ui.FormatTime(v.Expires)},
	}
}

func yesNo(enabled bool) string {
	if enabled {
		return "Yes"
	}
	return "No"
}

// revealedValueOrMask renders the secret-value cell. Empty string from
// the map (i.e. not revealed) returns a fixed-width mask plus the hint;
// any non-empty value is shown verbatim. Centralised so the mask doesn't
// drift between secret and version inspect rows.
func revealedValueOrMask(value string) string {
	if value == "" {
		return "••••••••  (R to reveal)"
	}
	return value
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

// clearReveals drops every revealed value. Called when the active
// subscription or vault changes — values from the prior context shouldn't
// follow the user across boundaries.
func (m *Model) clearReveals() {
	for k := range m.revealedSecrets {
		delete(m.revealedSecrets, k)
	}
	for k := range m.revealedVersions {
		delete(m.revealedVersions, k)
	}
}

// toggleInspect flips the inspect strip on/off for the focused pane.
func (m *Model) toggleInspect() {
	if m.inspectPanes == nil {
		m.inspectPanes = make(map[int]bool)
	}
	m.inspectPanes[m.focus] = !m.inspectPanes[m.focus]
	m.resize()
}
