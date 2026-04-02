package kvapp

import (
	"fmt"

	"azure-storage/internal/azure"
	"azure-storage/internal/azure/keyvault"
	"azure-storage/internal/cache"
	"azure-storage/internal/ui"

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
	if m.hasSubscription && m.currentSub.ID == sub.ID {
		return m, nil
	}

	m.currentSub = sub
	m.hasSubscription = true
	m.hasVault = false
	m.hasSecret = false
	m.currentVault = keyvault.Vault{}
	m.currentSecret = keyvault.Secret{}
	m.focus = vaultsPane

	if cached, ok := m.cache.vaults.Get(sub.ID); ok {
		m.vaults = cached
		m.vaultsList.ResetFilter()
		ui.SetItemsPreserveIndex(&m.vaultsList, vaultsToItems(cached))
		m.vaultsList.Title = fmt.Sprintf("Vaults (%d)", len(cached))
	} else {
		m.vaults = nil
		m.vaultsList.ResetFilter()
		m.vaultsList.SetItems(nil)
		m.vaultsList.Title = "Vaults"
	}

	m.secrets = nil
	m.versions = nil
	m.secretsList.ResetFilter()
	m.versionsList.ResetFilter()
	m.secretsList.SetItems(nil)
	m.versionsList.SetItems(nil)
	m.secretsList.Title = "Secrets"
	m.versionsList.Title = "Versions"

	m.loading = true
	m.status = fmt.Sprintf("Loading key vaults in %s", subscriptionDisplayName(sub))
	return m, tea.Batch(spinner.Tick, fetchVaultsCmd(m.service, m.cache.vaults, sub.ID))
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

		m.currentVault = item.vault
		m.hasVault = true
		m.hasSecret = false
		m.currentSecret = keyvault.Secret{}
		m.focus = secretsPane

		if cached, ok := m.cache.secrets.Get(cache.Key(m.currentSub.ID, item.vault.Name)); ok {
			m.secrets = cached
			m.secretsList.ResetFilter()
			ui.SetItemsPreserveIndex(&m.secretsList, secretsToItems(cached))
			m.secretsList.Title = fmt.Sprintf("Secrets (%d)", len(cached))
		} else {
			m.secrets = nil
			m.secretsList.ResetFilter()
			m.secretsList.SetItems(nil)
			m.secretsList.Title = "Secrets"
		}

		m.versions = nil
		m.versionsList.ResetFilter()
		m.versionsList.SetItems(nil)
		m.versionsList.Title = "Versions"

		m.loading = true
		m.status = fmt.Sprintf("Loading secrets in %s", item.vault.Name)
		return m, tea.Batch(spinner.Tick, fetchSecretsCmd(m.service, m.cache.secrets, item.vault))
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

		m.currentSecret = item.secret
		m.hasSecret = true
		m.focus = versionsPane

		versionKey := cache.Key(m.currentSub.ID, m.currentVault.Name, item.secret.Name)
		if cached, ok := m.cache.versions.Get(versionKey); ok {
			m.versions = cached
			m.versionsList.ResetFilter()
			ui.SetItemsPreserveIndex(&m.versionsList, versionsToItems(cached))
			m.versionsList.Title = fmt.Sprintf("Versions (%d)", len(cached))
		} else {
			m.versions = nil
			m.versionsList.ResetFilter()
			m.versionsList.SetItems(nil)
			m.versionsList.Title = "Versions"
		}

		m.loading = true
		m.status = fmt.Sprintf("Loading versions for %s", item.secret.Name)
		return m, tea.Batch(spinner.Tick, fetchVersionsCmd(m.service, m.cache.versions, m.currentVault, item.secret.Name))
	}

	return m, nil
}
