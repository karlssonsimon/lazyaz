package blobapp

import (
	"fmt"

	"azure-storage/internal/ui"

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

	if !m.hasAccount || m.focus == accountsPane {
		m.setLoading(accountsPane)
		m.lastErr = ""
		m.status = fmt.Sprintf("Loading storage accounts in %s", ui.SubscriptionDisplayName(m.currentSub))
		return m, tea.Batch(spinner.Tick, fetchAccountsCmd(m.service, m.cache.accounts, m.currentSub.ID, true))
	}

	if m.focus == containersPane || !m.hasContainer {
		m.setLoading(containersPane)
		m.lastErr = ""
		m.status = fmt.Sprintf("Loading containers in %s", m.currentAccount.Name)
		return m, tea.Batch(spinner.Tick, fetchContainersCmd(m.service, m.cache.containers, m.currentAccount, true))
	}
	if m.focus == previewPane && m.preview.open {
		return m.ensurePreviewWindowAtCursor()
	}

	m.setLoading(blobsPane)
	m.lastErr = ""
	if m.blobLoadAll {
		m.status = fmt.Sprintf("Loading all blobs in %s/%s", m.currentAccount.Name, m.containerName)
		return m, tea.Batch(spinner.Tick, fetchAllBlobsCmd(m.service, m.cache.blobs, m.currentAccount, m.containerName, m.prefix, true))
	}
	if m.search.prefixLocked && m.search.prefixQuery != "" {
		effectivePrefix := blobSearchPrefix(m.prefix, m.search.prefixQuery)
		m.status = fmt.Sprintf("Searching blobs by prefix %q...", effectivePrefix)
		return m, tea.Batch(spinner.Tick, fetchSearchBlobsCmd(m.service, m.cache.blobs, m.currentAccount, m.containerName, m.prefix, m.search.prefixQuery, defaultBlobPrefixSearchLimit, true))
	}
	m.status = fmt.Sprintf("Loading up to %d entries under %q", defaultHierarchyBlobLoadLimit, m.prefix)
	return m, tea.Batch(spinner.Tick, fetchHierarchyBlobsCmd(m.service, m.cache.blobs, m.currentAccount, m.containerName, m.prefix, defaultHierarchyBlobLoadLimit, true))
}
