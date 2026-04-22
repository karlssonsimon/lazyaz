package kvapp

import (
	"fmt"

	"github.com/karlssonsimon/lazyaz/internal/azure"
	"github.com/karlssonsimon/lazyaz/internal/azure/keyvault"
	"github.com/karlssonsimon/lazyaz/internal/cache"
	"github.com/karlssonsimon/lazyaz/internal/ui"

	tea "charm.land/bubbletea/v2"
)

func (m Model) navigateLeft() (Model, tea.Cmd) {
	switch m.focus {
	case versionsPane:
		m.transitionTo(secretsPane)
		return m, appendJumpRecord(m, nil)
	case secretsPane:
		m.transitionTo(vaultsPane)
		return m, appendJumpRecord(m, nil)
	case vaultsPane:
		return m, nil
	default:
		return m, nil
	}
}

func (m Model) handleBackspace() (Model, tea.Cmd) {
	if m.focus > vaultsPane {
		return m.navigateLeft()
	}
	return m, nil
}

func (m Model) selectSubscription(sub azure.Subscription) (Model, tea.Cmd) {
	// Re-selecting the same subscription: no-op.
	if m.HasSubscription && m.CurrentSub.ID == sub.ID {
		return m, nil
	}

	// Snapshot the current vaults list state under the outgoing sub so
	// navigating back to it later restores the cursor and filter.
	if m.HasSubscription {
		m.vaultsHistory[m.CurrentSub.ID] = ui.SnapshotListState(&m.vaultsList, vaultItemKey)
	}

	m.CurrentSub = sub
	m.HasSubscription = true
	m.hasVault = false
	m.hasSecret = false
	m.currentVault = keyvault.Vault{}
	m.currentSecret = keyvault.Secret{}
	m.clearSecretSelectionState()
	m.transitionTo(vaultsPane)

	if cached, ok := m.cache.vaults.Get(sub.ID); ok {
		m.vaults = cached
		m.vaultsList.SetItems(vaultsToItems(cached))
		m.vaultsList.Title = fmt.Sprintf("Vaults (%d)", len(cached))
	} else {
		m.vaults = nil
		m.vaultsList.SetItems(nil)
		m.vaultsList.Title = "Vaults"
	}
	ui.RestoreListState(&m.vaultsList, m.vaultsHistory[sub.ID], vaultItemKey)

	m.secrets = nil
	m.versions = nil
	m.secretsList.ResetFilter()
	m.versionsList.ResetFilter()
	m.secretsList.SetItems(nil)
	m.versionsList.SetItems(nil)
	m.secretsList.Title = "Secrets"
	m.versionsList.Title = "Versions"

	m.startLoading(m.focus, fmt.Sprintf("Loading key vaults in %s", ui.SubscriptionDisplayName(sub)))
	return m, tea.Batch(m.Spinner.Tick, fetchVaultsCmd(m.service, m.cache.vaults, sub.ID, m.vaults))
}

func (m Model) handleEnter() (Model, tea.Cmd) {
	if m.focus == vaultsPane {
		item, ok := m.vaultsList.SelectedItem().(vaultItem)
		if !ok {
			return m, nil
		}
		return m.selectVault(item.vault)
	}
	if m.focus == secretsPane {
		item, ok := m.secretsList.SelectedItem().(secretItem)
		if !ok {
			return m, nil
		}
		return m.selectSecret(item.secret)
	}
	return m, nil
}

// selectVault binds the explorer to a vault and loads secrets.
func (m Model) selectVault(vault keyvault.Vault) (Model, tea.Cmd) {
	// Re-selecting the same vault: just move focus.
	if m.hasVault && m.currentVault.Name == vault.Name {
		m.transitionTo(secretsPane)
		return m, appendJumpRecord(m, nil)
	}

	// Snapshot the current secrets list under the outgoing vault
	// so switching back to it (via sibling navigation) restores the
	// cursor and filter.
	if m.hasVault {
		oldKey := cache.Key(m.CurrentSub.ID, m.currentVault.Name)
		m.secretsHistory[oldKey] = ui.SnapshotListState(&m.secretsList, secretItemKey)
	}

	m.currentVault = vault
	m.hasVault = true
	m.hasSecret = false
	m.currentSecret = keyvault.Secret{}
	m.clearSecretSelectionState()
	m.transitionTo(secretsPane)
	ui.SelectByKey(&m.vaultsList, vault.Name, vaultItemKey)

	secretsScope := cache.Key(m.CurrentSub.ID, vault.Name)
	if cached, ok := m.cache.secrets.Get(secretsScope); ok {
		m.secrets = cached
		m.secretsList.SetItems(secretsToItems(cached))
		m.secretsList.Title = fmt.Sprintf("Secrets (%d)", len(cached))
	} else {
		m.secrets = nil
		m.secretsList.SetItems(nil)
		m.secretsList.Title = "Secrets"
	}
	ui.RestoreListState(&m.secretsList, m.secretsHistory[secretsScope], secretItemKey)

	m.versions = nil
	m.versionsList.ResetFilter()
	m.versionsList.SetItems(nil)
	m.versionsList.Title = "Versions"

	m.startLoading(m.focus, fmt.Sprintf("Loading secrets in %s", vault.Name))
	return m, appendJumpRecord(m, tea.Batch(m.Spinner.Tick, fetchSecretsCmd(m.service, m.cache.secrets, vault, m.secrets)))
}

// selectSecret binds the explorer to a secret under the active vault
// and loads versions.
func (m Model) selectSecret(secret keyvault.Secret) (Model, tea.Cmd) {
	// Re-selecting the same secret: just move focus.
	if m.hasSecret && m.currentSecret.Name == secret.Name {
		m.transitionTo(versionsPane)
		return m, appendJumpRecord(m, nil)
	}

	// Snapshot the current versions list under the outgoing secret.
	if m.hasSecret {
		oldKey := cache.Key(m.CurrentSub.ID, m.currentVault.Name, m.currentSecret.Name)
		m.versionsHistory[oldKey] = ui.SnapshotListState(&m.versionsList, versionItemKey)
	}

	m.currentSecret = secret
	m.hasSecret = true
	m.transitionTo(versionsPane)
	ui.SelectByKey(&m.secretsList, secret.Name, secretItemKey)

	versionScope := cache.Key(m.CurrentSub.ID, m.currentVault.Name, secret.Name)
	if cached, ok := m.cache.versions.Get(versionScope); ok {
		m.versions = cached
		m.versionsList.SetItems(versionsToItems(cached))
		m.versionsList.Title = fmt.Sprintf("Versions (%d)", len(cached))
	} else {
		m.versions = nil
		m.versionsList.SetItems(nil)
		m.versionsList.Title = "Versions"
	}
	ui.RestoreListState(&m.versionsList, m.versionsHistory[versionScope], versionItemKey)

	m.startLoading(m.focus, fmt.Sprintf("Loading versions for %s", secret.Name))
	return m, appendJumpRecord(m, tea.Batch(m.Spinner.Tick, fetchVersionsCmd(m.service, m.cache.versions, m.currentVault, secret.Name, m.versions)))
}
