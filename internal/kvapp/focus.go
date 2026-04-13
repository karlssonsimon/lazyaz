package kvapp

import (
	"github.com/karlssonsimon/lazyaz/internal/cache"
	"github.com/karlssonsimon/lazyaz/internal/ui"

	"charm.land/bubbles/v2/list"
)

// snapshotCurrentPane saves the focused pane's cursor and filter into
// the appropriate history map so they survive navigation.
func (m *Model) snapshotCurrentPane() {
	switch m.focus {
	case vaultsPane:
		if m.HasSubscription {
			m.vaultsHistory[m.CurrentSub.ID] = ui.SnapshotListState(&m.vaultsList, vaultItemKey)
		}
	case secretsPane:
		if m.hasVault {
			m.secretsHistory[cache.Key(m.CurrentSub.ID, m.currentVault.Name)] = ui.SnapshotListState(&m.secretsList, secretItemKey)
		}
	case versionsPane:
		if m.hasSecret {
			m.versionsHistory[cache.Key(m.CurrentSub.ID, m.currentVault.Name, m.currentSecret.Name)] = ui.SnapshotListState(&m.versionsList, versionItemKey)
		}
	}
}

// restoreCurrentPane re-applies saved cursor and filter for the focused
// pane from the history map, ensuring filters survive transitions.
func (m *Model) restoreCurrentPane() {
	switch m.focus {
	case vaultsPane:
		if m.HasSubscription {
			ui.RestoreListState(&m.vaultsList, m.vaultsHistory[m.CurrentSub.ID], vaultItemKey)
		}
	case secretsPane:
		if m.hasVault {
			ui.RestoreListState(&m.secretsList, m.secretsHistory[cache.Key(m.CurrentSub.ID, m.currentVault.Name)], secretItemKey)
		}
	case versionsPane:
		if m.hasSecret {
			ui.RestoreListState(&m.versionsList, m.versionsHistory[cache.Key(m.CurrentSub.ID, m.currentVault.Name, m.currentSecret.Name)], versionItemKey)
		}
	}
}

// exitPane cleans up the outgoing pane before a transition.
// Snapshots the pane's state, blurs filter inputs, and exits visual
// mode if active.
func (m *Model) exitPane() {
	m.snapshotCurrentPane()
	m.blurAllFilters()
	if m.focus == secretsPane && m.visualLineMode {
		m.visualLineMode = false
		m.visualAnchor = ""
		m.refreshSecretItems()
	}
}

// transitionTo performs exitPane cleanup on the current pane, then sets
// focus to the target pane and restores its saved state. This is the
// single codepath for all focus changes, guaranteeing that filters and
// cursor positions survive navigation.
func (m *Model) transitionTo(pane int) {
	m.exitPane()
	m.focus = pane
	m.resize()
	m.restoreCurrentPane()
}

func (m *Model) nextFocus() {
	next := (m.focus + 1) % 3
	m.transitionTo(next)
}

func (m *Model) previousFocus() {
	prev := m.focus - 1
	if prev < 0 {
		prev = 2
	}
	m.transitionTo(prev)
}

func (m *Model) blurAllFilters() {
	m.vaultsList.FilterInput.Blur()
	m.secretsList.FilterInput.Blur()
	m.versionsList.FilterInput.Blur()
}

func (m *Model) commitFocusedFilter() {
	m.blurAllFilters()

	switch m.focus {
	case vaultsPane:
		ui.ApplyFilterState(&m.vaultsList)
	case secretsPane:
		ui.ApplyFilterState(&m.secretsList)
	case versionsPane:
		ui.ApplyFilterState(&m.versionsList)
	}
}

func (m *Model) scrollFocusedHalfPage(direction int) {
	if direction == 0 {
		return
	}

	var target *list.Model
	switch m.focus {
	case vaultsPane:
		target = &m.vaultsList
	case secretsPane:
		target = &m.secretsList
	case versionsPane:
		target = &m.versionsList
	default:
		return
	}

	steps := ui.HalfPageStep(*target)
	for i := 0; i < steps; i++ {
		if direction > 0 {
			target.CursorDown()
		} else {
			target.CursorUp()
		}
	}
}

func (m Model) focusedListSettingFilter() bool {
	switch m.focus {
	case vaultsPane:
		return m.vaultsList.SettingFilter()
	case secretsPane:
		return m.secretsList.SettingFilter()
	case versionsPane:
		return m.versionsList.SettingFilter()
	default:
		return false
	}
}

// IsTextInputActive reports whether the model is currently accepting
// free-form text input (list filter, overlay search, etc.). The parent
// tabapp uses this to suppress single-key shortcuts like quit.
func (m Model) IsTextInputActive() bool {
	switch m.inputMode() {
	case ModeNormal, ModeVisualLine:
		return false
	default:
		return true
	}
}
