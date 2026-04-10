package blobapp

import (
	"fmt"

	"github.com/karlssonsimon/lazyaz/internal/appshell"
	"github.com/karlssonsimon/lazyaz/internal/azure"
	"github.com/karlssonsimon/lazyaz/internal/azure/blob"
	"github.com/karlssonsimon/lazyaz/internal/cache"
	"github.com/karlssonsimon/lazyaz/internal/ui"

	tea "charm.land/bubbletea/v2"
)

func (m Model) selectSubscription(sub azure.Subscription) (Model, tea.Cmd) {
	// Re-selecting the same subscription: no-op.
	if m.HasSubscription && m.CurrentSub.ID == sub.ID {
		return m, nil
	}

	// Snapshot the current accounts list under the outgoing sub.
	if m.HasSubscription {
		m.accountsHistory[m.CurrentSub.ID] = ui.SnapshotListState(&m.accountsList, accountItemKey)
	}

	m.CurrentSub = sub
	m.HasSubscription = true
	m.hasAccount = false
	m.hasContainer = false
	m.currentAccount = blob.Account{}
	m.containerName = ""
	m.prefix = ""
	m.clearBlobSelectionState()
	m.resetBlobLoadState()
	m.resetPreviewState()
	m.setFocus(accountsPane)

	if cached, ok := m.cache.accounts.Get(sub.ID); ok {
		m.accounts = cached
		m.accountsList.SetItems(accountsToItems(cached))
		m.accountsList.Title = fmt.Sprintf("Storage Accounts (%d)", len(cached))
	} else {
		m.accounts = nil
		m.accountsList.SetItems(nil)
		m.accountsList.Title = "Storage Accounts"
	}
	ui.RestoreListState(&m.accountsList, m.accountsHistory[sub.ID], accountItemKey)

	m.containers = nil
	m.blobs = nil
	m.containersList.ResetFilter()
	m.blobsList.ResetFilter()
	m.containersList.SetItems(nil)
	m.blobsList.SetItems(nil)
	m.containersList.Title = "Containers"
	m.blobsList.Title = "Blobs"

	m.SetLoading(accountsPane)
	m.loadingSpinnerID = m.NotifySpinner(fmt.Sprintf("Loading storage accounts in %s", ui.SubscriptionDisplayName(sub)))
	return m, tea.Batch(m.Spinner.Tick, fetchAccountsCmd(m.service, m.cache.accounts, sub.ID, m.accounts))
}

func (m Model) navigateLeft() (Model, tea.Cmd) {
	switch m.focus {
	case previewPane:
		m.setFocus(blobsPane)
		return m, nil
	case blobsPane:
		if m.hasContainer && !m.blobLoadAll && m.prefix != "" {
			// Snapshot current prefix's blobs list before going up.
			oldKey := blobsCacheKey(m.CurrentSub.ID, m.currentAccount.Name, m.containerName, m.prefix, false)
			m.blobsHistory[oldKey] = ui.SnapshotListState(&m.blobsList, blobItemKey)

			m.clearFilter()
			m.prefix = parentPrefix(m.prefix)

			blobsScope := blobsCacheKey(m.CurrentSub.ID, m.currentAccount.Name, m.containerName, m.prefix, false)
			if cached, ok := m.cache.blobs.Get(blobsScope); ok {
				m.blobs = cached
				m.blobsList.Title = fmt.Sprintf("Blobs (%d)", len(cached))
				m.refreshItems()
			}
			ui.RestoreListState(&m.blobsList, m.blobsHistory[blobsScope], blobItemKey)

			// Update parent blobs list for the new parent prefix.
			m.rebuildParentBlobsList()

			m.SetLoading(blobsPane)
			m.loadingSpinnerID = m.NotifySpinner(fmt.Sprintf("Loading up to %d entries under %q", defaultHierarchyBlobLoadLimit, displayPrefix(m.prefix)))
			return m, tea.Batch(m.Spinner.Tick, fetchHierarchyBlobsCmd(m.service, m.cache.blobs, m.currentAccount, m.containerName, m.prefix, defaultHierarchyBlobLoadLimit, m.blobs))
		}
		if m.visualLineMode {
			m.visualLineMode = false
			m.visualAnchor = ""
			m.refreshItems()
		}
		m.setFocus(containersPane)
		return m, nil
	case containersPane:
		m.setFocus(accountsPane)
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
			m.setFocus(containersPane)
			return m, nil
		}

		// Snapshot containers list under the outgoing account.
		if m.hasAccount {
			oldKey := cache.Key(m.CurrentSub.ID, m.currentAccount.Name)
			m.containersHistory[oldKey] = ui.SnapshotListState(&m.containersList, containerItemKey)
		}

		m.currentAccount = item.account
		m.hasAccount = true
		m.hasContainer = false
		m.containerName = ""
		m.prefix = ""
		m.clearBlobSelectionState()
		m.resetBlobLoadState()
		m.resetPreviewState()
		m.setFocus(containersPane)

		containersScope := cache.Key(m.CurrentSub.ID, item.account.Name)
		if cached, ok := m.cache.containers.Get(containersScope); ok {
			m.containers = cached
			m.containersList.SetItems(containersToItems(cached))
			m.containersList.Title = fmt.Sprintf("Containers (%d)", len(cached))
		} else {
			m.containers = nil
			m.containersList.SetItems(nil)
			m.containersList.Title = "Containers"
		}
		ui.RestoreListState(&m.containersList, m.containersHistory[containersScope], containerItemKey)

		m.blobs = nil
		m.blobsList.ResetFilter()
		m.blobsList.SetItems(nil)
		m.blobsList.Title = "Blobs"

		m.SetLoading(containersPane)
		m.loadingSpinnerID = m.NotifySpinner(fmt.Sprintf("Loading containers in %s", item.account.Name))
		return m, tea.Batch(m.Spinner.Tick, fetchContainersCmd(m.service, m.cache.containers, item.account, m.containers))
	}

	if m.focus == containersPane {
		item, ok := m.containersList.SelectedItem().(containerItem)
		if !ok {
			return m, nil
		}

		// Re-selecting the same container: just move focus.
		if m.hasContainer && m.containerName == item.container.Name {
			m.setFocus(blobsPane)
			return m, nil
		}

		// Snapshot blobs list under outgoing container (including prefix
		// and load-all flag).
		if m.hasContainer {
			oldKey := blobsCacheKey(m.CurrentSub.ID, m.currentAccount.Name, m.containerName, m.prefix, m.blobLoadAll)
			m.blobsHistory[oldKey] = ui.SnapshotListState(&m.blobsList, blobItemKey)
		}

		m.containerName = item.container.Name
		m.hasContainer = true
		m.prefix = ""
		m.clearBlobSelectionState()
		m.resetBlobLoadState()
		m.resetPreviewState()
		m.setFocus(blobsPane)

		blobsScope := blobsCacheKey(m.CurrentSub.ID, m.currentAccount.Name, item.container.Name, "", false)
		if cached, ok := m.cache.blobs.Get(blobsScope); ok {
			m.blobs = cached
			m.blobsList.Title = fmt.Sprintf("Blobs (%d)", len(cached))
			m.refreshItems()
		} else {
			m.blobs = nil
			m.blobsList.SetItems(nil)
			m.blobsList.Title = "Blobs"
		}
		ui.RestoreListState(&m.blobsList, m.blobsHistory[blobsScope], blobItemKey)

		m.SetLoading(blobsPane)
		m.loadingSpinnerID = m.NotifySpinner(fmt.Sprintf("Loading up to %d entries in %s/%s", defaultHierarchyBlobLoadLimit, m.currentAccount.Name, m.containerName))
		return m, tea.Batch(m.Spinner.Tick, fetchHierarchyBlobsCmd(m.service, m.cache.blobs, m.currentAccount, m.containerName, m.prefix, defaultHierarchyBlobLoadLimit, m.blobs))
	}

	if m.focus == blobsPane {
		item, ok := m.blobsList.SelectedItem().(blobItem)
		if !ok {
			return m, nil
		}

		if item.blob.IsPrefix {
			if m.blobLoadAll {
				m.Notify(appshell.LevelInfo, "Directory navigation is unavailable when all blobs are loaded")
				return m, nil
			}
			// Snapshot the current prefix's blobs list before descending.
			oldKey := blobsCacheKey(m.CurrentSub.ID, m.currentAccount.Name, m.containerName, m.prefix, m.blobLoadAll)
			m.blobsHistory[oldKey] = ui.SnapshotListState(&m.blobsList, blobItemKey)

			// Populate parent blobs list with current folder's contents
			// so the left column shows where we came from.
			m.updateParentBlobsList()

			m.clearFilter()
			m.prefix = item.blob.Name

			blobsScope := blobsCacheKey(m.CurrentSub.ID, m.currentAccount.Name, m.containerName, m.prefix, false)
			if cached, ok := m.cache.blobs.Get(blobsScope); ok {
				m.blobs = cached
				m.blobsList.Title = fmt.Sprintf("Blobs (%d)", len(cached))
				m.refreshItems()
			}
			ui.RestoreListState(&m.blobsList, m.blobsHistory[blobsScope], blobItemKey)

			m.SetLoading(blobsPane)
			m.loadingSpinnerID = m.NotifySpinner(fmt.Sprintf("Loading up to %d entries under %q", defaultHierarchyBlobLoadLimit, displayPrefix(m.prefix)))
			return m, tea.Batch(m.Spinner.Tick, fetchHierarchyBlobsCmd(m.service, m.cache.blobs, m.currentAccount, m.containerName, m.prefix, defaultHierarchyBlobLoadLimit, m.blobs))
		}

		return m.openPreview(item.blob)
	}

	return m, nil
}

// updateParentBlobsList copies the current blobs list contents into the
// parent blobs list. Called before descending into a subfolder so the
// left column shows the folder we came from.
func (m *Model) updateParentBlobsList() {
	parentPrefix := m.prefix
	pw := ui.PaneContentWidth(m.Styles.Chrome.Pane, m.paneWidths[containersPane])
	items := blobsToItems(m.blobs, parentPrefix, pw)
	m.parentBlobsList.SetItems(items)
	// Position cursor on the folder we're about to enter.
	if sel, ok := m.blobsList.SelectedItem().(blobItem); ok {
		for i, it := range m.parentBlobsList.VisibleItems() {
			if bi, ok := it.(blobItem); ok && bi.blob.Name == sel.blob.Name {
				m.parentBlobsList.Select(i)
				break
			}
		}
	}
}

// rebuildParentBlobsList rebuilds the parent blobs list from cache for
// the parent of the current prefix. Called after going up a folder.
func (m *Model) rebuildParentBlobsList() {
	if m.prefix == "" {
		// At container root — parent is containers, not blobs.
		m.parentBlobsList.SetItems(nil)
		return
	}
	pp := parentPrefix(m.prefix)
	scope := blobsCacheKey(m.CurrentSub.ID, m.currentAccount.Name, m.containerName, pp, false)
	if cached, ok := m.cache.blobs.Get(scope); ok {
		pw := ui.PaneContentWidth(m.Styles.Chrome.Pane, m.paneWidths[containersPane])
		items := blobsToItems(cached, pp, pw)
		m.parentBlobsList.SetItems(items)
		// Position cursor on the current prefix folder.
		for i, it := range m.parentBlobsList.VisibleItems() {
			if bi, ok := it.(blobItem); ok && bi.blob.Name == m.prefix {
				m.parentBlobsList.Select(i)
				break
			}
		}
	}
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

func sameAccount(a, b blob.Account) bool {
	return a.Name == b.Name && a.SubscriptionID == b.SubscriptionID
}
