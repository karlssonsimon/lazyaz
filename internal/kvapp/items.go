package kvapp

import (
	"github.com/karlssonsimon/lazyaz/internal/azure/keyvault"

	"charm.land/bubbles/v2/list"
)

type vaultItem struct {
	vault keyvault.Vault
}

func (i vaultItem) Title() string {
	return i.vault.Name
}

func (i vaultItem) Description() string {
	return ""
}

func (i vaultItem) FilterValue() string {
	return i.vault.Name + " " + i.vault.SubscriptionID + " " + i.vault.ResourceGroup
}

// kindItem is a row in the kindPane list. Three of them ever exist
// (Secrets / Certificates / Keys); selecting one drives what the items
// column shows. They have no underlying Azure entity — purely UI.
type kindItem struct {
	kind kvKind
}

func (i kindItem) Title() string {
	switch i.kind {
	case kvKindCertificates:
		return "Certificates"
	case kvKindKeys:
		return "Keys"
	default:
		return "Secrets"
	}
}
func (i kindItem) Description() string { return "" }
func (i kindItem) FilterValue() string { return i.Title() }

func kindItems() []list.Item {
	return []list.Item{
		kindItem{kind: kvKindSecrets},
		kindItem{kind: kvKindCertificates},
		kindItem{kind: kvKindKeys},
	}
}

func kindItemKey(it list.Item) string {
	if ki, ok := it.(kindItem); ok {
		return ki.kind.String()
	}
	return ""
}

type secretItem struct {
	secret keyvault.Secret
}

func (i secretItem) Title() string {
	return i.secret.Name
}

func (i secretItem) Description() string {
	if !i.secret.Enabled {
		return "disabled"
	}
	return ""
}

func (i secretItem) FilterValue() string {
	return i.secret.Name + " " + i.secret.ContentType
}

type versionItem struct {
	version keyvault.SecretVersion
}

func (i versionItem) Title() string {
	v := i.version.Version
	if len(v) > 12 {
		v = v[:12]
	}
	return v
}

func (i versionItem) Description() string {
	if !i.version.Enabled {
		return "disabled"
	}
	return ""
}

func (i versionItem) FilterValue() string {
	return i.version.Version
}

type certItem struct {
	cert keyvault.Certificate
}

func (i certItem) Title() string { return i.cert.Name }
func (i certItem) Description() string {
	if !i.cert.Enabled {
		return "disabled"
	}
	return ""
}
func (i certItem) FilterValue() string { return i.cert.Name + " " + i.cert.Thumbprint }

type certVersionItem struct {
	version keyvault.CertificateVersion
}

func (i certVersionItem) Title() string {
	v := i.version.Version
	if len(v) > 12 {
		v = v[:12]
	}
	return v
}
func (i certVersionItem) Description() string {
	if !i.version.Enabled {
		return "disabled"
	}
	return ""
}
func (i certVersionItem) FilterValue() string { return i.version.Version }

type keyItem struct {
	key keyvault.Key
}

func (i keyItem) Title() string { return i.key.Name }
func (i keyItem) Description() string {
	switch {
	case !i.key.Enabled:
		return "disabled"
	case i.key.Managed:
		return "managed"
	}
	return ""
}
func (i keyItem) FilterValue() string { return i.key.Name }

type keyVersionItem struct {
	version keyvault.KeyVersion
}

func (i keyVersionItem) Title() string {
	v := i.version.Version
	if len(v) > 12 {
		v = v[:12]
	}
	return v
}
func (i keyVersionItem) Description() string {
	if !i.version.Enabled {
		return "disabled"
	}
	return ""
}
func (i keyVersionItem) FilterValue() string { return i.version.Version }

func vaultsToItems(vaults []keyvault.Vault) []list.Item {
	items := make([]list.Item, 0, len(vaults))
	for _, v := range vaults {
		items = append(items, vaultItem{vault: v})
	}
	return items
}

func secretsToItems(secrets []keyvault.Secret) []list.Item {
	items := make([]list.Item, 0, len(secrets))
	for _, s := range secrets {
		items = append(items, secretItem{secret: s})
	}
	return items
}

func versionsToItems(versions []keyvault.SecretVersion) []list.Item {
	items := make([]list.Item, 0, len(versions))
	for _, v := range versions {
		items = append(items, versionItem{version: v})
	}
	return items
}

func certsToItems(certs []keyvault.Certificate) []list.Item {
	items := make([]list.Item, 0, len(certs))
	for _, c := range certs {
		items = append(items, certItem{cert: c})
	}
	return items
}

func certVersionsToItems(versions []keyvault.CertificateVersion) []list.Item {
	items := make([]list.Item, 0, len(versions))
	for _, v := range versions {
		items = append(items, certVersionItem{version: v})
	}
	return items
}

func keysToItems(keys []keyvault.Key) []list.Item {
	items := make([]list.Item, 0, len(keys))
	for _, k := range keys {
		items = append(items, keyItem{key: k})
	}
	return items
}

func keyVersionsToItems(versions []keyvault.KeyVersion) []list.Item {
	items := make([]list.Item, 0, len(versions))
	for _, v := range versions {
		items = append(items, keyVersionItem{version: v})
	}
	return items
}

// Identity functions used by cache.Broker's internal merge and
// ui.SetItemsPreserveKey. Names are unique within a scope (subscription,
// vault, secret), which is all the merge semantics require.

func vaultKey(v keyvault.Vault) string                          { return v.Name }
func secretKey(s keyvault.Secret) string                        { return s.Name }
func versionKey(v keyvault.SecretVersion) string                { return v.Version }
func certKey(c keyvault.Certificate) string                     { return c.Name }
func certVersionKey(v keyvault.CertificateVersion) string       { return v.Version }
func kvKeyKey(k keyvault.Key) string                            { return k.Name }
func keyVersionKey(v keyvault.KeyVersion) string                { return v.Version }

func vaultItemKey(it list.Item) string {
	if vi, ok := it.(vaultItem); ok {
		return vi.vault.Name
	}
	return ""
}

func secretItemKey(it list.Item) string {
	if si, ok := it.(secretItem); ok {
		return si.secret.Name
	}
	return ""
}

func versionItemKey(it list.Item) string {
	switch v := it.(type) {
	case versionItem:
		return v.version.Version
	case certVersionItem:
		return v.version.Version
	case keyVersionItem:
		return v.version.Version
	}
	return ""
}

func certItemKey(it list.Item) string {
	if ci, ok := it.(certItem); ok {
		return ci.cert.Name
	}
	return ""
}

func keyItemKey(it list.Item) string {
	if ki, ok := it.(keyItem); ok {
		return ki.key.Name
	}
	return ""
}
