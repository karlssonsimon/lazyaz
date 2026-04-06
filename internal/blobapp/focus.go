package blobapp

import (
	"fmt"

	"azure-storage/internal/ui"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

func (m *Model) nextFocus() {
	if m.focus == blobsPane && m.visualLineMode {
		m.visualLineMode = false
		m.visualAnchor = ""
		m.refreshItems()
	}
	m.blurAllFilters()
	count := 3
	if m.preview.open {
		count = 4
	}
	m.focus = (m.focus + 1) % count
}

func (m *Model) previousFocus() {
	if m.focus == blobsPane && m.visualLineMode {
		m.visualLineMode = false
		m.visualAnchor = ""
		m.refreshItems()
	}
	m.blurAllFilters()
	m.focus--
	if m.focus < 0 {
		m.focus = 2
		if m.preview.open {
			m.focus = 3
		}
	}
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
		m.status = fmt.Sprintf("Filter applied for %s", paneName(m.focus))
		return nil
	case containersPane:
		ui.ApplyFilterState(&m.containersList)
		m.status = fmt.Sprintf("Filter applied for %s", paneName(m.focus))
		return nil
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
		m.status = fmt.Sprintf("Visual mode on. %d in range.", len(m.visualSelectionBlobNames()))
	}
}

func (m Model) focusedListSettingFilter() bool {
	switch m.focus {
	case accountsPane:
		return m.accountsList.SettingFilter()
	case containersPane:
		return m.containersList.SettingFilter()
	default:
		return false
	}
}
