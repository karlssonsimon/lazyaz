package kvapp

import (
	"fmt"

	"github.com/karlssonsimon/lazyaz/internal/ui"

	tea "charm.land/bubbletea/v2"
)

func (m Model) refresh() (Model, tea.Cmd) {
	if !m.HasSubscription {
		// Can't refresh anything without a subscription; open the picker instead.
		m.SubOverlay.Open()
		m.startLoading(-1, "Refreshing subscriptions...")
		return m, tea.Batch(m.Spinner.Tick, fetchSubscriptionsCmd(m.service, m.cache.subscriptions, m.Tenant, m.Subscriptions))
	}

	if !m.hasVault || m.focus == vaultsPane {
		m.startLoading(m.focus, fmt.Sprintf("Loading key vaults in %s", ui.SubscriptionDisplayName(m.CurrentSub)))
		return m, tea.Batch(m.Spinner.Tick, fetchVaultsCmd(m.service, m.cache.vaults, m.CurrentSub.ID, m.vaults))
	}

	// Middle column refresh — kind-aware. The "selected child" we drill
	// into for the right column depends on which kind is active.
	if m.focus == secretsPane || (!m.hasMiddleSelection() && m.focus == versionsPane) {
		return m.fetchMiddleColumn()
	}

	return m.fetchVersionsForCurrent()
}

// hasMiddleSelection reports whether the kind-specific "current item"
// for the middle column is set, so versions fetches make sense.
func (m Model) hasMiddleSelection() bool {
	switch m.kvKind {
	case kvKindCertificates:
		return m.hasCert
	case kvKindKeys:
		return m.hasKey
	default:
		return m.hasSecret
	}
}

// fetchMiddleColumn dispatches the right per-kind list fetch for the
// active vault. Status messages match the kind so the user sees what's
// loading.
func (m Model) fetchMiddleColumn() (Model, tea.Cmd) {
	switch m.kvKind {
	case kvKindCertificates:
		m.startLoading(m.focus, fmt.Sprintf("Loading certificates in %s", m.currentVault.Name))
		return m, tea.Batch(m.Spinner.Tick, fetchCertsCmd(m.service, m.cache.certs, m.currentVault, m.certs))
	case kvKindKeys:
		m.startLoading(m.focus, fmt.Sprintf("Loading keys in %s", m.currentVault.Name))
		return m, tea.Batch(m.Spinner.Tick, fetchKeysCmd(m.service, m.cache.keys, m.currentVault, m.keys))
	default:
		m.startLoading(m.focus, fmt.Sprintf("Loading secrets in %s", m.currentVault.Name))
		return m, tea.Batch(m.Spinner.Tick, fetchSecretsCmd(m.service, m.cache.secrets, m.currentVault, m.secrets))
	}
}

// fetchVersionsForCurrent dispatches a per-kind versions fetch using the
// currently-selected middle-column item.
func (m Model) fetchVersionsForCurrent() (Model, tea.Cmd) {
	switch m.kvKind {
	case kvKindCertificates:
		m.startLoading(m.focus, fmt.Sprintf("Loading versions for %s", m.currentCert.Name))
		return m, tea.Batch(m.Spinner.Tick, fetchCertVersionsCmd(m.service, m.cache.certVersions, m.currentVault, m.currentCert.Name, m.certVersions))
	case kvKindKeys:
		m.startLoading(m.focus, fmt.Sprintf("Loading versions for %s", m.currentKey.Name))
		return m, tea.Batch(m.Spinner.Tick, fetchKeyVersionsCmd(m.service, m.cache.keyVersions, m.currentVault, m.currentKey.Name, m.keyVersions))
	default:
		m.startLoading(m.focus, fmt.Sprintf("Loading versions for %s", m.currentSecret.Name))
		return m, tea.Batch(m.Spinner.Tick, fetchVersionsCmd(m.service, m.cache.versions, m.currentVault, m.currentSecret.Name, m.versions))
	}
}
