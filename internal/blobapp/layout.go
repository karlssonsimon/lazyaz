package blobapp

import (
	"fmt"

	"azure-storage/internal/ui"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

func (m *Model) resize() {
	if m.width <= 0 || m.height <= 0 {
		return
	}

	numPanes := 3
	if m.preview.open {
		numPanes = 4
	}
	widths := ui.PaneLayout(m.styles.Chrome.Pane, m.width, numPanes)
	pane := m.styles.Chrome.Pane
	m.paneWidths = [4]int{widths[0], widths[1], widths[2], 0}
	if m.preview.open {
		m.paneWidths[3] = widths[3]
	}

	paneFrame := 2 // rounded border top + bottom
	height := m.height - paneFrame - ui.StatusBarHeight - ui.SubscriptionBarHeight
	if height < 8 {
		height = 8
	}
	m.paneHeight = height

	listHeight := height - ui.PaneHintHeight
	m.accountsList.SetSize(ui.PaneContentWidth(pane, widths[0]), listHeight)
	m.containersList.SetSize(ui.PaneContentWidth(pane, widths[1]), listHeight)
	blobListHeight := listHeight
	if m.search.active {
		blobListHeight -= searchInputHeight
	}
	m.blobsList.SetSize(ui.PaneContentWidth(pane, widths[2]), blobListHeight)
	if m.preview.open {
		m.preview.viewport.Width = ui.PaneContentWidth(pane, widths[3])
		m.preview.viewport.Height = listHeight
	}
}

func (m *Model) nextFocus() {
	if m.focus == blobsPane && m.visualLineMode {
		m.visualLineMode = false
		m.visualAnchor = ""
		m.refreshBlobItems()
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
		m.refreshBlobItems()
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
		m.refreshBlobItems()
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
