package blobapp

import (
	"fmt"
	"path/filepath"
	"sort"

	"github.com/karlssonsimon/lazyaz/internal/appshell"
	"github.com/karlssonsimon/lazyaz/internal/azure/blob"
	"github.com/karlssonsimon/lazyaz/internal/fuzzy"
	"github.com/karlssonsimon/lazyaz/internal/keymap"
	"github.com/karlssonsimon/lazyaz/internal/ui"

	tea "charm.land/bubbletea/v2"
)

func (m *Model) clearBlobSelectionState() {
	m.visualLineMode = false
	m.visualAnchor = ""
	if m.markedBlobs == nil {
		m.markedBlobs = make(map[string]blob.BlobEntry)
		return
	}
	for name := range m.markedBlobs {
		delete(m.markedBlobs, name)
	}
}

// startLoading dismisses any active spinner, marks the pane as loading,
// and pushes a new spinner notification. This prevents orphaned spinners
// when the user navigates away before a load finishes.
func (m *Model) startLoading(pane int, message string) {
	if m.Loading {
		m.ClearLoading()
		m.DismissSpinner(m.loadingSpinnerID)
	}
	m.SetLoading(pane)
	m.loadingSpinnerID = m.NotifySpinner(message)
}

func (m *Model) resetBlobLoadState() {
	if m.Loading {
		m.ClearLoading()
		m.DismissSpinner(m.loadingSpinnerID)
	}
	m.blobLoadAll = false
	m.clearFilter()
}

func (m *Model) refreshItems() {
	entries := m.displayBlobs()
	w := ui.PaneContentWidth(m.Styles.Chrome.Pane, m.paneWidths[blobsPane])
	ui.SetItemsPreserveKey(&m.blobsList, blobsToItems(entries, m.prefix, w), blobItemKey)
	m.refreshBlobSelectionDisplay()
}

// refreshBlobSelectionDisplay updates the delegate's mark/visual maps
// and re-sets it on the list. This triggers a re-render without
// rebuilding items or touching the filter state.
func (m *Model) refreshBlobSelectionDisplay() {
	d := newBlobDelegate(m.Styles.Delegate, m.Styles)
	d.marked = m.markedBlobs
	d.visual = m.visualSelectionNames()
	m.blobsList.SetDelegate(d)
}

// sortOverlayState manages the sort picker popup.
type sortOverlayState struct {
	active    bool
	cursorIdx int
	query     string
	filtered  []int // indices into sortOptions
}

type sortOption struct {
	label string
	field blobSortField
	desc  bool
}

var sortOptions = []sortOption{
	{"1  Default", blobSortNone, false},
	{"2  Name ascending", blobSortName, false},
	{"3  Name descending", blobSortName, true},
	{"4  Size ascending", blobSortSize, false},
	{"5  Size descending", blobSortSize, true},
	{"6  Date ascending", blobSortDate, false},
	{"7  Date descending", blobSortDate, true},
}

func (s *sortOverlayState) open(currentField blobSortField, currentDesc bool) {
	s.active = true
	s.query = ""
	s.filtered = nil
	s.cursorIdx = 0
	for i, opt := range sortOptions {
		if opt.field == currentField && opt.desc == currentDesc {
			s.cursorIdx = i
			break
		}
	}
}

func (s *sortOverlayState) refilter() {
	if s.query == "" {
		s.filtered = nil
		return
	}
	s.filtered = fuzzy.Filter(s.query, sortOptions, func(o sortOption) string { return o.label })
	if s.cursorIdx >= len(s.filtered) {
		s.cursorIdx = max(0, len(s.filtered)-1)
	}
}

func (s *sortOverlayState) selectedOption() (sortOption, bool) {
	if s.filtered != nil {
		if s.cursorIdx >= len(s.filtered) {
			return sortOption{}, false
		}
		return sortOptions[s.filtered[s.cursorIdx]], true
	}
	if s.cursorIdx >= len(sortOptions) {
		return sortOption{}, false
	}
	return sortOptions[s.cursorIdx], true
}

// handleKey processes a key press in the sort overlay. Returns true if
// a sort was applied (the caller should update the sort fields).
func (s *sortOverlayState) handleKey(key string, km keymap.Keymap) (applied bool, field blobSortField, desc bool) {
	switch {
	case km.ThemeUp.Matches(key):
		if s.cursorIdx > 0 {
			s.cursorIdx--
		}
	case km.ThemeDown.Matches(key):
		n := len(sortOptions)
		if s.filtered != nil {
			n = len(s.filtered)
		}
		if s.cursorIdx < n-1 {
			s.cursorIdx++
		}
	case km.ThemeApply.Matches(key):
		if opt, ok := s.selectedOption(); ok {
			s.active = false
			return true, opt.field, opt.desc
		}
	case km.ThemeCancel.Matches(key):
		if s.query != "" {
			s.query = ""
			s.filtered = nil
			s.cursorIdx = 0
		} else {
			s.active = false
		}
	case km.BackspaceUp.Matches(key):
		if len(s.query) > 0 {
			s.query = s.query[:len(s.query)-1]
			s.refilter()
		}
	case key == "ctrl+v":
		if text := ui.ReadClipboard(); text != "" {
			s.query += text
			s.refilter()
		}
	default:
		if len(key) == 1 && key[0] >= 32 && key[0] < 127 {
			s.query += key
			s.refilter()
		}
	}
	return false, blobSortNone, false
}

func blobSortLabel(field blobSortField, desc bool) string {
	if field == blobSortNone {
		return "default"
	}
	dir := "ascending"
	if desc {
		dir = "descending"
	}
	switch field {
	case blobSortName:
		return "Name " + dir
	case blobSortSize:
		return "Size " + dir
	case blobSortDate:
		return "Date " + dir
	default:
		return "default"
	}
}

func blobSortIndicator(field blobSortField, desc bool) string {
	if field == blobSortNone {
		return ""
	}
	arrow := "\u2191" // ↑
	if desc {
		arrow = "\u2193" // ↓
	}
	switch field {
	case blobSortName:
		return "Name" + arrow
	case blobSortSize:
		return "Size" + arrow
	case blobSortDate:
		return "Date" + arrow
	default:
		return ""
	}
}

func (m Model) toggleBlobLoadAllMode() (Model, tea.Cmd) {
	if !m.hasContainer {
		m.Notify(appshell.LevelInfo, "Open a container before loading blobs")
		return m, nil
	}

	savedPrefix := m.filter.prefixQuery
	m.clearFilter()

	if m.blobLoadAll {
		// Switching back to hierarchy mode.
		m.blobLoadAll = false

		if cached, ok := m.cache.blobs.Get(blobsCacheKey(m.CurrentSub.ID, m.currentAccount.Name, m.containerName, m.prefix, false)); ok {
			m.blobs = cached
			m.blobsList.Title = fmt.Sprintf("Blobs (%d)", len(cached))
			m.refreshItems()
		}

		m.startLoading(blobsPane, fmt.Sprintf("Loading up to %d entries under %q", defaultHierarchyBlobLoadLimit, displayPrefix(m.prefix)))
		return m, tea.Batch(m.Spinner.Tick, fetchHierarchyBlobsCmd(m.service, m.cache.blobs, m.currentAccount, m.containerName, m.prefix, defaultHierarchyBlobLoadLimit, m.blobs))
	}

	// Switching to load-all mode.
	m.blobLoadAll = true

	if savedPrefix != "" {
		// Prefix was active — load all blobs under that prefix.
		// Keep showing current data while the fetch runs.
		effectivePrefix := blobSearchPrefix(m.prefix, savedPrefix)
		m.startLoading(blobsPane, fmt.Sprintf("Loading all blobs under %q", effectivePrefix))
		return m, tea.Batch(m.Spinner.Tick,
			fetchAllBlobsWithPrefixCmd(m.service, m.currentAccount, m.containerName, m.prefix, savedPrefix))
	}

	if cached, ok := m.cache.blobs.Get(blobsCacheKey(m.CurrentSub.ID, m.currentAccount.Name, m.containerName, m.prefix, true)); ok {
		m.blobs = cached
		m.blobsList.Title = fmt.Sprintf("Blobs (%d)", len(cached))
		m.refreshItems()
	}

	m.startLoading(blobsPane, fmt.Sprintf("Loading all blobs in %s/%s", m.currentAccount.Name, m.containerName))
	return m, tea.Batch(m.Spinner.Tick, fetchAllBlobsCmd(m.service, m.cache.blobs, m.currentAccount, m.containerName, m.prefix, m.blobs))
}

func (m *Model) toggleVisualLineMode() {
	if !m.hasContainer {
		m.Notify(appshell.LevelInfo, "Open a container before visual selection")
		return
	}

	if m.visualLineMode {
		m.commitVisualSelection()
		m.visualLineMode = false
		m.visualAnchor = ""
		m.refreshBlobSelectionDisplay()
		m.Notify(appshell.LevelInfo, fmt.Sprintf("Visual mode off. %d marked.", len(m.markedBlobs)))
		return
	}

	m.visualLineMode = true
	m.visualAnchor = m.currentBlobName()
	m.refreshBlobSelectionDisplay()
	if m.visualAnchor == "" {
		m.Notify(appshell.LevelInfo, "Visual mode on. Move up/down to select a range.")
		return
	}
	selectionCount := len(m.visualSelectionBlobNames())
	m.Notify(appshell.LevelInfo, fmt.Sprintf("Visual mode on. %d in range.", selectionCount))
}

// commitVisualSelection merges the current visual range into markedBlobs.
func (m *Model) commitVisualSelection() {
	if !m.visualLineMode {
		return
	}
	for _, item := range m.visualSelectionItems() {
		if item.blob.IsPrefix {
			continue
		}
		m.markedBlobs[item.blob.Name] = item.blob
	}
}

// swapVisualAnchor moves the cursor to the visual anchor position and sets
// the anchor to the old cursor position. Lets you extend the range from
// either end.
func (m *Model) swapVisualAnchor() {
	if !m.visualLineMode || m.visualAnchor == "" {
		return
	}
	oldAnchor := m.visualAnchor
	oldCursor := m.currentBlobName()
	if oldCursor == "" || oldCursor == oldAnchor {
		return
	}
	// Find index of the anchor in the visible list.
	for i, it := range m.blobsList.VisibleItems() {
		if b, ok := it.(blobItem); ok && b.blob.Name == oldAnchor {
			m.blobsList.Select(i)
			m.visualAnchor = oldCursor
			return
		}
	}
}

func (m *Model) toggleCurrentBlobMark() {
	if !m.hasContainer {
		m.Notify(appshell.LevelInfo, "Open a container before marking blobs")
		return
	}

	item, ok := m.blobsList.SelectedItem().(blobItem)
	if !ok {
		m.Notify(appshell.LevelInfo, "No blob selected")
		return
	}
	if item.blob.IsPrefix {
		m.Notify(appshell.LevelInfo, "Folder selection is not supported yet")
		return
	}

	if _, exists := m.markedBlobs[item.blob.Name]; exists {
		delete(m.markedBlobs, item.blob.Name)
		m.refreshBlobSelectionDisplay()
		m.Notify(appshell.LevelInfo, fmt.Sprintf("Unmarked %s (%d marked)", item.displayName, len(m.markedBlobs)))
		return
	}

	m.markedBlobs[item.blob.Name] = item.blob
	m.refreshBlobSelectionDisplay()
	m.Notify(appshell.LevelInfo, fmt.Sprintf("Marked %s (%d marked)", item.displayName, len(m.markedBlobs)))
}

func (m Model) currentBlobName() string {
	item, ok := m.blobsList.SelectedItem().(blobItem)
	if !ok {
		return ""
	}
	return item.blob.Name
}

func (m Model) visualSelectionItems() []blobItem {
	if !m.visualLineMode {
		return nil
	}

	current := m.currentBlobName()
	if current == "" {
		return nil
	}

	anchor := m.visualAnchor
	if anchor == "" {
		anchor = current
	}

	// Use the full sorted blob list (not VisibleItems) so the visual
	// range includes items hidden by a bubbles list filter — same
	// mental model as vim visual mode with folds.
	entries := m.displayBlobs()
	if len(entries) == 0 {
		return nil
	}

	anchorIdx := -1
	currentIdx := -1
	for i, entry := range entries {
		if anchorIdx < 0 && entry.Name == anchor {
			anchorIdx = i
		}
		if currentIdx < 0 && entry.Name == current {
			currentIdx = i
		}
	}
	if currentIdx < 0 {
		return nil
	}
	if anchorIdx < 0 {
		anchorIdx = currentIdx
	}

	start, end := anchorIdx, currentIdx
	if start > end {
		start, end = end, start
	}

	items := make([]blobItem, 0, end-start+1)
	for _, entry := range entries[start : end+1] {
		items = append(items, blobItem{
			blob:        entry,
			displayName: trimPrefixForDisplay(entry.Name, m.prefix),
		})
	}
	return items
}

func (m Model) visualSelectionNames() map[string]struct{} {
	selectedItems := m.visualSelectionItems()
	if len(selectedItems) == 0 {
		return nil
	}

	selectedNames := make(map[string]struct{}, len(selectedItems))
	for _, item := range selectedItems {
		selectedNames[item.blob.Name] = struct{}{}
	}
	return selectedNames
}

func (m Model) visualSelectionBlobNames() []string {
	selectedItems := m.visualSelectionItems()
	if len(selectedItems) == 0 {
		return nil
	}

	unique := make(map[string]struct{}, len(selectedItems))
	for _, item := range selectedItems {
		if item.blob.IsPrefix {
			continue
		}
		unique[item.blob.Name] = struct{}{}
	}

	names := make([]string, 0, len(unique))
	for name := range unique {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (m Model) startMarkedAction(action string) (Model, tea.Cmd) {
	switch action {
	case "download":
		return m.startDownloadMarkedBlobs()
	default:
		m.Notify(appshell.LevelInfo, fmt.Sprintf("Unknown marked action: %s", action))
		return m, nil
	}
}

func (m Model) startDownloadMarkedBlobs() (Model, tea.Cmd) {
	if !m.hasAccount || !m.hasContainer {
		m.Notify(appshell.LevelInfo, "Open a container before downloading")
		return m, nil
	}

	// If visual mode is active, commit the range first.
	if m.visualLineMode {
		m.commitVisualSelection()
		m.visualLineMode = false
		m.visualAnchor = ""
	}

	blobNames := m.sortedMarkedBlobNames()
	if len(blobNames) == 0 {
		item, ok := m.blobsList.SelectedItem().(blobItem)
		if !ok || item.blob.IsPrefix {
			m.Notify(appshell.LevelInfo, "Select blobs with space or visual mode before downloading")
			return m, nil
		}
		blobNames = []string{item.blob.Name}
	}

	if m.downloadDir == "" {
		m.Notify(appshell.LevelError, "no download directory available — set download_dir in ~/.config/lazyaz/config.json")
		return m, nil
	}
	destinationRoot := filepath.Join(m.downloadDir, m.currentAccount.Name, m.containerName)
	m.startLoading(blobsPane, fmt.Sprintf("Downloading %d blob(s) to %s", len(blobNames), destinationRoot))
	return m, tea.Batch(m.Spinner.Tick, downloadBlobsCmd(m.service, m.currentAccount, m.containerName, blobNames, destinationRoot))
}

func (m Model) sortedMarkedBlobNames() []string {
	if len(m.markedBlobs) == 0 {
		return nil
	}
	names := make([]string, 0, len(m.markedBlobs))
	for name := range m.markedBlobs {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func sortedBlobNameSet(values map[string]struct{}) []string {
	if len(values) == 0 {
		return nil
	}

	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
