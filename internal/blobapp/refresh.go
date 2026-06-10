package blobapp

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

	if !m.hasAccount || m.focus == accountsPane {
		// Standalone tabs have a fixed account list — refreshing it via
		// ARM would fail. Fall through to refreshing whatever is below.
		if m.standalone {
			if !m.hasAccount {
				return m, nil
			}
		} else {
			m.StartLoading(accountsPane, fmt.Sprintf("Loading storage accounts in %s", ui.SubscriptionDisplayName(m.CurrentSub)))
			return m, tea.Batch(m.Spinner.Tick, fetchAccountsCmd(m.service, m.cache.accounts, m.CurrentSub.ID, m.accounts))
		}
	}

	if m.focus == containersPane || !m.hasContainer {
		m.StartLoading(containersPane, fmt.Sprintf("Loading containers in %s", m.currentAccount.Name))
		return m, tea.Batch(m.Spinner.Tick, fetchContainersCmd(m.service, m.cache.containers, m.currentAccount, m.containers))
	}
	if m.focus == previewPane && m.preview.open {
		return m.ensurePreviewWindowAtCursor()
	}

	if m.blobLoadAll {
		m.StartLoading(blobsPane, fmt.Sprintf("Loading all blobs in %s/%s", m.currentAccount.Name, m.containerName))
		return m, tea.Batch(m.Spinner.Tick, fetchAllBlobsCmd(m.service, m.cache.blobs, m.currentAccount, m.containerName, m.prefix, m.blobs))
	}
	// Re-run the API prefix search if a filter is active. fetching must be
	// set so handleBlobsLoaded routes the result to the filter handler
	// instead of dropping it (which left the spinner unresolved).
	if m.filter.prefixFetched && m.filter.prefixQuery != "" {
		m.filter.fetching = true
		effectivePrefix := blobSearchPrefix(m.prefix, m.filter.prefixQuery)
		m.StartLoading(blobsPane, fmt.Sprintf("Searching blobs by prefix %q...", effectivePrefix))
		return m, tea.Batch(m.Spinner.Tick, fetchSearchBlobsCmd(m.service, m.currentAccount, m.containerName, m.prefix, m.filter.prefixQuery, defaultBlobPrefixSearchLimit))
	}
	m.StartLoading(blobsPane, fmt.Sprintf("Loading up to %d entries under %q", defaultHierarchyBlobLoadLimit, displayPrefix(m.prefix)))
	return m, tea.Batch(m.Spinner.Tick, fetchHierarchyBlobsCmd(m.service, m.cache.blobs, m.currentAccount, m.containerName, m.prefix, defaultHierarchyBlobLoadLimit, m.blobs))
}
