package kvapp

import (
	"fmt"

	"github.com/karlssonsimon/lazyaz/internal/azure"
	"github.com/karlssonsimon/lazyaz/internal/azure/keyvault"
	"github.com/karlssonsimon/lazyaz/internal/cache"
	"github.com/karlssonsimon/lazyaz/internal/ui"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
)

func (m Model) navigateLeft() (Model, tea.Cmd) {
	switch m.focus {
	case versionsPane:
		m.transitionTo(secretsPane)
		return m, appendJumpRecord(m, nil)
	case secretsPane:
		m.transitionTo(kindPane)
		return m, appendJumpRecord(m, nil)
	case kindPane:
		m.transitionTo(vaultsPane)
		return m, appendJumpRecord(m, nil)
	case vaultsPane:
		return m, nil
	default:
		return m, nil
	}
}

func (m Model) handleBackspace() (Model, tea.Cmd) {
	if m.focus > vaultsPane {
		return m.navigateLeft()
	}
	return m, nil
}

func (m Model) selectSubscription(sub azure.Subscription) (Model, tea.Cmd) {
	// Re-selecting the same subscription: no-op.
	if m.HasSubscription && m.CurrentSub.ID == sub.ID {
		return m, nil
	}

	// Snapshot the current vaults list state under the outgoing sub so
	// navigating back to it later restores the cursor and filter.
	if m.HasSubscription {
		m.vaultsHistory[m.CurrentSub.ID] = ui.SnapshotListState(&m.vaultsList, vaultItemKey)
	}

	m.CurrentSub = sub
	m.HasSubscription = true
	m.hasVault = false
	m.hasSecret = false
	m.hasCert = false
	m.hasKey = false
	m.currentVault = keyvault.Vault{}
	m.currentSecret = keyvault.Secret{}
	m.currentCert = keyvault.Certificate{}
	m.currentKey = keyvault.Key{}
	m.clearSecretSelectionState()
	m.clearReveals()
	m.transitionTo(vaultsPane)

	if cached, ok := m.cache.vaults.Get(sub.ID); ok {
		m.vaults = cached
		m.vaultsList.SetItems(vaultsToItems(cached))
		m.vaultsList.Title = fmt.Sprintf("Vaults (%d)", len(cached))
	} else {
		m.vaults = nil
		m.vaultsList.SetItems(nil)
		m.vaultsList.Title = "Vaults"
	}
	ui.RestoreListState(&m.vaultsList, m.vaultsHistory[sub.ID], vaultItemKey)

	m.secrets = nil
	m.versions = nil
	m.secretsList.ResetFilter()
	m.versionsList.ResetFilter()
	m.secretsList.SetItems(nil)
	m.versionsList.SetItems(nil)
	m.secretsList.Title = "Secrets"
	m.versionsList.Title = "Versions"

	m.startLoading(m.focus, fmt.Sprintf("Loading key vaults in %s", ui.SubscriptionDisplayName(sub)))
	return m, tea.Batch(m.Spinner.Tick, fetchVaultsCmd(m.service, m.cache.vaults, sub.ID, m.vaults))
}

func (m Model) handleEnter() (Model, tea.Cmd) {
	switch m.focus {
	case vaultsPane:
		item, ok := m.vaultsList.SelectedItem().(vaultItem)
		if !ok {
			return m, nil
		}
		return m.selectVault(item.vault)
	case kindPane:
		item, ok := m.kindList.SelectedItem().(kindItem)
		if !ok {
			return m, nil
		}
		return m.selectKind(item.kind)
	case secretsPane:
		switch v := m.secretsList.SelectedItem().(type) {
		case secretItem:
			return m.selectSecret(v.secret)
		case certItem:
			return m.selectCert(v.cert)
		case keyItem:
			return m.selectKey(v.key)
		}
	}
	return m, nil
}

// selectVault binds the explorer to a vault and parks focus on the kind
// chooser. The user explicitly picks Secrets/Certs/Keys before any items
// load — no implicit "default to secrets and fetch" anymore.
func (m Model) selectVault(vault keyvault.Vault) (Model, tea.Cmd) {
	// Re-selecting the same vault: just move focus to kindPane.
	if m.hasVault && m.currentVault.Name == vault.Name {
		m.transitionTo(kindPane)
		return m, appendJumpRecord(m, nil)
	}

	// Snapshot the outgoing vault's items column so sibling navigation
	// restores cursor + filter. Snapshot the OUTGOING kind's scope.
	if m.hasVault {
		m.snapshotMiddleColumn()
	}

	m.currentVault = vault
	m.hasVault = true
	m.hasSecret = false
	m.hasCert = false
	m.hasKey = false
	m.currentSecret = keyvault.Secret{}
	m.currentCert = keyvault.Certificate{}
	m.currentKey = keyvault.Key{}
	m.clearSecretSelectionState()
	m.clearReveals()
	// Reset the kind cursor to whichever kvKind is current so the user
	// sees their last-chosen type pre-selected when they re-enter a vault.
	m.syncKindListCursor()
	m.transitionTo(kindPane)
	ui.SelectByKey(&m.vaultsList, vault.Name, vaultItemKey)

	// Items column reflects the still-active kind so the user sees
	// what's coming once they Enter, but the fetch waits for selectKind.
	return m.repopulateMiddleColumn(true /*resetVersions*/)
}

// selectKind sets m.kvKind from a kindPane row, focuses the items
// column, and triggers the per-kind fetch via repopulateMiddleColumn.
func (m Model) selectKind(kind kvKind) (Model, tea.Cmd) {
	if !m.hasVault {
		return m, nil
	}
	// Same kind already active: just move focus.
	if m.kvKind == kind {
		m.transitionTo(secretsPane)
		return m, appendJumpRecord(m, nil)
	}
	// Different kind — snapshot the outgoing kind's items column and
	// reset per-kind selection state so the right column doesn't keep
	// stale data from the prior kind.
	m.snapshotMiddleColumn()
	m.kvKind = kind
	m.hasSecret = false
	m.hasCert = false
	m.hasKey = false
	m.currentSecret = keyvault.Secret{}
	m.currentCert = keyvault.Certificate{}
	m.currentKey = keyvault.Key{}
	m.transitionTo(secretsPane)
	return m.repopulateMiddleColumn(true /*resetVersions*/)
}

// syncKindListCursor moves the kindList cursor onto the row matching
// m.kvKind so a re-entry into a vault shows the user's previous choice
// pre-highlighted. Cheap (3 items).
func (m *Model) syncKindListCursor() {
	for i, it := range m.kindList.Items() {
		if ki, ok := it.(kindItem); ok && ki.kind == m.kvKind {
			m.kindList.Select(i)
			return
		}
	}
}

// snapshotMiddleColumn stores the middle column's list state under the
// scope that matches the OUTGOING kind. Called before any navigation
// that swaps what the middle column shows.
func (m *Model) snapshotMiddleColumn() {
	scope := cache.Key(m.CurrentSub.ID, m.currentVault.Name)
	m.secretsHistory[middleHistoryKey(m.kvKind, scope)] = ui.SnapshotListState(&m.secretsList, middleItemKeyForList(m.kvKind))
}

// middleHistoryKey scopes the history entry by both vault and kind so
// switching kinds within a vault doesn't clobber each other's cursors.
func middleHistoryKey(kind kvKind, vaultScope string) string {
	return vaultScope + "/" + kind.String()
}

// repopulateMiddleColumn fills the middle list from the cache or clears
// it, then triggers a fetch for the active kind. resetVersions clears
// the right column too — used when switching vault or kind.
func (m Model) repopulateMiddleColumn(resetVersions bool) (Model, tea.Cmd) {
	scope := cache.Key(m.CurrentSub.ID, m.currentVault.Name)

	switch m.kvKind {
	case kvKindCertificates:
		if cached, ok := m.cache.certs.Get(scope); ok {
			m.certs = cached
			m.secretsList.SetItems(certsToItems(cached))
			m.secretsList.Title = fmt.Sprintf("Certificates (%d)", len(cached))
		} else {
			m.certs = nil
			m.secretsList.SetItems(nil)
			m.secretsList.Title = "Certificates"
		}
	case kvKindKeys:
		if cached, ok := m.cache.keys.Get(scope); ok {
			m.keys = cached
			m.secretsList.SetItems(keysToItems(cached))
			m.secretsList.Title = fmt.Sprintf("Keys (%d)", len(cached))
		} else {
			m.keys = nil
			m.secretsList.SetItems(nil)
			m.secretsList.Title = "Keys"
		}
	default:
		if cached, ok := m.cache.secrets.Get(scope); ok {
			m.secrets = cached
			m.secretsList.SetItems(secretsToItems(cached))
			m.secretsList.Title = fmt.Sprintf("Secrets (%d)", len(cached))
		} else {
			m.secrets = nil
			m.secretsList.SetItems(nil)
			m.secretsList.Title = "Secrets"
		}
	}

	keyFn := middleItemKeyForList(m.kvKind)
	ui.RestoreListState(&m.secretsList, m.secretsHistory[middleHistoryKey(m.kvKind, scope)], keyFn)

	if resetVersions {
		m.versions = nil
		m.certVersions = nil
		m.keyVersions = nil
		m.versionsList.ResetFilter()
		m.versionsList.SetItems(nil)
		m.versionsList.Title = "Versions"
	}

	updated, fetch := m.fetchMiddleColumn()
	return updated, appendJumpRecord(updated, fetch)
}

// middleItemKeyForList returns a list.Item-keyed extractor (the shape
// ui.RestoreListState wants).
func middleItemKeyForList(kind kvKind) func(it list.Item) string {
	switch kind {
	case kvKindCertificates:
		return certItemKey
	case kvKindKeys:
		return keyItemKey
	default:
		return secretItemKey
	}
}


// selectCert binds the explorer to a certificate and loads its versions.
func (m Model) selectCert(cert keyvault.Certificate) (Model, tea.Cmd) {
	if m.hasCert && m.currentCert.Name == cert.Name {
		m.transitionTo(versionsPane)
		return m, appendJumpRecord(m, nil)
	}
	if m.hasCert {
		oldKey := cache.Key(m.CurrentSub.ID, m.currentVault.Name, m.currentCert.Name)
		m.versionsHistory[oldKey] = ui.SnapshotListState(&m.versionsList, versionItemKey)
	}
	m.currentCert = cert
	m.hasCert = true
	m.transitionTo(versionsPane)
	ui.SelectByKey(&m.secretsList, cert.Name, certItemKey)

	scope := cache.Key(m.CurrentSub.ID, m.currentVault.Name, cert.Name)
	if cached, ok := m.cache.certVersions.Get(scope); ok {
		m.certVersions = cached
		m.versionsList.SetItems(certVersionsToItems(cached))
		m.versionsList.Title = fmt.Sprintf("Versions (%d)", len(cached))
	} else {
		m.certVersions = nil
		m.versionsList.SetItems(nil)
		m.versionsList.Title = "Versions"
	}
	ui.RestoreListState(&m.versionsList, m.versionsHistory[scope], versionItemKey)

	m.startLoading(m.focus, fmt.Sprintf("Loading versions for %s", cert.Name))
	return m, appendJumpRecord(m, tea.Batch(m.Spinner.Tick, fetchCertVersionsCmd(m.service, m.cache.certVersions, m.currentVault, cert.Name, m.certVersions)))
}

// selectKey binds the explorer to a key and loads its versions.
func (m Model) selectKey(key keyvault.Key) (Model, tea.Cmd) {
	if m.hasKey && m.currentKey.Name == key.Name {
		m.transitionTo(versionsPane)
		return m, appendJumpRecord(m, nil)
	}
	if m.hasKey {
		oldKey := cache.Key(m.CurrentSub.ID, m.currentVault.Name, m.currentKey.Name)
		m.versionsHistory[oldKey] = ui.SnapshotListState(&m.versionsList, versionItemKey)
	}
	m.currentKey = key
	m.hasKey = true
	m.transitionTo(versionsPane)
	ui.SelectByKey(&m.secretsList, key.Name, keyItemKey)

	scope := cache.Key(m.CurrentSub.ID, m.currentVault.Name, key.Name)
	if cached, ok := m.cache.keyVersions.Get(scope); ok {
		m.keyVersions = cached
		m.versionsList.SetItems(keyVersionsToItems(cached))
		m.versionsList.Title = fmt.Sprintf("Versions (%d)", len(cached))
	} else {
		m.keyVersions = nil
		m.versionsList.SetItems(nil)
		m.versionsList.Title = "Versions"
	}
	ui.RestoreListState(&m.versionsList, m.versionsHistory[scope], versionItemKey)

	m.startLoading(m.focus, fmt.Sprintf("Loading versions for %s", key.Name))
	return m, appendJumpRecord(m, tea.Batch(m.Spinner.Tick, fetchKeyVersionsCmd(m.service, m.cache.keyVersions, m.currentVault, key.Name, m.keyVersions)))
}

// selectSecret binds the explorer to a secret under the active vault
// and loads versions.
func (m Model) selectSecret(secret keyvault.Secret) (Model, tea.Cmd) {
	// Re-selecting the same secret: just move focus.
	if m.hasSecret && m.currentSecret.Name == secret.Name {
		m.transitionTo(versionsPane)
		return m, appendJumpRecord(m, nil)
	}

	// Snapshot the current versions list under the outgoing secret.
	if m.hasSecret {
		oldKey := cache.Key(m.CurrentSub.ID, m.currentVault.Name, m.currentSecret.Name)
		m.versionsHistory[oldKey] = ui.SnapshotListState(&m.versionsList, versionItemKey)
	}

	m.currentSecret = secret
	m.hasSecret = true
	m.transitionTo(versionsPane)
	ui.SelectByKey(&m.secretsList, secret.Name, secretItemKey)

	versionScope := cache.Key(m.CurrentSub.ID, m.currentVault.Name, secret.Name)
	if cached, ok := m.cache.versions.Get(versionScope); ok {
		m.versions = cached
		m.versionsList.SetItems(versionsToItems(cached))
		m.versionsList.Title = fmt.Sprintf("Versions (%d)", len(cached))
	} else {
		m.versions = nil
		m.versionsList.SetItems(nil)
		m.versionsList.Title = "Versions"
	}
	ui.RestoreListState(&m.versionsList, m.versionsHistory[versionScope], versionItemKey)

	m.startLoading(m.focus, fmt.Sprintf("Loading versions for %s", secret.Name))
	return m, appendJumpRecord(m, tea.Batch(m.Spinner.Tick, fetchVersionsCmd(m.service, m.cache.versions, m.currentVault, secret.Name, m.versions)))
}
