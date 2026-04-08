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

// Identity functions used by cache.FetchSession and
// ui.SetItemsPreserveKey. Names are unique within a scope (subscription,
// vault, secret), which is all the merge semantics require.

func vaultKey(v keyvault.Vault) string           { return v.Name }
func secretKey(s keyvault.Secret) string         { return s.Name }
func versionKey(v keyvault.SecretVersion) string { return v.Version }

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
	if vi, ok := it.(versionItem); ok {
		return vi.version.Version
	}
	return ""
}
