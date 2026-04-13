package blobapp

import (
	"fmt"

	"github.com/karlssonsimon/lazyaz/internal/appshell"
	"github.com/karlssonsimon/lazyaz/internal/cache"
	"github.com/karlssonsimon/lazyaz/internal/ui"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
)

// snapshotCurrentPane saves the focused pane's cursor and filter into
// the appropriate history map so they survive navigation.
func (m *Model) snapshotCurrentPane() {
	switch m.focus {
	case accountsPane:
		if m.HasSubscription {
			m.accountsHistory[m.CurrentSub.ID] = ui.SnapshotListState(&m.accountsList, accountItemKey)
		}
	case containersPane:
		if m.hasAccount {
			m.containersHistory[cache.Key(m.CurrentSub.ID, m.currentAccount.Name)] = ui.SnapshotListState(&m.containersList, containerItemKey)
		}
	case blobsPane:
		if m.hasContainer {
			m.blobsHistory[blobsCacheKey(m.CurrentSub.ID, m.currentAccount.Name, m.containerName, m.prefix, m.blobLoadAll)] = ui.SnapshotListState(&m.blobsList, blobItemKey)
		}
	}
}

// restoreCurrentPane re-applies saved cursor and filter for the focused
// pane from the history map, ensuring filters survive transitions.
func (m *Model) restoreCurrentPane() {
	switch m.focus {
	case accountsPane:
		if m.HasSubscription {
			ui.RestoreListState(&m.accountsList, m.accountsHistory[m.CurrentSub.ID], accountItemKey)
		}
	case containersPane:
		if m.hasAccount {
			ui.RestoreListState(&m.containersList, m.containersHistory[cache.Key(m.CurrentSub.ID, m.currentAccount.Name)], containerItemKey)
		}
	case blobsPane:
		if m.hasContainer {
			ui.RestoreListState(&m.blobsList, m.blobsHistory[blobsCacheKey(m.CurrentSub.ID, m.currentAccount.Name, m.containerName, m.prefix, m.blobLoadAll)], blobItemKey)
		}
	}
}

// exitPane cleans up the outgoing pane before a transition.
// Snapshots the pane's state, blurs filter inputs, and exits visual
// mode if active. When clearFilter is true, the blobFilter prefix
// search is also cleared (used for prefix scope changes).
func (m *Model) exitPane(clearFilter bool) {
	m.snapshotCurrentPane()
	m.blurAllFilters()
	if m.focus == blobsPane && m.visualLineMode {
		m.visualLineMode = false
		m.visualAnchor = ""
		m.refreshItems()
	}
	if clearFilter && m.focus == blobsPane {
		m.clearFilter()
	}
}

// transitionTo performs exitPane cleanup on the current pane, then sets
// focus to the target pane and restores its saved state. This is the
// single codepath for all focus changes, guaranteeing that filters and
// cursor positions survive navigation.
func (m *Model) transitionTo(pane int, clearFilter bool) {
	m.exitPane(clearFilter)
	m.focus = pane
	m.resize()
	m.restoreCurrentPane()
}

func (m *Model) nextFocus() {
	count := 3
	if m.preview.open {
		count = 4
	}
	next := (m.focus + 1) % count
	m.transitionTo(next, false)
}

func (m *Model) previousFocus() {
	prev := m.focus - 1
	if prev < 0 {
		prev = 2
		if m.preview.open {
			prev = 3
		}
	}
	m.transitionTo(prev, false)
}

func (m *Model) blurAllFilters() {
	m.accountsList.FilterInput.Blur()
	m.containersList.FilterInput.Blur()
	m.blobsList.FilterInput.Blur()
}

func (m *Model) commitFocusedFilter() tea.Cmd {
	m.blurAllFilters()

	switch m.focus {
	case accountsPane:
		ui.ApplyFilterState(&m.accountsList)
		m.Notify(appshell.LevelInfo, fmt.Sprintf("Filter applied for %s", paneName(m.focus)))
	case containersPane:
		ui.ApplyFilterState(&m.containersList)
		m.Notify(appshell.LevelInfo, fmt.Sprintf("Filter applied for %s", paneName(m.focus)))
	case blobsPane:
		ui.ApplyFilterState(&m.blobsList)
		m.Notify(appshell.LevelInfo, fmt.Sprintf("Filter applied for %s", paneName(m.focus)))
	}

	return nil
}

func (m *Model) scrollFocusedHalfPage(direction int) {
	if direction == 0 {
		return
	}

	var target *list.Model
	switch m.focus {
	case accountsPane:
		target = &m.accountsList
	case containersPane:
		target = &m.containersList
	case blobsPane:
		target = &m.blobsList
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

	if m.focus == blobsPane && m.visualLineMode {
		m.refreshItems()
		m.Notify(appshell.LevelInfo, fmt.Sprintf("Visual mode on. %d in range.", len(m.visualSelectionBlobNames())))
	}
}

func (m Model) focusedListSettingFilter() bool {
	switch m.focus {
	case accountsPane:
		return m.accountsList.SettingFilter()
	case containersPane:
		return m.containersList.SettingFilter()
	case blobsPane:
		return m.blobsList.SettingFilter()
	default:
		return false
	}
}

// IsTextInputActive reports whether the model is currently accepting
// free-form text input (list filter, search bar, overlay search, etc.).
// The parent tabapp uses this to suppress single-key shortcuts like quit.
func (m Model) IsTextInputActive() bool {
	switch m.inputMode() {
	case ModeNormal, ModeVisualLine:
		return false
	default:
		return true
	}
}
