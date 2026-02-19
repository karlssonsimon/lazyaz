package blobapp

import (
	"fmt"
	"strings"

	ui "azure-storage/internal/ui"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
)

func (m *Model) resize() {
	if m.width <= 0 || m.height <= 0 {
		return
	}

	sub := m.width / 5
	acc := m.width / 5
	con := m.width / 5
	if sub < 24 {
		sub = 24
	}
	if acc < 24 {
		acc = 24
	}
	if con < 24 {
		con = 24
	}
	marginBudget := 12
	if m.preview.open {
		marginBudget = 14
	}
	blob := m.width - sub - acc - con - marginBudget
	preview := 0
	if m.preview.open {
		preview = blob / 2
		blob = blob - preview
	}
	if blob < 40 {
		blob = 40
	}
	if m.preview.open && preview < 40 {
		preview = 40
	}

	height := m.height - 10
	if height < 8 {
		height = 8
	}

	m.subscriptionsList.SetSize(sub, height)
	m.accountsList.SetSize(acc, height)
	m.containersList.SetSize(con, height)
	m.blobsList.SetSize(blob, height)
	if m.preview.open {
		m.preview.viewport.Width = preview
		m.preview.viewport.Height = height
	}
}

func (m *Model) nextFocus() {
	if m.focus == blobsPane && m.visualLineMode {
		m.visualLineMode = false
		m.visualAnchor = ""
		m.refreshBlobItems()
	}
	m.blurAllFilters()
	count := 4
	if m.preview.open {
		count = 5
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
		m.focus = 3
		if m.preview.open {
			m.focus = 4
		}
	}
}

func (m *Model) blurAllFilters() {
	m.subscriptionsList.FilterInput.Blur()
	m.accountsList.FilterInput.Blur()
	m.containersList.FilterInput.Blur()
	m.blobsList.FilterInput.Blur()
}

func (m *Model) commitFocusedFilter() tea.Cmd {
	m.blurAllFilters()

	switch m.focus {
	case subscriptionsPane:
		ui.ApplyFilterState(&m.subscriptionsList)
		m.status = fmt.Sprintf("Filter applied for %s", paneName(m.focus))
		return nil
	case accountsPane:
		ui.ApplyFilterState(&m.accountsList)
		m.status = fmt.Sprintf("Filter applied for %s", paneName(m.focus))
		return nil
	case containersPane:
		ui.ApplyFilterState(&m.containersList)
		m.status = fmt.Sprintf("Filter applied for %s", paneName(m.focus))
		return nil
	case blobsPane:
		if !m.hasContainer {
			m.status = "Open a container before searching blobs"
			return nil
		}

		if m.blobLoadAll {
			ui.ApplyFilterState(&m.blobsList)
			m.status = "Filter applied for blobs"
			return nil
		}

		query := strings.TrimSpace(m.blobsList.FilterValue())
		if query == "" {
			m.blobsList.ResetFilter()
			m.blobSearchQuery = ""
			m.loading = true
			m.status = fmt.Sprintf("Loading up to %d entries under %q", defaultHierarchyBlobLoadLimit, m.prefix)
			return tea.Batch(spinner.Tick, loadHierarchyBlobsCmd(m.service, m.currentAccount, m.containerName, m.prefix, defaultHierarchyBlobLoadLimit))
		}

		m.blobSearchQuery = query
		m.loading = true
		m.status = fmt.Sprintf("Searching blobs by prefix %q...", blobSearchPrefix(m.prefix, query))
		return tea.Batch(spinner.Tick, searchBlobsByPrefixCmd(m.service, m.currentAccount, m.containerName, m.prefix, query, defaultBlobPrefixSearchLimit))
	}

	return nil
}

func (m *Model) scrollFocusedHalfPage(direction int) {
	if direction == 0 {
		return
	}

	var target *list.Model
	switch m.focus {
	case subscriptionsPane:
		target = &m.subscriptionsList
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
	case subscriptionsPane:
		return m.subscriptionsList.SettingFilter()
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
