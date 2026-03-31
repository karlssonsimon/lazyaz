package kvapp

import (
	"fmt"

	"azure-storage/internal/cache"
	"azure-storage/internal/keyvault"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
)

func (m Model) navigateLeft() (Model, tea.Cmd) {
	switch m.focus {
	case versionsPane:
		m.focus = secretsPane
		m.status = "Focus: secrets"
		return m, nil
	case secretsPane:
		m.focus = vaultsPane
		m.status = "Focus: vaults"
		return m, nil
	case vaultsPane:
		m.focus = subscriptionsPane
		m.status = "Focus: subscriptions"
		return m, nil
	default:
		return m, nil
	}
}

func (m Model) handleBackspace() (Model, tea.Cmd) {
	if m.focus == versionsPane {
		m.focus = secretsPane
		m.status = "Focus: secrets"
	}
	return m, nil
}

func (m Model) handleEnter() (Model, tea.Cmd) {
	if m.focus == subscriptionsPane {
		item, ok := m.subscriptionsList.SelectedItem().(subscriptionItem)
		if !ok {
			return m, nil
		}

		m.currentSub = item.subscription
		m.hasSubscription = true
		m.hasVault = false
		m.hasSecret = false
		m.currentVault = keyvault.Vault{}
		m.currentSecret = keyvault.Secret{}
		m.focus = vaultsPane

		if cached, ok := m.cache.vaults.Get(item.subscription.ID); ok {
			m.vaults = cached
			m.vaultsList.ResetFilter()
			m.vaultsList.SetItems(vaultsToItems(cached))
			m.vaultsList.Title = fmt.Sprintf("Vaults (%d)", len(cached))
			m.vaultsList.Select(0)
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
		m.status = fmt.Sprintf("Loading key vaults in %s", subscriptionDisplayName(item.subscription))
		return m, tea.Batch(spinner.Tick, loadVaultsCmd(m.service, item.subscription.ID))
	}

	if m.focus == vaultsPane {
		item, ok := m.vaultsList.SelectedItem().(vaultItem)
		if !ok {
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
			m.secretsList.SetItems(secretsToItems(cached))
			m.secretsList.Title = fmt.Sprintf("Secrets (%d)", len(cached))
			m.secretsList.Select(0)
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
		return m, tea.Batch(spinner.Tick, loadSecretsCmd(m.service, item.vault))
	}

	if m.focus == secretsPane {
		item, ok := m.secretsList.SelectedItem().(secretItem)
		if !ok {
			return m, nil
		}

		m.currentSecret = item.secret
		m.hasSecret = true
		m.focus = versionsPane

		versionKey := cache.Key(m.currentSub.ID, m.currentVault.Name, item.secret.Name)
		if cached, ok := m.cache.versions.Get(versionKey); ok {
			m.versions = cached
			m.versionsList.ResetFilter()
			m.versionsList.SetItems(versionsToItems(cached))
			m.versionsList.Title = fmt.Sprintf("Versions (%d)", len(cached))
			m.versionsList.Select(0)
		} else {
			m.versions = nil
			m.versionsList.ResetFilter()
			m.versionsList.SetItems(nil)
			m.versionsList.Title = "Versions"
		}

		m.loading = true
		m.status = fmt.Sprintf("Loading versions for %s", item.secret.Name)
		return m, tea.Batch(spinner.Tick, loadVersionsCmd(m.service, m.currentVault, item.secret.Name))
	}

	return m, nil
}
