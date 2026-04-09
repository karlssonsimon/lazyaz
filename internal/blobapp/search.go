package blobapp

import (
	"fmt"
	"strings"

	"github.com/karlssonsimon/lazyaz/internal/appshell"
	"github.com/karlssonsimon/lazyaz/internal/azure/blob"
	"github.com/karlssonsimon/lazyaz/internal/fuzzy"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// filterInputHeight returns the number of rows the filter input occupies.
func (m *Model) filterInputHeight() int {
	h := 1 // fuzzy input always shown
	if !m.blobLoadAll {
		h++ // prefix input
	}
	if m.filter.fetching || len(m.filter.apiResults) > 0 {
		h++ // count line
	}
	h++ // entry count (replaces hidden status bar)
	return h
}

// hasActiveFilter reports whether any filter query is set.
func (m *Model) hasActiveFilter() bool {
	return m.filter.prefixQuery != "" || m.filter.fuzzyQuery != ""
}

// openFilterInput opens the filter input UI. If a filter is already
// active, the existing queries are preserved for editing.
func (m *Model) openFilterInput() tea.Cmd {
	m.filter.inputOpen = true
	if m.blobLoadAll {
		m.filter.focusedInput = searchInputFuzzy
		if !m.filter.prefixFetched {
			m.filter.apiResults = m.blobs
			m.filter.apiCount = len(m.blobs)
			m.filter.prefixFetched = true
		}
	} else if m.filter.prefixQuery == "" && m.filter.fuzzyQuery == "" {
		m.filter.focusedInput = searchInputPrefix
	}
	// Exit visual mode if active.
	if m.visualLineMode {
		m.visualLineMode = false
		m.visualAnchor = ""
		m.refreshItems()
	}
	m.resize()
	return m.Cursor.Focus()
}

// closeFilterInput closes the filter input UI but keeps the filter applied.
func (m *Model) closeFilterInput() {
	m.filter.inputOpen = false
	m.Cursor.Blur()
	m.resize()
}

// clearFilter removes all filter state and restores the unfiltered view.
func (m *Model) clearFilter() {
	m.filter = blobFilter{}
	m.resize()
	m.refreshItems()
}

func (m Model) handleFilterKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	key := msg.String()

	switch {
	case m.Keymap.Cancel.Matches(key):
		m.closeFilterInput()
		return m, nil

	case m.Keymap.NextFocus.Matches(key), key == "up", key == "down":
		if !m.blobLoadAll {
			return m.switchFilterInput()
		}
		return m, nil

	case m.Keymap.OpenFocused.Matches(key):
		return m.handleFilterEnter()

	case m.Keymap.BackspaceUp.Matches(key):
		return m.handleFilterBackspace()

	case key == "pgup", key == "pgdown", key == "home", key == "end":
		var cmd tea.Cmd
		m.blobsList, cmd = m.blobsList.Update(msg)
		return m, cmd

	default:
		if len(key) == 1 && key[0] >= 32 && key[0] < 127 {
			return m.handleFilterChar(key)
		}
	}

	return m, nil
}

// switchFilterInput toggles focus between prefix and fuzzy inputs.
// When leaving the prefix field, auto-fires the API search.
func (m Model) switchFilterInput() (Model, tea.Cmd) {
	var cmd tea.Cmd
	if m.filter.focusedInput == searchInputPrefix {
		if m.filter.prefixQuery != "" && !m.filter.fetching {
			cmd = m.firePrefixSearch()
		}
		m.filter.focusedInput = searchInputFuzzy
	} else {
		m.filter.focusedInput = searchInputPrefix
	}
	return m, cmd
}

func (m Model) handleFilterEnter() (Model, tea.Cmd) {
	if m.filter.focusedInput == searchInputPrefix {
		if m.filter.prefixQuery == "" {
			m.closeFilterInput()
			return m, nil
		}
		cmd := m.firePrefixSearch()
		return m, cmd
	}

	// Fuzzy input — close input, keep filter applied.
	m.closeFilterInput()
	return m, nil
}

// firePrefixSearch fires an API prefix search and returns the command.
func (m *Model) firePrefixSearch() tea.Cmd {
	m.filter.fetching = true
	m.filter.prefixFetched = false
	m.filter.apiResults = nil
	m.filter.apiCount = 0
	m.filter.filtered = nil
	m.resize() // count line may have appeared (fetching...)
	m.refreshItems() // clear the list immediately
	m.SetLoading(blobsPane)
	effectivePrefix := blobSearchPrefix(m.prefix, m.filter.prefixQuery)
	m.Status = fmt.Sprintf("Searching blobs by prefix %q...", effectivePrefix)
	return tea.Batch(m.Spinner.Tick,
		fetchSearchBlobsCmd(m.service, m.currentAccount, m.containerName, m.prefix, m.filter.prefixQuery, defaultBlobPrefixSearchLimit))
}

func (m Model) handleFilterBackspace() (Model, tea.Cmd) {
	if m.filter.focusedInput == searchInputFuzzy {
		if m.filter.fuzzyQuery == "" {
			return m, nil
		}
		m.filter.fuzzyQuery = m.filter.fuzzyQuery[:len(m.filter.fuzzyQuery)-1]
		m.applyFuzzyFilter()
		return m, nil
	}

	// Prefix input.
	if m.filter.prefixQuery == "" {
		m.closeFilterInput()
		return m, nil
	}
	m.filter.prefixQuery = m.filter.prefixQuery[:len(m.filter.prefixQuery)-1]
	return m, nil
}

func (m Model) handleFilterChar(ch string) (Model, tea.Cmd) {
	if m.filter.focusedInput == searchInputFuzzy {
		m.filter.fuzzyQuery += ch
		m.applyFuzzyFilter()
		return m, nil
	}

	// Prefix input.
	m.filter.prefixQuery += ch
	return m, nil
}

func (m *Model) applyFuzzyFilter() {
	source := m.filterSource()
	if m.filter.fuzzyQuery == "" {
		m.filter.filtered = nil
	} else {
		m.filter.filtered = fuzzy.Filter(m.filter.fuzzyQuery, source, func(e blob.BlobEntry) string {
			return e.Name
		})
	}
	m.refreshItems()
}

// filterSource returns the blob slice that fuzzy filtering operates on.
// When a prefix search is active (query set), only API results are used —
// if none have arrived yet, an empty slice is returned so stale hierarchy
// data is never shown.
func (m *Model) filterSource() []blob.BlobEntry {
	if m.filter.prefixQuery != "" {
		return m.filter.apiResults // nil/empty while fetching — intentional
	}
	return m.blobs
}

// displayBlobs computes the final display set: source → fuzzy filter → sort.
// This is called by refreshItems() and never modifies m.blobs.
func (m *Model) displayBlobs() []blob.BlobEntry {
	source := m.filterSource()

	if m.filter.fuzzyQuery != "" && m.filter.filtered != nil {
		entries := make([]blob.BlobEntry, len(m.filter.filtered))
		for i, idx := range m.filter.filtered {
			entries[i] = source[idx]
		}
		source = entries
	}

	return sortBlobs(source, m.blobSortField, m.blobSortDesc)
}

func (m Model) handleFilterBlobsLoaded(msg blobsLoadedMsg) (Model, tea.Cmd) {
	if msg.err != nil {
		m.ClearLoading()
		m.filter.fetching = false
		m.Notify(appshell.LevelError, fmt.Sprintf("Search failed: %s", msg.err.Error()))
		return m, nil
	}

	m.LastErr = ""
	m.filter.apiResults = msg.blobs
	m.filter.apiCount = len(msg.blobs)
	m.resize() // count line may have appeared
	m.applyFuzzyFilter()

	if msg.done {
		m.ClearLoading()
		m.filter.fetching = false
		m.filter.prefixFetched = true
		effectivePrefix := blobSearchPrefix(m.prefix, m.filter.prefixQuery)
		m.Status = fmt.Sprintf("Found %d blobs by prefix %q", len(msg.blobs), effectivePrefix)
	}

	return m, msg.next
}

// renderFilterBanner renders a one-line summary of the active filter,
// shown when the input is closed but a filter is applied.
func (m Model) renderFilterBanner() string {
	muted := m.Styles.Muted
	accent2 := m.Styles.Accent2
	accent := m.Styles.Accent

	var parts []string
	if m.filter.prefixQuery != "" {
		p := accent2.Render("API") + muted.Render(": "+m.filter.prefixQuery)
		if m.filter.apiCount > 0 {
			p += muted.Render(fmt.Sprintf(" → %d", m.filter.apiCount))
		}
		parts = append(parts, p)
	}
	if m.filter.fuzzyQuery != "" {
		showing := len(m.blobsList.Items())
		p := accent.Render("fuzzy") + muted.Render(fmt.Sprintf(": %s → %d", m.filter.fuzzyQuery, showing))
		parts = append(parts, p)
	}
	sep := muted.Render(" · ")
	banner := strings.Join(parts, sep)
	hint := muted.Render("   / edit · esc clear")
	return " " + banner + hint
}

func (m Model) renderFilterInput(width int) string {
	muted := m.Styles.Muted
	accent := m.Styles.Accent
	accent2 := m.Styles.Accent2

	var lines []string

	if m.blobLoadAll {
		lines = append(lines, m.renderInputField("Local fuzzy", m.filter.fuzzyQuery, true, accent, muted))
	} else {
		prefixActive := m.filter.focusedInput == searchInputPrefix
		lines = append(lines, m.renderInputField("API prefix", m.filter.prefixQuery, prefixActive, accent2, muted))
		lines = append(lines, m.renderInputField("Local fuzzy", m.filter.fuzzyQuery, !prefixActive, accent, muted))
	}

	// Count line.
	var countLine string
	if m.filter.fetching {
		countLine = muted.Render("  ⟳ fetching from API...")
	} else if len(m.filter.apiResults) > 0 {
		total := m.filter.apiCount
		showing := len(m.blobsList.Items())
		if m.filter.fuzzyQuery != "" && showing != total {
			countLine = muted.Render(fmt.Sprintf("  %d from API · %d matched", total, showing))
		} else {
			countLine = muted.Render(fmt.Sprintf("  %d from API", total))
		}
	}
	if countLine != "" {
		lines = append(lines, countLine)
	}

	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

// renderInputField renders a single labeled input field.
func (m Model) renderInputField(label, query string, active bool, labelStyle, muted lipgloss.Style) string {
	const pad = " "
	if active {
		return pad + labelStyle.Render(label+":") + " " + m.Styles.Overlay.Input.Render(query) + m.Cursor.View()
	}
	if query == "" {
		return pad + muted.Render(label+": ─")
	}
	return pad + muted.Render(label+":") + " " + muted.Render(query)
}
