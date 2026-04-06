package kvapp

import (
	"fmt"

	"github.com/karlssonsimon/lazyaz/internal/azure"
	"github.com/karlssonsimon/lazyaz/internal/azure/keyvault"
	"github.com/karlssonsimon/lazyaz/internal/cache"
	"github.com/karlssonsimon/lazyaz/internal/ui"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
)

func (m Model) navigateLeft() (Model, tea.Cmd) {
	switch m.focus {
	case versionsPane:
		m.focus = secretsPane
		return m, nil
	case secretsPane:
		m.focus = vaultsPane
		return m, nil
	case vaultsPane:
		return m, nil
	default:
		return m, nil
	}
}

func (m Model) handleBackspace() (Model, tea.Cmd) {
	if m.focus == versionsPane {
		m.focus = secretsPane
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
	m.focus = vaultsPane

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

	m.fetchGen++
	m.vaultsSession = cache.NewFetchSession(m.vaults, m.fetchGen, vaultKey)
	m.SetLoading(m.focus)
	m.Status = fmt.Sprintf("Loading key vaults in %s", ui.SubscriptionDisplayName(sub))
	return m, tea.Batch(spinner.Tick, fetchVaultsCmd(m.service, m.cache.vaults, sub.ID, m.fetchGen))
}

func (m Model) handleEnter() (Model, tea.Cmd) {
	if m.focus == vaultsPane {
		item, ok := m.vaultsList.SelectedItem().(vaultItem)
		if !ok {
			return m, nil
		}

		// Re-selecting the same vault: just move focus.
		if m.hasVault && m.currentVault.Name == item.vault.Name {
			m.focus = secretsPane
			return m, nil
		}

		// Snapshot the current secrets list under the outgoing vault
		// so switching back to it (via sibling navigation) restores the
		// cursor and filter.
		if m.hasVault {
			oldKey := cache.Key(m.CurrentSub.ID, m.currentVault.Name)
			m.secretsHistory[oldKey] = ui.SnapshotListState(&m.secretsList, secretItemKey)
		}

		m.currentVault = item.vault
		m.hasVault = true
		m.hasSecret = false
		m.currentSecret = keyvault.Secret{}
		m.focus = secretsPane

		secretsScope := cache.Key(m.CurrentSub.ID, item.vault.Name)
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

		m.fetchGen++
		m.secretsSession = cache.NewFetchSession(m.secrets, m.fetchGen, secretKey)
		m.SetLoading(m.focus)
		m.Status = fmt.Sprintf("Loading secrets in %s", item.vault.Name)
		return m, tea.Batch(spinner.Tick, fetchSecretsCmd(m.service, m.cache.secrets, item.vault, m.fetchGen))
	}

	if m.focus == secretsPane {
		item, ok := m.secretsList.SelectedItem().(secretItem)
		if !ok {
			return m, nil
		}

		// Re-selecting the same secret: just move focus.
		if m.hasSecret && m.currentSecret.Name == item.secret.Name {
			m.focus = versionsPane
			return m, nil
		}

		// Snapshot the current versions list under the outgoing secret.
		if m.hasSecret {
			oldKey := cache.Key(m.CurrentSub.ID, m.currentVault.Name, m.currentSecret.Name)
			m.versionsHistory[oldKey] = ui.SnapshotListState(&m.versionsList, versionItemKey)
		}

		m.currentSecret = item.secret
		m.hasSecret = true
		m.focus = versionsPane

		versionScope := cache.Key(m.CurrentSub.ID, m.currentVault.Name, item.secret.Name)
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

		m.fetchGen++
		m.versionsSession = cache.NewFetchSession(m.versions, m.fetchGen, versionKey)
		m.SetLoading(m.focus)
		m.Status = fmt.Sprintf("Loading versions for %s", item.secret.Name)
		return m, tea.Batch(spinner.Tick, fetchVersionsCmd(m.service, m.cache.versions, m.currentVault, item.secret.Name, m.fetchGen))
	}

	return m, nil
}
