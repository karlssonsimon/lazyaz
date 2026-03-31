package kvapp

import (
	"fmt"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
)

func (m Model) refresh() (Model, tea.Cmd) {
	if m.focus == subscriptionsPane || !m.hasSubscription {
		m.loading = true
		m.lastErr = ""
		m.status = "Refreshing subscriptions..."
		return m, tea.Batch(spinner.Tick, loadSubscriptionsCmd(m.service))
	}

	if !m.hasVault || m.focus == vaultsPane {
		m.loading = true
		m.lastErr = ""
		m.status = fmt.Sprintf("Loading key vaults in %s", subscriptionDisplayName(m.currentSub))
		return m, tea.Batch(spinner.Tick, loadVaultsCmd(m.service, m.currentSub.ID))
	}

	if !m.hasSecret || m.focus == secretsPane {
		m.loading = true
		m.lastErr = ""
		m.status = fmt.Sprintf("Loading secrets in %s", m.currentVault.Name)
		return m, tea.Batch(spinner.Tick, loadSecretsCmd(m.service, m.currentVault))
	}

	m.loading = true
	m.lastErr = ""
	m.status = fmt.Sprintf("Loading versions for %s", m.currentSecret.Name)
	return m, tea.Batch(spinner.Tick, loadVersionsCmd(m.service, m.currentVault, m.currentSecret.Name))
}
