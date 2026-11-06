package acapp

import (
	"github.com/karlssonsimon/lazyaz/internal/appshell"
	"github.com/karlssonsimon/lazyaz/internal/ui"
)

func (m *Model) snapshotCurrentPane() {
	switch m.focus {
	case storesPane:
		if m.HasSubscription {
			m.storesHistory[m.CurrentSub.ID] = ui.SnapshotListState(&m.storesList, storeItemKey)
		}
	case settingsPane:
		if m.hasStore {
			m.settingsHistory[settingsScope(m.currentStore)] = ui.SnapshotListState(&m.settingsList, settingItemKey)
		}
	}
}

func (m *Model) restoreCurrentPane() {
	switch m.focus {
	case storesPane:
		if m.HasSubscription {
			ui.RestoreListState(&m.storesList, m.storesHistory[m.CurrentSub.ID], storeItemKey)
		}
	case settingsPane:
		if m.hasStore {
			ui.RestoreListState(&m.settingsList, m.settingsHistory[settingsScope(m.currentStore)], settingItemKey)
		}
	}
}

func (m *Model) exitPane() {
	m.snapshotCurrentPane()
	m.blurAllFilters()
}

// transitionTo is the single codepath for focus changes.
func (m *Model) transitionTo(pane int) {
	m.exitPane()
	m.focus = pane
	m.resize()
	m.restoreCurrentPane()
}

// navigablePanes is the ordered set of panes focus can cycle through:
// the settings column once a store is open, the value pane while a
// value is shown.
func (m Model) navigablePanes() []int {
	panes := []int{storesPane}
	if m.hasStore {
		panes = append(panes, settingsPane)
	}
	if m.viewingValue {
		panes = append(panes, valuePane)
	}
	return panes
}

func (m *Model) nextFocus() {
	panes := m.navigablePanes()
	next := panes[0]
	for i, p := range panes {
		if p == m.focus {
			next = panes[(i+1)%len(panes)]
			break
		}
	}
	m.transitionTo(next)
}

func (m *Model) previousFocus() {
	panes := m.navigablePanes()
	prev := panes[len(panes)-1]
	for i, p := range panes {
		if p == m.focus {
			prev = panes[(i-1+len(panes))%len(panes)]
			break
		}
	}
	m.transitionTo(prev)
}

func (m *Model) blurAllFilters() {
	m.storesList.FilterInput.Blur()
	m.settingsList.FilterInput.Blur()
}

func (m *Model) commitFocusedFilter() {
	m.blurAllFilters()
	if l := m.listForPane(m.focus); l != nil {
		ui.ApplyFilterState(l)
	}
}

func (m *Model) scrollFocusedHalfPage(direction int) {
	target := m.listForPane(m.focus)
	if direction == 0 || target == nil {
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
}

func (m Model) focusedListSettingFilter() bool {
	if target := m.listForPane(m.focus); target != nil {
		return target.SettingFilter()
	}
	return false
}

// IsTextInputActive reports whether the model is accepting free-form
// text, so the parent suppresses single-key shortcuts.
func (m Model) IsTextInputActive() bool {
	return m.inputMode() != ModeNormal
}

// scrollMotion applies a vim scroll motion to the focused list.
func (m *Model) scrollMotion(key string) bool {
	l := m.listForPane(m.focus)
	if l == nil {
		return false
	}
	motion := ui.HandleListMotion(l, m.Keymap, key, &m.vimr)
	if motion == ui.MotionChordOpen {
		m.Notify(appshell.LevelInfo, ui.ScrollChordHint)
	}
	return motion != ui.MotionNone
}
