package kvapp

import (
	"azure-storage/internal/azure/keyvault"

	"github.com/charmbracelet/bubbles/list"
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
