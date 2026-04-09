package blobapp

import (
	"fmt"
	"strings"

	"github.com/karlssonsimon/lazyaz/internal/appshell"
	"github.com/karlssonsimon/lazyaz/internal/azure/blob"
	"github.com/karlssonsimon/lazyaz/internal/ui"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// hasActiveFilter reports whether a prefix search has been performed.
func (m *Model) hasActiveFilter() bool {
	return m.filter.prefixQuery != ""
}

// clearFilter removes all filter state and restores the unfiltered view.
func (m *Model) clearFilter() {
	m.filter = blobFilter{}
	m.refreshItems()
}

// --- Server prefix search (action menu) ------------------------------------

// openPrefixSearchInput opens the prefix search overlay.
// Called from the action menu "Server prefix search".
func (m Model) openPrefixSearchInput() (Model, tea.Cmd) {
	m.filter.inputOpen = true
	if m.visualLineMode {
		m.visualLineMode = false
		m.visualAnchor = ""
		m.refreshItems()
	}
	return m, m.Cursor.Focus()
}

// handlePrefixSearchKey handles input while the prefix search overlay
// is active.
func (m Model) handlePrefixSearchKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	key := msg.String()

	switch {
	case m.Keymap.Cancel.Matches(key):
		m.filter.inputOpen = false
		m.Cursor.Blur()
		return m, nil

	case m.Keymap.OpenFocused.Matches(key):
		if m.filter.prefixQuery == "" {
			m.filter.inputOpen = false
			m.Cursor.Blur()
			return m, nil
		}
		cmd := m.firePrefixSearch()
		return m, cmd

	case m.Keymap.BackspaceUp.Matches(key):
		if m.filter.prefixQuery == "" {
			m.filter.inputOpen = false
			m.Cursor.Blur()
			return m, nil
		}
		m.filter.prefixQuery = m.filter.prefixQuery[:len(m.filter.prefixQuery)-1]
		return m, nil

	case key == "ctrl+v":
		if text := ui.ReadClipboard(); text != "" {
			m.filter.prefixQuery += text
		}
		return m, nil
	default:
		if len(key) == 1 && key[0] >= 32 && key[0] < 127 {
			m.filter.prefixQuery += key
			return m, nil
		}
	}

	return m, nil
}

// firePrefixSearch fires an API prefix search and returns the command.
func (m *Model) firePrefixSearch() tea.Cmd {
	m.filter.fetching = true
	m.filter.prefixFetched = false
	m.filter.apiResults = nil
	m.filter.apiCount = 0
	m.filter.inputOpen = false
	m.Cursor.Blur()
	m.refreshItems()
	m.SetLoading(blobsPane)
	effectivePrefix := blobSearchPrefix(m.prefix, m.filter.prefixQuery)
	m.loadingSpinnerID = m.NotifySpinner(fmt.Sprintf("Searching blobs by prefix %q...", effectivePrefix))
	return tea.Batch(m.Spinner.Tick,
		fetchSearchBlobsCmd(m.service, m.currentAccount, m.containerName, m.prefix, m.filter.prefixQuery, defaultBlobPrefixSearchLimit))
}

// handleFilterBlobsLoaded processes results from a server prefix search.
func (m Model) handleFilterBlobsLoaded(msg blobsLoadedMsg) (Model, tea.Cmd) {
	if msg.err != nil {
		m.ClearLoading()
		m.filter.fetching = false
		m.Notify(appshell.LevelError, fmt.Sprintf("Search failed: %s", msg.err.Error()))
		return m, nil
	}

	m.filter.apiResults = msg.blobs
	m.filter.apiCount = len(msg.blobs)
	m.refreshItems()

	if msg.done {
		m.ClearLoading()
		m.filter.fetching = false
		m.filter.prefixFetched = true
		effectivePrefix := blobSearchPrefix(m.prefix, m.filter.prefixQuery)
		m.Notify(appshell.LevelSuccess, fmt.Sprintf("Found %d blobs by prefix %q", len(msg.blobs), effectivePrefix))
	}

	return m, msg.next
}

// filterSource returns the base set of blobs for the list. When a
// prefix search has been performed, the API results are the source;
// otherwise the currently loaded blobs are used.
func (m *Model) filterSource() []blob.BlobEntry {
	if m.filter.prefixQuery != "" {
		return m.filter.apiResults
	}
	return m.blobs
}

// displayBlobs returns the blob entries to show in the list after
// applying prefix search results and sorting.
func (m *Model) displayBlobs() []blob.BlobEntry {
	return sortBlobs(m.filterSource(), m.blobSortField, m.blobSortDesc)
}

// --- Rendering --------------------------------------------------------------

// renderPrefixSearchOverlay renders the centered prefix search input overlay.
func (m Model) renderPrefixSearchOverlay(base string) string {
	styles := m.Styles
	width := 60
	if m.Width < 70 {
		width = m.Width - 10
	}
	if width < 30 {
		width = 30
	}

	title := styles.Overlay.Title.Render("Server Prefix Search")
	closeHint := styles.Muted.Render(m.Keymap.Cancel.Short())
	gap := width - lipgloss.Width(title) - lipgloss.Width(closeHint) - 2
	if gap < 1 {
		gap = 1
	}
	titleBar := title + strings.Repeat(" ", gap) + closeHint

	prompt := styles.Accent2.Render("> ")
	if m.prefix != "" {
		prompt = styles.Accent2.Render("> " + m.prefix)
	}
	query := m.filter.prefixQuery
	cursor := m.Cursor.View()
	inputLine := prompt + query + cursor

	hint := styles.Muted.Render("  enter search · esc cancel")

	content := lipgloss.JoinVertical(lipgloss.Left, titleBar, inputLine, hint)
	box := styles.Overlay.Box.Width(width).Render(content)

	return ui.PlaceOverlay(m.Width, m.Height, box, base)
}

// renderFilterBanner returns a one-line summary for the pane when a
// prefix search is active. Shows how to clear it.
func (m Model) renderFilterBanner() string {
	muted := m.Styles.Muted
	accent := m.Styles.Accent
	label := accent.Render("PREFIX · " + m.filter.prefixQuery)
	count := ""
	if m.filter.apiCount > 0 {
		count = muted.Render(fmt.Sprintf(" → %d results", m.filter.apiCount))
	}
	hint := muted.Render("  esc clear")
	return " " + label + count + hint
}
