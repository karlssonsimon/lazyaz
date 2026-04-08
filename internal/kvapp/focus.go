package kvapp

import (
	"github.com/karlssonsimon/lazyaz/internal/ui"

	"charm.land/bubbles/v2/list"
)

func (m *Model) nextFocus() {
	m.blurAllFilters()
	m.focus = (m.focus + 1) % 3
}

func (m *Model) previousFocus() {
	m.blurAllFilters()
	m.focus--
	if m.focus < 0 {
		m.focus = 2
	}
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
