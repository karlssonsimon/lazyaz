package acapp

import (
	"github.com/karlssonsimon/lazyaz/internal/azure/appconfig"

	"charm.land/bubbles/v2/list"
)

type storeItem struct {
	store appconfig.Store
}

func (i storeItem) Title() string       { return i.store.Name }
func (i storeItem) Description() string { return "" }

// FilterValue is the visible row text only, so filter matches always
// have a visible reason.
func (i storeItem) FilterValue() string { return i.store.Name }

type settingItem struct {
	setting appconfig.Setting
}

func (i settingItem) Title() string       { return i.setting.DisplayKey() }
func (i settingItem) Description() string { return "" }
func (i settingItem) FilterValue() string { return i.setting.DisplayKey() }

func storesToItems(stores []appconfig.Store) []list.Item {
	items := make([]list.Item, len(stores))
	for i, s := range stores {
		items[i] = storeItem{store: s}
	}
	return items
}

func settingsToItems(settings []appconfig.Setting) []list.Item {
	items := make([]list.Item, len(settings))
	for i, s := range settings {
		items[i] = settingItem{setting: s}
	}
	return items
}

// List item key functions for ui.SetItemsPreserveKey and the list-state
// helpers. A setting is identified by key and label together.
func storeItemKey(it list.Item) string {
	if si, ok := it.(storeItem); ok {
		return si.store.Name
	}
	return ""
}

func settingItemKey(it list.Item) string {
	if si, ok := it.(settingItem); ok {
		return appconfig.SettingKey(si.setting)
	}
	return ""
}
