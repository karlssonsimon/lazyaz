package blobapp

import (
	"fmt"

	"github.com/karlssonsimon/lazyaz/internal/ui"

	tea "charm.land/bubbletea/v2"
)

func (m Model) refresh() (Model, tea.Cmd) {
	if !m.HasSubscription {
		m.SubOverlay.Open()
		m.SetLoading(-1)
	
		m.loadingSpinnerID = m.NotifySpinner("Refreshing subscriptions...")
		return m, tea.Batch(m.Spinner.Tick, fetchSubscriptionsCmd(m.service, m.cache.subscriptions, m.Subscriptions))
	}

	if !m.hasAccount || m.focus == accountsPane {
		m.SetLoading(accountsPane)
	
		m.loadingSpinnerID = m.NotifySpinner(fmt.Sprintf("Loading storage accounts in %s", ui.SubscriptionDisplayName(m.CurrentSub)))
		return m, tea.Batch(m.Spinner.Tick, fetchAccountsCmd(m.service, m.cache.accounts, m.CurrentSub.ID, m.accounts))
	}

	if m.focus == containersPane || !m.hasContainer {
		m.SetLoading(containersPane)
	
		m.loadingSpinnerID = m.NotifySpinner(fmt.Sprintf("Loading containers in %s", m.currentAccount.Name))
		return m, tea.Batch(m.Spinner.Tick, fetchContainersCmd(m.service, m.cache.containers, m.currentAccount, m.containers))
	}
	if m.focus == previewPane && m.preview.open {
		return m.ensurePreviewWindowAtCursor()
	}

	m.SetLoading(blobsPane)

	if m.blobLoadAll {
		m.loadingSpinnerID = m.NotifySpinner(fmt.Sprintf("Loading all blobs in %s/%s", m.currentAccount.Name, m.containerName))
		return m, tea.Batch(m.Spinner.Tick, fetchAllBlobsCmd(m.service, m.cache.blobs, m.currentAccount, m.containerName, m.prefix, m.blobs))
	}
	// Re-run the API prefix search if a filter is active.
	if m.filter.prefixFetched && m.filter.prefixQuery != "" {
		effectivePrefix := blobSearchPrefix(m.prefix, m.filter.prefixQuery)
		m.loadingSpinnerID = m.NotifySpinner(fmt.Sprintf("Searching blobs by prefix %q...", effectivePrefix))
		return m, tea.Batch(m.Spinner.Tick, fetchSearchBlobsCmd(m.service, m.currentAccount, m.containerName, m.prefix, m.filter.prefixQuery, defaultBlobPrefixSearchLimit))
	}
	m.loadingSpinnerID = m.NotifySpinner(fmt.Sprintf("Loading up to %d entries under %q", defaultHierarchyBlobLoadLimit, displayPrefix(m.prefix)))
	return m, tea.Batch(m.Spinner.Tick, fetchHierarchyBlobsCmd(m.service, m.cache.blobs, m.currentAccount, m.containerName, m.prefix, defaultHierarchyBlobLoadLimit, m.blobs))
}
