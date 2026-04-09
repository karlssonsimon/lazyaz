package blobapp

import (
	"fmt"

	"github.com/karlssonsimon/lazyaz/internal/appshell"
	"github.com/karlssonsimon/lazyaz/internal/ui"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
)

func (m *Model) setFocus(pane int) {
	m.focus = pane
	m.resize()
}

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
	m.resize()
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
	m.resize()
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
	return m.focusedListSettingFilter() || (m.focus == blobsPane && m.filter.inputOpen) || m.SubOverlay.Active || m.sortOverlay.active
}
