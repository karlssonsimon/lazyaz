package blobapp

import (
	"fmt"

	"azure-storage/internal/azure/blob"
	"azure-storage/internal/fuzzy"
	"azure-storage/internal/ui"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const searchInputHeight = 2 // input line + count line

func (m *Model) activateSearch() {
	m.search = blobSearch{active: true}
	if m.blobLoadAll {
		// All blobs loaded — go straight to fuzzy on m.blobs.
		m.search.stage = searchStageFuzzy
		m.search.results = m.blobs
		m.search.totalResults = len(m.blobs)
	} else {
		m.search.stage = searchStagePrefix
	}
	// Exit visual mode if active.
	if m.visualLineMode {
		m.visualLineMode = false
		m.visualAnchor = ""
		m.refreshBlobItems()
	}
}

func (m *Model) deactivateSearch() {
	m.search = blobSearch{}
	m.refreshBlobItems()
}

func (m Model) handleSearchKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	key := msg.String()

	switch {
	case key == "esc":
		m.deactivateSearch()
		return m, nil

	case key == "enter":
		return m.handleSearchEnter()

	case key == "backspace":
		return m.handleSearchBackspace()

	case key == "up", key == "down", key == "pgup", key == "pgdown",
		key == "home", key == "end":
		// Pass navigation keys to the blob list.
		var cmd tea.Cmd
		m.blobsList, cmd = m.blobsList.Update(msg)
		return m, cmd

	default:
		if len(key) == 1 && key[0] >= 32 && key[0] < 127 {
			return m.handleSearchChar(key)
		}
	}

	return m, nil
}

func (m Model) handleSearchEnter() (Model, tea.Cmd) {
	if m.search.stage == searchStagePrefix {
		if m.search.prefixQuery == "" {
			// Empty prefix — deactivate.
			m.deactivateSearch()
			return m, nil
		}
		// Lock prefix and fetch from API.
		m.search.prefixLocked = true
		m.search.fetching = true
		m.setLoading(blobsPane)
		effectivePrefix := blobSearchPrefix(m.prefix, m.search.prefixQuery)
		m.status = fmt.Sprintf("Searching blobs by prefix %q...", effectivePrefix)
		return m, tea.Batch(spinner.Tick,
			fetchSearchBlobsCmd(m.service, m.cache.blobs, m.currentAccount, m.containerName, m.prefix, m.search.prefixQuery, defaultBlobPrefixSearchLimit, false))
	}

	// In fuzzy stage — accept results and close search.
	m.deactivateSearch()
	return m, nil
}

func (m Model) handleSearchBackspace() (Model, tea.Cmd) {
	if m.search.stage == searchStageFuzzy {
		if m.search.fuzzyQuery == "" {
			if m.search.prefixLocked {
				// Fall back to prefix stage with results still loaded.
				m.search.stage = searchStagePrefix
				m.search.prefixLocked = false
				m.search.filtered = nil
				// Show all prefix results.
				m.searchRebuildItems()
				return m, nil
			}
			// No prefix to fall back to — deactivate.
			m.deactivateSearch()
			return m, nil
		}
		m.search.fuzzyQuery = m.search.fuzzyQuery[:len(m.search.fuzzyQuery)-1]
		m.applyFuzzyFilter()
		return m, nil
	}

	// Prefix stage.
	if m.search.prefixQuery == "" {
		m.deactivateSearch()
		return m, nil
	}
	m.search.prefixQuery = m.search.prefixQuery[:len(m.search.prefixQuery)-1]
	return m, nil
}

func (m Model) handleSearchChar(ch string) (Model, tea.Cmd) {
	if m.search.stage == searchStageFuzzy {
		m.search.fuzzyQuery += ch
		m.applyFuzzyFilter()
		return m, nil
	}

	// Prefix stage.
	m.search.prefixQuery += ch
	return m, nil
}

func (m *Model) applyFuzzyFilter() {
	if m.search.fuzzyQuery == "" {
		m.search.filtered = nil
		m.searchRebuildItems()
		return
	}
	m.search.filtered = fuzzy.Filter(m.search.fuzzyQuery, m.search.results, func(e blob.BlobEntry) string {
		return e.Name
	})
	m.searchRebuildItems()
}

func (m *Model) searchRebuildItems() {
	var entries []blob.BlobEntry
	if m.search.fuzzyQuery != "" && m.search.filtered != nil {
		entries = make([]blob.BlobEntry, len(m.search.filtered))
		for i, idx := range m.search.filtered {
			entries[i] = m.search.results[idx]
		}
	} else if len(m.search.results) > 0 {
		entries = m.search.results
	} else {
		entries = m.blobs
	}
	m.blobsList.SetItems(blobsToItems(entries, m.prefix, m.markedBlobs, m.visualSelectionNames()))
	ui.ClampListSelection(&m.blobsList)
}

func (m Model) handleSearchBlobsLoaded(msg blobsLoadedMsg) (Model, tea.Cmd) {
	if msg.err != nil {
		m.clearLoading()
		m.search.fetching = false
		m.lastErr = msg.err.Error()
		m.status = "Search failed"
		return m, nil
	}

	m.lastErr = ""
	m.search.results = msg.blobs
	m.search.totalResults = len(msg.blobs)
	m.searchRebuildItems()

	if msg.done {
		m.clearLoading()
		m.search.fetching = false
		// Auto-switch to fuzzy stage.
		m.search.stage = searchStageFuzzy
		effectivePrefix := blobSearchPrefix(m.prefix, m.search.prefixQuery)
		m.status = fmt.Sprintf("Found %d blobs by prefix %q", len(msg.blobs), effectivePrefix)
	}

	return m, msg.next
}

func (m Model) renderSearchInput(width int) string {
	muted := m.styles.Muted
	accent := m.styles.Accent

	var line1 string
	if m.blobLoadAll || m.search.stage == searchStageFuzzy {
		if m.search.prefixLocked && m.search.prefixQuery != "" {
			prefix := muted.Render("PREFIX: " + m.search.prefixQuery + " │ ")
			line1 = prefix + accent.Render("FZZ: "+m.search.fuzzyQuery) + muted.Render("█")
		} else {
			line1 = accent.Render("FZZ: "+m.search.fuzzyQuery) + muted.Render("█")
		}
	} else {
		line1 = accent.Render("PREFIX: "+m.search.prefixQuery) + muted.Render("█")
	}

	var line2 string
	if m.search.fetching {
		line2 = muted.Render("fetching...")
	} else if len(m.search.results) > 0 {
		total := m.search.totalResults
		showing := len(m.blobsList.Items())
		if m.search.fuzzyQuery != "" && showing != total {
			line2 = muted.Render(fmt.Sprintf("%d blobs │ showing %d", total, showing))
		} else {
			line2 = muted.Render(fmt.Sprintf("%d blobs", total))
		}
	}

	content := line1
	if line2 != "" {
		content = lipgloss.JoinVertical(lipgloss.Left, line1, line2)
	}
	return content
}
