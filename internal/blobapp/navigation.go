package blobapp

import (
	"fmt"

	"azure-storage/internal/azure"
	"azure-storage/internal/azure/blob"
	"azure-storage/internal/cache"
	"azure-storage/internal/ui"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
)

func (m Model) selectSubscription(sub azure.Subscription) (Model, tea.Cmd) {
	// Re-selecting the same subscription: no-op.
	if m.hasSubscription && m.currentSub.ID == sub.ID {
		return m, nil
	}

	m.currentSub = sub
	m.hasSubscription = true
	m.hasAccount = false
	m.hasContainer = false
	m.currentAccount = blob.Account{}
	m.containerName = ""
	m.prefix = ""
	m.clearBlobSelectionState()
	m.resetBlobLoadState()
	m.resetPreviewState()
	m.focus = accountsPane

	if cached, ok := m.cache.accounts.Get(sub.ID); ok {
		m.accounts = cached
		m.accountsList.ResetFilter()
		ui.SetItemsPreserveIndex(&m.accountsList, accountsToItems(cached))
		m.accountsList.Title = fmt.Sprintf("Storage Accounts (%d)", len(cached))
	} else {
		m.accounts = nil
		m.accountsList.ResetFilter()
		m.accountsList.SetItems(nil)
		m.accountsList.Title = "Storage Accounts"
	}

	m.containers = nil
	m.blobs = nil
	m.containersList.ResetFilter()
	m.blobsList.ResetFilter()
	m.containersList.SetItems(nil)
	m.blobsList.SetItems(nil)
	m.containersList.Title = "Containers"
	m.blobsList.Title = "Blobs"

	m.loading = true
	m.status = fmt.Sprintf("Loading storage accounts in %s", subscriptionDisplayName(sub))
	return m, tea.Batch(spinner.Tick, fetchAccountsCmd(m.service, m.cache.accounts, sub.ID))
}

func (m Model) refresh() (Model, tea.Cmd) {
	if !m.hasSubscription {
		m.loading = true
		m.lastErr = ""
		m.status = "Refreshing subscriptions..."
		return m, tea.Batch(spinner.Tick, fetchSubscriptionsCmd(m.service, m.cache.subscriptions))
	}

	if !m.hasAccount || m.focus == accountsPane {
		m.loading = true
		m.lastErr = ""
		m.status = fmt.Sprintf("Loading storage accounts in %s", subscriptionDisplayName(m.currentSub))
		return m, tea.Batch(spinner.Tick, fetchAccountsCmd(m.service, m.cache.accounts, m.currentSub.ID))
	}

	if m.focus == containersPane || !m.hasContainer {
		m.loading = true
		m.lastErr = ""
		m.status = fmt.Sprintf("Loading containers in %s", m.currentAccount.Name)
		return m, tea.Batch(spinner.Tick, fetchContainersCmd(m.service, m.cache.containers, m.currentAccount))
	}
	if m.focus == previewPane && m.preview.open {
		return m.ensurePreviewWindowAtCursor()
	}

	m.loading = true
	m.lastErr = ""
	if m.blobLoadAll {
		m.status = fmt.Sprintf("Loading all blobs in %s/%s", m.currentAccount.Name, m.containerName)
		return m, tea.Batch(spinner.Tick, fetchAllBlobsCmd(m.service, m.cache.blobs, m.currentAccount, m.containerName, m.prefix))
	}
	if m.search.prefixLocked && m.search.prefixQuery != "" {
		effectivePrefix := blobSearchPrefix(m.prefix, m.search.prefixQuery)
		m.status = fmt.Sprintf("Searching blobs by prefix %q...", effectivePrefix)
		return m, tea.Batch(spinner.Tick, fetchSearchBlobsCmd(m.service, m.cache.blobs, m.currentAccount, m.containerName, m.prefix, m.search.prefixQuery, defaultBlobPrefixSearchLimit))
	}
	m.loading = true
	m.status = fmt.Sprintf("Loading up to %d entries under %q", defaultHierarchyBlobLoadLimit, m.prefix)
	return m, tea.Batch(spinner.Tick, fetchHierarchyBlobsCmd(m.service, m.cache.blobs, m.currentAccount, m.containerName, m.prefix, defaultHierarchyBlobLoadLimit))
}

func (m Model) navigateLeft() (Model, tea.Cmd) {
	switch m.focus {
	case previewPane:
		m.focus = blobsPane
		return m, nil
	case blobsPane:
		if m.hasContainer && !m.blobLoadAll && m.prefix != "" {
			m.deactivateSearch()
			m.prefix = parentPrefix(m.prefix)

			if cached, ok := m.cache.blobs.Get(blobsCacheKey(m.currentSub.ID, m.currentAccount.Name, m.containerName, m.prefix, false)); ok {
				m.blobs = cached
				m.blobsList.Title = fmt.Sprintf("Blobs (%d)", len(cached))
				m.refreshBlobItems()
			}

			m.loading = true
			m.status = fmt.Sprintf("Loading up to %d entries under %q", defaultHierarchyBlobLoadLimit, m.prefix)
			return m, tea.Batch(spinner.Tick, fetchHierarchyBlobsCmd(m.service, m.cache.blobs, m.currentAccount, m.containerName, m.prefix, defaultHierarchyBlobLoadLimit))
		}
		if m.visualLineMode {
			m.visualLineMode = false
			m.visualAnchor = ""
			m.refreshBlobItems()
		}
		m.focus = containersPane
		return m, nil
	case containersPane:
		m.focus = accountsPane
		return m, nil
	case accountsPane:
		return m, nil
	default:
		return m, nil
	}
}

func (m Model) handleEnter() (Model, tea.Cmd) {
	if m.focus == accountsPane {
		item, ok := m.accountsList.SelectedItem().(accountItem)
		if !ok {
			return m, nil
		}

		// Re-selecting the same account: just move focus.
		if m.hasAccount && sameAccount(m.currentAccount, item.account) {
			m.focus = containersPane
			return m, nil
		}

		m.currentAccount = item.account
		m.hasAccount = true
		m.hasContainer = false
		m.containerName = ""
		m.prefix = ""
		m.clearBlobSelectionState()
		m.resetBlobLoadState()
		m.resetPreviewState()
		m.focus = containersPane

		if cached, ok := m.cache.containers.Get(cache.Key(m.currentSub.ID, item.account.Name)); ok {
			m.containers = cached
			m.containersList.ResetFilter()
			ui.SetItemsPreserveIndex(&m.containersList, containersToItems(cached))
			m.containersList.Title = fmt.Sprintf("Containers (%d)", len(cached))
		} else {
			m.containers = nil
			m.containersList.ResetFilter()
			m.containersList.SetItems(nil)
			m.containersList.Title = "Containers"
		}

		m.blobs = nil
		m.blobsList.ResetFilter()
		m.blobsList.SetItems(nil)
		m.blobsList.Title = "Blobs"

		m.loading = true
		m.status = fmt.Sprintf("Loading containers in %s", item.account.Name)
		return m, tea.Batch(spinner.Tick, fetchContainersCmd(m.service, m.cache.containers, item.account))
	}

	if m.focus == containersPane {
		item, ok := m.containersList.SelectedItem().(containerItem)
		if !ok {
			return m, nil
		}

		// Re-selecting the same container: just move focus.
		if m.hasContainer && m.containerName == item.container.Name {
			m.focus = blobsPane
			return m, nil
		}

		m.containerName = item.container.Name
		m.hasContainer = true
		m.prefix = ""
		m.clearBlobSelectionState()
		m.resetBlobLoadState()
		m.resetPreviewState()
		m.focus = blobsPane

		if cached, ok := m.cache.blobs.Get(blobsCacheKey(m.currentSub.ID, m.currentAccount.Name, item.container.Name, "", false)); ok {
			m.blobs = cached
			m.blobsList.ResetFilter()
			m.blobsList.Title = fmt.Sprintf("Blobs (%d)", len(cached))
			m.refreshBlobItems()
		} else {
			m.blobs = nil
			m.blobsList.ResetFilter()
			m.blobsList.SetItems(nil)
			m.blobsList.Title = "Blobs"
		}

		m.loading = true
		m.status = fmt.Sprintf("Loading up to %d entries in %s/%s", defaultHierarchyBlobLoadLimit, m.currentAccount.Name, m.containerName)
		return m, tea.Batch(spinner.Tick, fetchHierarchyBlobsCmd(m.service, m.cache.blobs, m.currentAccount, m.containerName, m.prefix, defaultHierarchyBlobLoadLimit))
	}

	if m.focus == blobsPane {
		item, ok := m.blobsList.SelectedItem().(blobItem)
		if !ok {
			return m, nil
		}

		if item.blob.IsPrefix {
			if m.blobLoadAll {
				m.status = "Directory navigation is unavailable when all blobs are loaded"
				return m, nil
			}
			m.deactivateSearch()
			m.prefix = item.blob.Name

			if cached, ok := m.cache.blobs.Get(blobsCacheKey(m.currentSub.ID, m.currentAccount.Name, m.containerName, m.prefix, false)); ok {
				m.blobs = cached
				m.blobsList.Title = fmt.Sprintf("Blobs (%d)", len(cached))
				m.refreshBlobItems()
			}

			m.loading = true
			m.status = fmt.Sprintf("Loading up to %d entries under %q", defaultHierarchyBlobLoadLimit, m.prefix)
			return m, tea.Batch(spinner.Tick, fetchHierarchyBlobsCmd(m.service, m.cache.blobs, m.currentAccount, m.containerName, m.prefix, defaultHierarchyBlobLoadLimit))
		}

		return m.openPreview(item.blob)
	}

	return m, nil
}

func paneName(pane int) string {
	switch pane {
	case accountsPane:
		return "storage accounts"
	case containersPane:
		return "containers"
	case blobsPane:
		return "blobs"
	case previewPane:
		return "preview"
	default:
		return "items"
	}
}

func subscriptionDisplayName(sub azure.Subscription) string {
	return ui.SubscriptionDisplayName(sub)
}

func sameAccount(a, b blob.Account) bool {
	return a.Name == b.Name && a.SubscriptionID == b.SubscriptionID
}
