package acapp

import (
	"context"
	"fmt"
	"time"

	"github.com/karlssonsimon/lazyaz/internal/appshell"
	"github.com/karlssonsimon/lazyaz/internal/azure/appconfig"
	"github.com/karlssonsimon/lazyaz/internal/ui"

	tea "charm.land/bubbletea/v2"
)

// Editing a value: action menu → form prefilled with the current value
// → confirmation showing old and new → conditional write. The write
// carries the ETag the setting was read with, so if someone changed it
// in between the save fails instead of overwriting their change.

// actionSetting is the setting an action applies to: the one in the
// value pane when focused, else the settings cursor.
func (m Model) actionSetting() (appconfig.Setting, bool) {
	if m.viewingValue && m.focus == valuePane {
		return m.currentSetting, true
	}
	if m.focus == settingsPane {
		if item, ok := m.settingsList.SelectedItem().(settingItem); ok {
			return item.setting, true
		}
	}
	return appconfig.Setting{}, false
}

// openEditValueForm mounts the single-field form for the action
// setting. Locked settings are refused up front with the reason.
func (m Model) openEditValueForm() (Model, tea.Cmd) {
	setting, ok := m.actionSetting()
	if !ok || !m.hasStore {
		return m, nil
	}
	if setting.ReadOnly {
		m.Notify(appshell.LevelWarn, fmt.Sprintf("%s is locked (read-only); unlock it in the portal before editing", setting.DisplayKey()))
		return m, nil
	}
	m.editTarget = setting
	crumbs := []string{m.currentStore.Name, setting.DisplayKey()}
	if setting.Label != "" {
		crumbs = append(crumbs, setting.Label)
	}
	help := "saved as is; the content type stays " + ui.EmptyToDash(setting.ContentType)
	m.editForm.OpenWithBreadcrumb("Edit value", crumbs, []ui.FormField{
		{
			Label:  "Value",
			Value:  setting.Value,
			Cursor: len([]rune(setting.Value)),
			Help:   help,
		},
	})
	return m, nil
}

// handleFormKey routes keys to the edit form. Submit opens the
// confirmation; cancel just closes.
func (m Model) handleFormKey(key string) (Model, tea.Cmd) {
	res := m.editForm.HandleKey(key)
	if res.Action != ui.FormActionSubmit || len(res.Values) < 1 {
		return m, nil
	}
	newValue := res.Values[0]
	target := m.editTarget
	if newValue == target.Value {
		m.Notify(appshell.LevelInfo, "Value unchanged")
		return m, nil
	}
	return m.confirmSave(target, newValue, target.Value)
}

type settingSavedMsg struct {
	store   appconfig.Store
	before  appconfig.Setting
	setting appconfig.Setting
	err     error
}

func setValueCmd(svc *appconfig.Service, store appconfig.Store, setting appconfig.Setting, value string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		saved, err := svc.SetValue(ctx, store, setting, value)
		return settingSavedMsg{store: store, before: setting, setting: saved, err: err}
	}
}

func (m Model) handleSettingSaved(msg settingSavedMsg) (Model, tea.Cmd) {
	m.ClearLoading()
	key := msg.before.DisplayKey()
	if msg.err != nil {
		switch {
		case appconfig.IsConflict(msg.err):
			m.ResolveSpinner(m.LoadingSpinnerID, appshell.LevelError, fmt.Sprintf("%s changed since you opened it; refresh and edit again", key))
		case appconfig.IsLocked(msg.err):
			m.ResolveSpinner(m.LoadingSpinnerID, appshell.LevelError, fmt.Sprintf("%s is locked (read-only); unlock it before editing", key))
		default:
			m.ResolveSpinner(m.LoadingSpinnerID, appshell.LevelError, fmt.Sprintf("Failed to save %s: %s", key, msg.err.Error()))
		}
		return m, nil
	}
	m.ResolveSpinner(m.LoadingSpinnerID, appshell.LevelSuccess, fmt.Sprintf("Saved %s in %s", key, msg.store.Name))

	// Show the stored value at once, then refresh the list so the cache
	// and every row agree with the store.
	if m.hasStore && m.currentStore.Name == msg.store.Name {
		m.replaceSetting(msg.setting)
		if m.viewingValue && appconfig.SettingKey(m.currentSetting) == appconfig.SettingKey(msg.setting) {
			m.currentSetting = msg.setting
			m.setValueContent()
		}
		m.StartLoading(settingsPane, fmt.Sprintf("Refreshing settings in %s", m.currentStore.Name))
		return m, tea.Batch(m.Spinner.Tick, fetchSettingsCmd(m.service, m.cache.settings, m.currentStore, m.settings))
	}
	return m, nil
}

// replaceSetting swaps the loaded copy of a setting and re-renders the
// list row. The slice is copied first: the cache broker hands out the
// slice it also reads from its fetch worker, so writing into it in
// place would race.
func (m *Model) replaceSetting(s appconfig.Setting) {
	for i := range m.settings {
		if appconfig.SettingKey(m.settings[i]) == appconfig.SettingKey(s) {
			updated := make([]appconfig.Setting, len(m.settings))
			copy(updated, m.settings)
			updated[i] = s
			m.settings = updated
			ui.SetItemsPreserveKey(&m.settingsList, settingsToItems(m.settings), settingItemKey)
			return
		}
	}
}
