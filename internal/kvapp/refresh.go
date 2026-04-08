package kvapp

import (
	"fmt"

	"github.com/karlssonsimon/lazyaz/internal/cache"
	"github.com/karlssonsimon/lazyaz/internal/ui"

	tea "charm.land/bubbletea/v2"
)

func (m Model) refresh() (Model, tea.Cmd) {
	if !m.HasSubscription {
		// Can't refresh anything without a subscription; open the picker instead.
		m.SubOverlay.Open()
		m.SetLoading(-1)
		m.LastErr = ""
		m.Status = "Refreshing subscriptions..."
		return m, tea.Batch(m.Spinner.Tick, fetchSubscriptionsCmd(m.service, m.cache.subscriptions, true))
	}

	if !m.hasVault || m.focus == vaultsPane {
		m.fetchGen++
		m.vaultsSession = cache.NewFetchSession(m.vaults, m.fetchGen, vaultKey)
		m.SetLoading(m.focus)
		m.LastErr = ""
		m.Status = fmt.Sprintf("Loading key vaults in %s", ui.SubscriptionDisplayName(m.CurrentSub))
		return m, tea.Batch(m.Spinner.Tick, fetchVaultsCmd(m.service, m.cache.vaults, m.CurrentSub.ID, m.fetchGen))
	}

	if !m.hasSecret || m.focus == secretsPane {
		m.fetchGen++
		m.secretsSession = cache.NewFetchSession(m.secrets, m.fetchGen, secretKey)
		m.SetLoading(m.focus)
		m.LastErr = ""
		m.Status = fmt.Sprintf("Loading secrets in %s", m.currentVault.Name)
		return m, tea.Batch(m.Spinner.Tick, fetchSecretsCmd(m.service, m.cache.secrets, m.currentVault, m.fetchGen))
	}

	m.fetchGen++
	m.versionsSession = cache.NewFetchSession(m.versions, m.fetchGen, versionKey)
	m.SetLoading(m.focus)
	m.LastErr = ""
	m.Status = fmt.Sprintf("Loading versions for %s", m.currentSecret.Name)
	return m, tea.Batch(m.Spinner.Tick, fetchVersionsCmd(m.service, m.cache.versions, m.currentVault, m.currentSecret.Name, m.fetchGen))
}
