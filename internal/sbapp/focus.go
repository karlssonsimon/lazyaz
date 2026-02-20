package sbapp

import (
	"azure-storage/internal/ui"

	"github.com/charmbracelet/bubbles/list"
)

func (m *Model) nextFocus() {
	m.blurAllFilters()
	m.focus = (m.focus + 1) % 4
}

func (m *Model) previousFocus() {
	m.blurAllFilters()
	m.focus--
	if m.focus < 0 {
		m.focus = 3
	}
}

func (m *Model) blurAllFilters() {
	m.subscriptionsList.FilterInput.Blur()
	m.namespacesList.FilterInput.Blur()
	m.entitiesList.FilterInput.Blur()
	m.detailList.FilterInput.Blur()
}

func (m *Model) commitFocusedFilter() {
	m.blurAllFilters()

	switch m.focus {
	case subscriptionsPane:
		ui.ApplyFilterState(&m.subscriptionsList)
	case namespacesPane:
		ui.ApplyFilterState(&m.namespacesList)
	case entitiesPane:
		ui.ApplyFilterState(&m.entitiesList)
	case detailPane:
		ui.ApplyFilterState(&m.detailList)
	}
}

func (m *Model) scrollFocusedHalfPage(direction int) {
	if direction == 0 {
		return
	}

	var target *list.Model
	switch m.focus {
	case subscriptionsPane:
		target = &m.subscriptionsList
	case namespacesPane:
		target = &m.namespacesList
	case entitiesPane:
		target = &m.entitiesList
	case detailPane:
		target = &m.detailList
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
	case subscriptionsPane:
		return m.subscriptionsList.SettingFilter()
	case namespacesPane:
		return m.namespacesList.SettingFilter()
	case entitiesPane:
		return m.entitiesList.SettingFilter()
	case detailPane:
		return m.detailList.SettingFilter()
	default:
		return false
	}
}
