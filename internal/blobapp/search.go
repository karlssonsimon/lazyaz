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
		ti := ui.TextInput{Value: m.filter.prefixQuery, Cursor: m.filter.prefixCaret}
		ti.HandleKey("backspace")
		m.filter.prefixQuery = ti.Value
		m.filter.prefixCaret = ti.Cursor
		return m, nil

	case key == "ctrl+v":
		if text := ui.ReadClipboard(); text != "" {
			ti := ui.TextInput{Value: m.filter.prefixQuery, Cursor: m.filter.prefixCaret}
			ti.Insert(text)
			m.filter.prefixQuery = ti.Value
			m.filter.prefixCaret = ti.Cursor
		}
		return m, nil
	}
	ti := ui.TextInput{Value: m.filter.prefixQuery, Cursor: m.filter.prefixCaret}
	if ti.HandleKey(key) {
		m.filter.prefixQuery = ti.Value
		m.filter.prefixCaret = ti.Cursor
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
	effectivePrefix := blobSearchPrefix(m.prefix, m.filter.prefixQuery)
	m.StartLoading(blobsPane, fmt.Sprintf("Searching blobs by prefix %q...", effectivePrefix))
	return tea.Batch(m.Spinner.Tick,
		fetchSearchBlobsCmd(m.service, m.currentAccount, m.containerName, m.prefix, m.filter.prefixQuery, defaultBlobPrefixSearchLimit))
}

// handleFilterBlobsLoaded processes results from a server prefix search.
func (m Model) handleFilterBlobsLoaded(msg blobsLoadedMsg) (Model, tea.Cmd) {
	// Drop results from a search fired in a scope the user has left —
	// without this an in-flight search from container A lands as the
	// results of a newer search in container B.
	if !m.hasAccount || !m.hasContainer ||
		!sameAccount(m.currentAccount, msg.account) || m.containerName != msg.container ||
		m.prefix != msg.prefix || msg.query != m.filter.prefixQuery {
		return m, nil
	}
	if msg.err != nil {
		m.ClearLoading()
		m.filter.fetching = false
		// ResolveSpinner, not Notify: the StartLoading spinner toast
		// never auto-expires, so it must be explicitly replaced here.
		m.ResolveSpinner(m.LoadingSpinnerID, appshell.LevelError, fmt.Sprintf("Search failed: %s", msg.err.Error()))
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
		m.ResolveSpinner(m.LoadingSpinnerID, appshell.LevelSuccess, fmt.Sprintf("Found %d blobs by prefix %q", len(msg.blobs), effectivePrefix))
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
	before, cv, after := ui.PrepareCursor(m.filter.prefixQuery, m.filter.prefixCaret, m.Cursor)
	inputLine := prompt + before + cv + after

	hint := styles.Muted.Render("  enter search · esc cancel")

	content := lipgloss.JoinVertical(lipgloss.Left, titleBar, inputLine, hint)
	box := styles.Overlay.Box.Width(width).Render(content)

	return ui.PlaceOverlay(m.Width, m.Height, box, base)
}
