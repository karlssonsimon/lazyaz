package acapp

import (
	"github.com/karlssonsimon/lazyaz/internal/azure/appconfig"
	"github.com/karlssonsimon/lazyaz/internal/ui"

	tea "charm.land/bubbletea/v2"
)

// PendingNav is a navigation target for the tab to land on. Empty
// StoreName means no nav. SettingKey (appconfig.SettingKey form) opens
// that value; SelectKey only places the settings cursor.
type PendingNav struct {
	StoreName  string
	SettingKey string
	SelectKey  string
}

func (p PendingNav) hasTarget() bool { return p.StoreName != "" }

// SetPendingNav records the intent and fast-forwards through cached
// layers so the user lands on the destination without watching staged
// fetches: each step that lands from cache lets the next run at once;
// a step that needs a fetch leaves the rest to the load handlers.
func (m *Model) SetPendingNav(p PendingNav) tea.Cmd {
	m.pendingNav = p
	var cmds []tea.Cmd
	for i := 0; i < 3 && m.pendingNav.hasTarget(); i++ {
		before := m.navPosition()
		updated, cmd := m.advancePendingNav()
		*m = updated
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
		if m.navPosition() == before {
			break
		}
	}
	return batchNavCmds(cmds)
}

// navPosition summarises where the walk stands, so SetPendingNav can
// tell a step that moved from one that is waiting on a fetch.
func (m Model) navPosition() string {
	pos := ""
	if m.hasStore {
		pos += m.currentStore.Name
	}
	if m.viewingValue {
		pos += "\x00" + appconfig.SettingKey(m.currentSetting)
	}
	return pos
}

// advancePendingNav walks one step toward the target. Called after
// each load lands and once eagerly from SetPendingNav.
func (m Model) advancePendingNav() (Model, tea.Cmd) {
	if !m.pendingNav.hasTarget() || !m.HasSubscription {
		return m, nil
	}
	target := m.pendingNav

	if !m.hasStore || m.currentStore.Name != target.StoreName {
		for _, s := range m.stores {
			if s.Name == target.StoreName {
				return m.selectStore(s)
			}
		}
		if len(m.stores) > 0 {
			m.pendingNav = PendingNav{} // loaded, but the target is gone
		}
		return m, nil
	}

	if target.SettingKey == "" {
		if target.SelectKey != "" {
			ui.SelectByKey(&m.settingsList, target.SelectKey, settingItemKey)
		}
		m.pendingNav = PendingNav{}
		return m, nil
	}
	if len(m.settings) == 0 {
		return m, nil // wait for the settings load
	}
	for _, s := range m.settings {
		if appconfig.SettingKey(s) == target.SettingKey {
			updated, cmd := m.openValue(s)
			updated.pendingNav = PendingNav{}
			return updated, cmd
		}
	}
	m.pendingNav = PendingNav{}
	return m, nil
}

func batchNavCmds(cmds []tea.Cmd) tea.Cmd {
	switch len(cmds) {
	case 0:
		return nil
	case 1:
		return cmds[0]
	}
	return tea.Batch(cmds...)
}
