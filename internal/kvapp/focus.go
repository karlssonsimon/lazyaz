package kvapp

import (
	"github.com/karlssonsimon/lazyaz/internal/appshell"
	"github.com/karlssonsimon/lazyaz/internal/cache"
	"github.com/karlssonsimon/lazyaz/internal/ui"
)

// snapshotCurrentPane saves the focused pane's cursor and filter into
// the appropriate history map so they survive navigation. The middle and
// versions panes are kind-aware and use the same history keys as
// navigation.go's snapshotMiddleColumn — two writers on different keys
// for the same column would silently lose cursor state.
func (m *Model) snapshotCurrentPane() {
	switch m.focus {
	case vaultsPane:
		if m.HasSubscription {
			m.vaultsHistory[m.CurrentSub.ID] = ui.SnapshotListState(&m.vaultsList, vaultItemKey)
		}
	case secretsPane:
		if m.hasVault {
			scope := cache.Key(m.CurrentSub.ID, m.currentVault.Name)
			m.secretsHistory[middleHistoryKey(m.kvKind, scope)] = ui.SnapshotListState(&m.secretsList, middleItemKeyForList(m.kvKind))
		}
	case versionsPane:
		if name := m.currentVersionOwnerName(); name != "" {
			m.versionsHistory[cache.Key(m.CurrentSub.ID, m.currentVault.Name, name)] = ui.SnapshotListState(&m.versionsList, versionItemKey)
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
			scope := cache.Key(m.CurrentSub.ID, m.currentVault.Name)
			ui.RestoreListState(&m.secretsList, m.secretsHistory[middleHistoryKey(m.kvKind, scope)], middleItemKeyForList(m.kvKind))
		}
	case versionsPane:
		if name := m.currentVersionOwnerName(); name != "" {
			ui.RestoreListState(&m.versionsList, m.versionsHistory[cache.Key(m.CurrentSub.ID, m.currentVault.Name, name)], versionItemKey)
		}
	}
}

// currentVersionOwnerName returns the name of the secret/cert/key whose
// versions fill the right column, or "" when nothing is selected for
// the active kind.
func (m *Model) currentVersionOwnerName() string {
	switch m.kvKind {
	case kvKindCertificates:
		if m.hasCert {
			return m.currentCert.Name
		}
	case kvKindKeys:
		if m.hasKey {
			return m.currentKey.Name
		}
	default:
		if m.hasSecret {
			return m.currentSecret.Name
		}
	}
	return ""
}

// exitPane cleans up the outgoing pane before a transition.
// Snapshots the pane's state, blurs filter inputs, and exits visual
// mode if active.
func (m *Model) exitPane() {
	m.snapshotCurrentPane()
	m.blurAllFilters()
	if m.focus == secretsPane && m.visual.Active() {
		m.visual.Stop()
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
	next := (m.focus + 1) % 4
	m.transitionTo(next)
}

func (m *Model) previousFocus() {
	prev := m.focus - 1
	if prev < 0 {
		prev = 3
	}
	m.transitionTo(prev)
}

func (m *Model) blurAllFilters() {
	m.vaultsList.FilterInput.Blur()
	m.secretsList.FilterInput.Blur()
	m.versionsList.FilterInput.Blur()
	m.kindList.FilterInput.Blur()
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

	var target *ui.List
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

	steps := ui.HalfPageStep(*target) * m.vimr.TakeCount()
	for i := 0; i < steps; i++ {
		if direction > 0 {
			target.CursorDown()
		} else {
			target.CursorUp()
		}
	}

	if m.focus == secretsPane && m.visual.Active() {
		m.refreshSecretSelectionDisplay()
	}
}

func (m Model) focusedListSettingFilter() bool {
	if target := m.listForPane(m.focus); target != nil {
		return target.SettingFilter()
	}
	return false
}

// IsTextInputActive reports whether the model is currently accepting
// free-form text input (list filter, overlay search, etc.). The parent
// tabapp uses this to suppress single-key shortcuts (quit, tab-jump 1–9,
// etc.) so they don't fire while the user is typing into a fuzzy filter.
//
// Action menu fuzzy-filters on typed characters, so it's text input even
// though it doesn't *look* like an input box.
func (m Model) IsTextInputActive() bool {
	switch m.inputMode() {
	case ModeNormal, ModeVisualLine:
		return false
	default:
		return true
	}
}

// scrollMotion applies a vim-style scroll motion to the focused list and
// reports whether the key was consumed. The `z` chord spans two
// keystrokes, so its pending state lives on the model.
func (m *Model) scrollMotion(key string) bool {
	motion := ui.HandleListMotion(m.listForPane(m.focus), m.Keymap, key, &m.vimr)
	if motion == ui.MotionChordOpen {
		m.Notify(appshell.LevelInfo, ui.ScrollChordHint)
	}
	return motion != ui.MotionNone
}
