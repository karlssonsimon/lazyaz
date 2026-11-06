package acapp

import (
	"fmt"

	"github.com/karlssonsimon/lazyaz/internal/appshell"
	"github.com/karlssonsimon/lazyaz/internal/ui"

	tea "charm.land/bubbletea/v2"
)

func (m Model) refresh() (Model, tea.Cmd) {
	if !m.HasSubscription {
		m.SubOverlay.Open()
		m.StartLoading(-1, "Refreshing subscriptions...")
		return m, tea.Batch(m.Spinner.Tick, appshell.FetchSubscriptionsCmd(m.service, m.cache.subscriptions, m.Tenant, m.Subscriptions))
	}
	if !m.hasStore || m.focus == storesPane {
		m.StartLoading(m.focus, fmt.Sprintf("Loading App Configuration stores in %s", ui.SubscriptionDisplayName(m.CurrentSub)))
		return m, tea.Batch(m.Spinner.Tick, fetchStoresCmd(m.service, m.cache.stores, m.CurrentSub.ID, m.stores))
	}
	m.StartLoading(m.focus, fmt.Sprintf("Loading settings in %s", m.currentStore.Name))
	return m, tea.Batch(m.Spinner.Tick, fetchSettingsCmd(m.service, m.cache.settings, m.currentStore, m.settings))
}
