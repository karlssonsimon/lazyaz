package kvapp

import (
	"fmt"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
)

func (m Model) refresh() (Model, tea.Cmd) {
	if !m.hasSubscription {
		// Can't refresh anything without a subscription; open the picker instead.
		m.subOverlay.Open()
		m.setLoading(-1)
		m.lastErr = ""
		m.status = "Refreshing subscriptions..."
		return m, tea.Batch(spinner.Tick, fetchSubscriptionsCmd(m.service, m.cache.subscriptions, true))
	}

	if !m.hasVault || m.focus == vaultsPane {
		m.setLoading(m.focus)
		m.lastErr = ""
		m.status = fmt.Sprintf("Loading key vaults in %s", subscriptionDisplayName(m.currentSub))
		return m, tea.Batch(spinner.Tick, fetchVaultsCmd(m.service, m.cache.vaults, m.currentSub.ID))
	}

	if !m.hasSecret || m.focus == secretsPane {
		m.setLoading(m.focus)
		m.lastErr = ""
		m.status = fmt.Sprintf("Loading secrets in %s", m.currentVault.Name)
		return m, tea.Batch(spinner.Tick, fetchSecretsCmd(m.service, m.cache.secrets, m.currentVault))
	}

	m.setLoading(m.focus)
	m.lastErr = ""
	m.status = fmt.Sprintf("Loading versions for %s", m.currentSecret.Name)
	return m, tea.Batch(spinner.Tick, fetchVersionsCmd(m.service, m.cache.versions, m.currentVault, m.currentSecret.Name))
}
