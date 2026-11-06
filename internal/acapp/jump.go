package acapp

import (
	"strings"

	"github.com/karlssonsimon/lazyaz/internal/azure/appconfig"
	"github.com/karlssonsimon/lazyaz/internal/jumplist"
	"github.com/karlssonsimon/lazyaz/internal/ui"

	tea "charm.land/bubbletea/v2"
)

// acNavSnapshot is the user's position: subscription, store, the
// opened setting, the selected row and the focused pane.
type acNavSnapshot struct {
	subscriptionID string
	storeName      string
	settingKey     string // SettingKey of the opened value, "" when none
	itemKey        string // selected row in the focused pane
	focusedPane    int
}

func (s acNavSnapshot) Description() string {
	parts := []string{"appconfig"}
	if s.storeName != "" {
		parts = append(parts, s.storeName)
	}
	if s.settingKey != "" {
		parts = append(parts, strings.ReplaceAll(s.settingKey, "\x00", "@"))
	}
	parts = append(parts, paneName(s.focusedPane))
	return strings.Join(parts, " / ")
}

// CurrentNav captures the active position; nil without a subscription.
func (m Model) CurrentNav() jumplist.NavSnapshot {
	if !m.HasSubscription {
		return nil
	}
	snap := acNavSnapshot{subscriptionID: m.CurrentSub.ID, focusedPane: m.focus}
	if m.hasStore {
		snap.storeName = m.currentStore.Name
	}
	if m.viewingValue {
		snap.settingKey = appconfig.SettingKey(m.currentSetting)
	}
	switch m.focus {
	case storesPane:
		snap.itemKey = storeItemKey(m.storesList.SelectedItem())
	case settingsPane, valuePane:
		snap.itemKey = settingItemKey(m.settingsList.SelectedItem())
	}
	return snap
}

// ApplyNav restores a captured position without recording jumps.
func (m *Model) ApplyNav(snap jumplist.NavSnapshot) tea.Cmd {
	s, ok := snap.(acNavSnapshot)
	if !ok {
		return nil
	}
	m.applyingNav = true
	defer func() { m.applyingNav = false }()

	var cmds []tea.Cmd
	if s.subscriptionID != "" && (!m.HasSubscription || m.CurrentSub.ID != s.subscriptionID) {
		found := false
		for _, sub := range m.Subscriptions {
			if sub.ID == s.subscriptionID {
				updated, cmd := m.selectSubscription(sub)
				*m = updated
				if cmd != nil {
					cmds = append(cmds, cmd)
				}
				found = true
				break
			}
		}
		if !found {
			return nil
		}
	}

	if s.storeName == "" {
		m.transitionTo(storesPane)
		ui.SelectByKey(&m.storesList, s.itemKey, storeItemKey)
		return batchNavCmds(cmds)
	}

	nav := PendingNav{StoreName: s.storeName, SettingKey: s.settingKey}
	if nav.SettingKey == "" {
		nav.SelectKey = s.itemKey
	}
	if cmd := m.SetPendingNav(nav); cmd != nil {
		cmds = append(cmds, cmd)
	}
	if s.focusedPane >= storesPane && s.focusedPane <= valuePane {
		if s.focusedPane == valuePane && !m.viewingValue {
			m.transitionTo(settingsPane)
		} else {
			m.transitionTo(s.focusedPane)
		}
	}
	if s.focusedPane != storesPane {
		ui.SelectByKey(&m.settingsList, s.itemKey, settingItemKey)
	}
	return batchNavCmds(cmds)
}

func (m Model) WithAppliedNav(snap jumplist.NavSnapshot) (tea.Model, tea.Cmd) {
	cmd := m.ApplyNav(snap)
	return m, cmd
}

// recordDeparture batches cmd with a jump record for the position being
// left, unless a restore or pending navigation is in flight.
func recordDeparture(m Model, depart jumplist.NavSnapshot, cmd tea.Cmd) tea.Cmd {
	return jumplist.AppendRecord(m.applyingNav, m.pendingNav.hasTarget(), depart, cmd)
}

// StrictAncestorOf reports whether s sits strictly above other on the
// drill path (subscription → store → setting).
func (s acNavSnapshot) StrictAncestorOf(other jumplist.NavSnapshot) bool {
	o, ok := other.(acNavSnapshot)
	if !ok || s.subscriptionID != o.subscriptionID {
		return false
	}
	if s.storeName != "" && s.storeName != o.storeName {
		return false
	}
	if s.settingKey != "" && s.settingKey != o.settingKey {
		return false
	}
	sameScope := s.storeName == o.storeName && s.settingKey == o.settingKey
	return !sameScope
}
