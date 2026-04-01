package kvapp

import (
	"fmt"
	"strings"

	"azure-storage/internal/azure"
	"azure-storage/internal/azure/keyvault"
	"azure-storage/internal/ui"

	"github.com/charmbracelet/bubbles/list"
)

type subscriptionItem struct {
	subscription azure.Subscription
}

func (i subscriptionItem) Title() string {
	if strings.TrimSpace(i.subscription.Name) != "" {
		return i.subscription.Name
	}
	return i.subscription.ID
}

func (i subscriptionItem) Description() string {
	id := i.subscription.ID
	if len(id) > 12 {
		id = id[:12]
	}
	state := strings.TrimSpace(i.subscription.State)
	if state == "" {
		return fmt.Sprintf("id %s", id)
	}
	return fmt.Sprintf("%s | id %s", state, id)
}

func (i subscriptionItem) FilterValue() string {
	return i.subscription.Name + " " + i.subscription.ID + " " + i.subscription.State
}

type vaultItem struct {
	vault keyvault.Vault
}

func (i vaultItem) Title() string {
	return i.vault.Name
}

func (i vaultItem) Description() string {
	shortSub := i.vault.SubscriptionID
	if len(shortSub) > 8 {
		shortSub = shortSub[:8]
	}
	if i.vault.ResourceGroup == "" {
		return fmt.Sprintf("sub %s", shortSub)
	}
	return fmt.Sprintf("sub %s | rg %s", shortSub, i.vault.ResourceGroup)
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
	enabled := "no"
	if i.secret.Enabled {
		enabled = "yes"
	}
	ct := ui.EmptyToDash(i.secret.ContentType)
	return fmt.Sprintf("%s | updated %s | enabled: %s", ct, ui.FormatTime(i.secret.UpdatedOn), enabled)
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
	enabled := "no"
	if i.version.Enabled {
		enabled = "yes"
	}
	expires := ui.FormatTime(i.version.ExpiresOn)
	return fmt.Sprintf("created %s | enabled: %s | expires: %s", ui.FormatTime(i.version.CreatedOn), enabled, expires)
}

func (i versionItem) FilterValue() string {
	return i.version.Version
}

func subscriptionsToItems(subs []azure.Subscription) []list.Item {
	items := make([]list.Item, 0, len(subs))
	for _, s := range subs {
		items = append(items, subscriptionItem{subscription: s})
	}
	return items
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
