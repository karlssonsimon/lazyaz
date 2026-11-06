package acapp

import (
	"fmt"

	"github.com/karlssonsimon/lazyaz/internal/appshell"
	"github.com/karlssonsimon/lazyaz/internal/azure"
	"github.com/karlssonsimon/lazyaz/internal/azure/appconfig"
	"github.com/karlssonsimon/lazyaz/internal/ui"

	tea "charm.land/bubbletea/v2"
)

// navigateLeft moves focus one column left. Leaving the value pane
// keeps it open beside the list, like sbapp's preview; it closes when
// the store or subscription changes.
func (m Model) navigateLeft() (Model, tea.Cmd) {
	switch m.focus {
	case valuePane:
		m.transitionTo(settingsPane)
	case settingsPane:
		m.transitionTo(storesPane)
	}
	return m, nil
}

func (m Model) handleBackspace() (Model, tea.Cmd) {
	if m.focus > storesPane {
		return m.navigateLeft()
	}
	return m, nil
}

func (m Model) selectSubscription(sub azure.Subscription) (Model, tea.Cmd) {
	if m.HasSubscription && m.CurrentSub.ID == sub.ID {
		return m, nil
	}
	depart := m.CurrentNav()

	if m.HasSubscription {
		m.storesHistory[m.CurrentSub.ID] = ui.SnapshotListState(&m.storesList, storeItemKey)
	}

	m.CurrentSub = sub
	m.HasSubscription = true
	m.hasStore = false
	m.currentStore = appconfig.Store{}
	m.closeValue()
	m.transitionTo(storesPane)

	if cached, ok := m.cache.stores.Get(sub.ID); ok {
		m.stores = cached
		m.storesList.SetItems(storesToItems(cached))
		m.storesList.Title = fmt.Sprintf("Stores (%d)", len(cached))
	} else {
		m.stores = nil
		m.storesList.SetItems(nil)
		m.storesList.Title = "Stores"
	}
	ui.RestoreListState(&m.storesList, m.storesHistory[sub.ID], storeItemKey)

	m.settings = nil
	m.settingsList.ResetFilter()
	m.settingsList.SetItems(nil)
	m.settingsList.Title = "Settings"

	m.StartLoading(m.focus, fmt.Sprintf("Loading App Configuration stores in %s", ui.SubscriptionDisplayName(sub)))
	return m, recordDeparture(m, depart, tea.Batch(m.Spinner.Tick, fetchStoresCmd(m.service, m.cache.stores, sub.ID, m.stores)))
}

func (m Model) handleEnter() (Model, tea.Cmd) {
	switch m.focus {
	case storesPane:
		item, ok := m.storesList.SelectedItem().(storeItem)
		if !ok {
			return m, nil
		}
		return m.selectStore(item.store)
	case settingsPane:
		item, ok := m.settingsList.SelectedItem().(settingItem)
		if !ok {
			return m, nil
		}
		return m.openValue(item.setting)
	case valuePane:
		return m.openKeyVaultReference()
	}
	return m, nil
}

// selectStore binds the explorer to a store and focuses its settings,
// hydrating from cache and refetching.
func (m Model) selectStore(store appconfig.Store) (Model, tea.Cmd) {
	if m.hasStore && m.currentStore.Name == store.Name {
		m.transitionTo(settingsPane)
		return m, nil
	}
	depart := m.CurrentNav()

	if m.hasStore {
		m.settingsHistory[settingsScope(m.currentStore)] = ui.SnapshotListState(&m.settingsList, settingItemKey)
	}

	m.currentStore = store
	m.hasStore = true
	m.closeValue()
	m.transitionTo(settingsPane)
	ui.SelectByKey(&m.storesList, store.Name, storeItemKey)

	updated, fetch := m.repopulateSettings()
	return updated, recordDeparture(updated, depart, fetch)
}

// repopulateSettings fills the settings list from cache or clears it,
// then triggers the fetch for the current store.
func (m Model) repopulateSettings() (Model, tea.Cmd) {
	scope := settingsScope(m.currentStore)
	if cached, ok := m.cache.settings.Get(scope); ok {
		m.settings = cached
		m.settingsList.SetItems(settingsToItems(cached))
		m.settingsList.Title = fmt.Sprintf("Settings (%d)", len(cached))
	} else {
		m.settings = nil
		m.settingsList.SetItems(nil)
		m.settingsList.Title = "Settings"
	}
	ui.RestoreListState(&m.settingsList, m.settingsHistory[scope], settingItemKey)

	m.StartLoading(m.focus, fmt.Sprintf("Loading settings in %s", m.currentStore.Name))
	return m, tea.Batch(m.Spinner.Tick, fetchSettingsCmd(m.service, m.cache.settings, m.currentStore, m.settings))
}

// openValue shows a setting in the value pane and focuses it. Opening
// is local motion, not a scope change, so it is not a jump.
func (m Model) openValue(setting appconfig.Setting) (Model, tea.Cmd) {
	m.currentSetting = setting
	m.viewingValue = true
	m.transitionTo(valuePane)
	ui.SelectByKey(&m.settingsList, appconfig.SettingKey(setting), settingItemKey)
	m.setValueContent()
	m.valueViewport.GotoTop()
	return m, nil
}

// closeValue drops the value pane. Called whenever the store or
// subscription changes so a value never outlives its scope.
func (m *Model) closeValue() {
	m.viewingValue = false
	m.currentSetting = appconfig.Setting{}
	m.valueViewport.SetContent("")
}

// syncValueToSelection keeps the open value pane on the settings
// cursor as it moves.
func (m *Model) syncValueToSelection() {
	if !m.viewingValue {
		return
	}
	item, ok := m.settingsList.SelectedItem().(settingItem)
	if !ok {
		return
	}
	if appconfig.SettingKey(item.setting) == appconfig.SettingKey(m.currentSetting) {
		return
	}
	m.currentSetting = item.setting
	m.setValueContent()
	m.valueViewport.GotoTop()
}

// openKeyVaultReference asks the parent to open the referenced secret
// in a Key Vault tab, when the current value is a reference.
func (m Model) openKeyVaultReference() (Model, tea.Cmd) {
	if !m.viewingValue {
		return m, nil
	}
	ref, ok := m.currentSetting.KeyVaultReference()
	if !ok {
		return m, nil
	}
	if m.EmbeddedMode {
		sub := m.CurrentSub
		return m, func() tea.Msg {
			return OpenKeyVaultReferenceMsg{Subscription: sub, VaultName: ref.VaultName, SecretName: ref.SecretName}
		}
	}
	m.Notify(appshell.LevelInfo, fmt.Sprintf("Key Vault reference: %s / %s", ref.VaultName, ref.SecretName))
	return m, nil
}
