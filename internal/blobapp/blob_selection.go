package blobapp

import (
	"fmt"
	"path/filepath"
	"sort"

	"github.com/karlssonsimon/lazyaz/internal/appshell"
	"github.com/karlssonsimon/lazyaz/internal/azure/blob"
	"github.com/karlssonsimon/lazyaz/internal/keymap"
	"github.com/karlssonsimon/lazyaz/internal/ui"

	tea "charm.land/bubbletea/v2"
)

func (m *Model) clearBlobSelectionState() {
	m.visualLineMode = false
	m.visualAnchor = ""
	if m.markedBlobs == nil {
		m.markedBlobs = make(map[string]struct{})
		return
	}
	for name := range m.markedBlobs {
		delete(m.markedBlobs, name)
	}
}

func (m *Model) resetBlobLoadState() {
	if m.Loading {
		m.ClearLoading()
		m.DismissSpinner(m.LoadingSpinnerID)
	}
	m.blobLoadAll = false
	m.clearFilter()
}

func (m *Model) refreshItems() {
	entries := m.displayBlobs()
	// blobsPane is rightmost only when no child column is rendered;
	// the layout always reserves the preview slot so RightRule=true.
	w := ui.MillerColumnContentWidth(ui.MillerColumnFrame{
		Width:     m.paneWidths[blobsPane],
		RightRule: m.paneWidths[previewPane] > 0,
	})
	m.refreshItemsWithWidth(entries, w)
}

func (m *Model) refreshItemsWithWidth(entries []blob.BlobEntry, w int) {
	ui.SetItemsPreserveKey(&m.blobsList, blobsToItems(entries, m.prefix, w), blobItemKey)
	m.refreshBlobSelectionDisplay()
}

// refreshBlobSelectionDisplay updates the delegate's mark/visual maps
// and re-sets it on the list. This triggers a re-render without
// rebuilding items or touching the filter state.
func (m *Model) refreshBlobSelectionDisplay() {
	d := ui.NewMarkDelegate(m.Styles.Delegate, m.Styles, blobMarkKey)
	d.Marked = m.markedBlobs
	d.Visual = m.visualSelectionNames()
	m.blobsList.SetDelegate(d)
}

// sortOverlayState manages the sort picker popup.
type sortOverlayState struct {
	ui.SearchableOverlay[sortOption]
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
	s.Open(sortOptions, func(o sortOption) string { return o.label })
	for i, opt := range sortOptions {
		if opt.field == currentField && opt.desc == currentDesc {
			s.CursorIdx = i
			break
		}
	}
}

// handleKey processes a key press in the sort overlay. Returns true if
// a sort was applied (the caller should update the sort fields).
func (s *sortOverlayState) handleKey(key string, km keymap.Keymap) (applied bool, field blobSortField, desc bool) {
	switch {
	case km.ThemeUp.Matches(key):
		s.Move(-1)
		return false, blobSortNone, false
	case km.ThemeDown.Matches(key):
		s.Move(1)
		return false, blobSortNone, false
	case km.ThemeApply.Matches(key):
		if opt, ok := s.Selected(); ok {
			s.Close()
			return true, opt.field, opt.desc
		}
		return false, blobSortNone, false
	case km.ThemeCancel.Matches(key):
		s.Cancel()
		return false, blobSortNone, false
	case key == "ctrl+v":
		if text := ui.ReadClipboard(); text != "" {
			s.TypeText(text)
		}
		return false, blobSortNone, false
	}
	s.HandleQueryKey(key)
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

		m.StartLoading(blobsPane, fmt.Sprintf("Loading up to %d entries under %q", defaultHierarchyBlobLoadLimit, displayPrefix(m.prefix)))
		return m, tea.Batch(m.Spinner.Tick, fetchHierarchyBlobsCmd(m.service, m.cache.blobs, m.currentAccount, m.containerName, m.prefix, defaultHierarchyBlobLoadLimit, m.blobs))
	}

	// Switching to load-all mode.
	m.blobLoadAll = true

	if savedPrefix != "" {
		// Prefix was active — load all blobs under that prefix.
		// Keep showing current data while the fetch runs.
		effectivePrefix := blobSearchPrefix(m.prefix, savedPrefix)
		m.StartLoading(blobsPane, fmt.Sprintf("Loading all blobs under %q", effectivePrefix))
		return m, tea.Batch(m.Spinner.Tick,
			fetchAllBlobsWithPrefixCmd(m.service, m.currentAccount, m.containerName, m.prefix, savedPrefix))
	}

	if cached, ok := m.cache.blobs.Get(blobsCacheKey(m.CurrentSub.ID, m.currentAccount.Name, m.containerName, m.prefix, true)); ok {
		m.blobs = cached
		m.blobsList.Title = fmt.Sprintf("Blobs (%d)", len(cached))
		m.refreshItems()
	}

	scope := m.currentAccount.Name + "/" + m.containerName
	if m.prefix != "" {
		scope += " under " + m.prefix
	}
	m.StartLoading(blobsPane, fmt.Sprintf("Loading all blobs in %s", scope))
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
		m.markedBlobs[item.blob.Name] = struct{}{}
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

	m.markedBlobs[item.blob.Name] = struct{}{}
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

	// Walk the list's visible items so the range covers exactly what
	// the user sees — with a filter applied, hidden rows between the
	// endpoints stay unselected (matches kvapp).
	visible := m.blobsList.VisibleItems()
	if len(visible) == 0 {
		return nil
	}

	anchorIdx := -1
	currentIdx := -1
	for i, it := range visible {
		b, ok := it.(blobItem)
		if !ok {
			continue
		}
		if anchorIdx < 0 && b.blob.Name == anchor {
			anchorIdx = i
		}
		if currentIdx < 0 && b.blob.Name == current {
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
	for _, it := range visible[start : end+1] {
		if b, ok := it.(blobItem); ok {
			items = append(items, b)
		}
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

	return m.startDownloadBlobs(blobNames)
}

// startDownloadBlobs dispatches a download for the given blob names
// without touching marks — "Download current blob" goes through here
// directly so it can't inflate the marked set.
func (m Model) startDownloadBlobs(blobNames []string) (Model, tea.Cmd) {
	if m.downloadDir == "" {
		m.Notify(appshell.LevelError, "no download directory available — set download_dir in ~/.config/lazyaz/config.json")
		return m, nil
	}
	destinationRoot := filepath.Join(m.downloadDir, m.currentAccount.Name, m.containerName)
	m.StartLoading(blobsPane, fmt.Sprintf("Downloading %d blob(s) to %s", len(blobNames), destinationRoot))
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
