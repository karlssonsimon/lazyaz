package blobapp

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

	if !m.hasAccount || m.focus == accountsPane {
		m.fetchGen++
		m.accountsSession = cache.NewFetchSession(m.accounts, m.fetchGen, accountKey)
		m.SetLoading(accountsPane)
		m.LastErr = ""
		m.Status = fmt.Sprintf("Loading storage accounts in %s", ui.SubscriptionDisplayName(m.CurrentSub))
		return m, tea.Batch(m.Spinner.Tick, fetchAccountsCmd(m.service, m.cache.accounts, m.CurrentSub.ID, true, m.fetchGen))
	}

	if m.focus == containersPane || !m.hasContainer {
		m.fetchGen++
		m.containersSession = cache.NewFetchSession(m.containers, m.fetchGen, containerKey)
		m.SetLoading(containersPane)
		m.LastErr = ""
		m.Status = fmt.Sprintf("Loading containers in %s", m.currentAccount.Name)
		return m, tea.Batch(m.Spinner.Tick, fetchContainersCmd(m.service, m.cache.containers, m.currentAccount, true, m.fetchGen))
	}
	if m.focus == previewPane && m.preview.open {
		return m.ensurePreviewWindowAtCursor()
	}

	m.SetLoading(blobsPane)
	m.LastErr = ""
	// Refresh refetches m.blobs, so any committed filter snapshot would
	// be stale — drop it.
	m.discardCommittedFilter()
	if m.blobLoadAll {
		m.fetchGen++
		m.blobsSession = cache.NewFetchSession(m.blobs, m.fetchGen, blobEntryKey)
		m.Status = fmt.Sprintf("Loading all blobs in %s/%s", m.currentAccount.Name, m.containerName)
		return m, tea.Batch(m.Spinner.Tick, fetchAllBlobsCmd(m.service, m.cache.blobs, m.currentAccount, m.containerName, m.prefix, true, m.fetchGen))
	}
	if m.search.prefixLocked && m.search.prefixQuery != "" {
		// Search does not use FetchSession merge — handled separately.
		effectivePrefix := blobSearchPrefix(m.prefix, m.search.prefixQuery)
		m.Status = fmt.Sprintf("Searching blobs by prefix %q...", effectivePrefix)
		return m, tea.Batch(m.Spinner.Tick, fetchSearchBlobsCmd(m.service, m.cache.blobs, m.currentAccount, m.containerName, m.prefix, m.search.prefixQuery, defaultBlobPrefixSearchLimit, true))
	}
	m.fetchGen++
	m.blobsSession = cache.NewFetchSession(m.blobs, m.fetchGen, blobEntryKey)
	m.Status = fmt.Sprintf("Loading up to %d entries under %q", defaultHierarchyBlobLoadLimit, m.prefix)
	return m, tea.Batch(m.Spinner.Tick, fetchHierarchyBlobsCmd(m.service, m.cache.blobs, m.currentAccount, m.containerName, m.prefix, defaultHierarchyBlobLoadLimit, true, m.fetchGen))
}
