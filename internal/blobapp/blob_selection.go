package blobapp

import (
	"fmt"
	"path/filepath"
	"sort"

	"github.com/karlssonsimon/lazyaz/internal/appshell"
	"github.com/karlssonsimon/lazyaz/internal/azure/blob"
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

func (m *Model) resetBlobLoadState() {
	m.blobLoadAll = false
	m.deactivateSearch()
	m.discardCommittedFilter()
}

func (m *Model) refreshItems() {
	if m.search.active && m.search.fuzzyQuery != "" && m.search.filtered != nil {
		m.searchRebuildItems()
		return
	}
	if m.search.active && len(m.search.results) > 0 {
		m.searchRebuildItems()
		return
	}
	m.blobsList.SetItems(blobsToItems(m.blobs, m.prefix, m.markedBlobs, m.visualSelectionNames()))
	ui.ClampListSelection(&m.blobsList)
}

func (m Model) toggleBlobLoadAllMode() (Model, tea.Cmd) {
	if !m.hasContainer {
		m.Status = "Open a container before loading blobs"
		return m, nil
	}

	m.deactivateSearch()
	m.discardCommittedFilter()
	m.LastErr = ""

	if m.blobLoadAll {
		m.blobLoadAll = false

		if cached, ok := m.cache.blobs.Get(blobsCacheKey(m.CurrentSub.ID, m.currentAccount.Name, m.containerName, m.prefix, false)); ok {
			m.blobs = cached
			m.blobsList.Title = fmt.Sprintf("Blobs (%d)", len(cached))
			m.refreshItems()
		}

		m.SetLoading(blobsPane)
		m.Status = fmt.Sprintf("Loading up to %d entries under %q", defaultHierarchyBlobLoadLimit, m.prefix)
		return m, tea.Batch(m.Spinner.Tick, fetchHierarchyBlobsCmd(m.service, m.cache.blobs, m.currentAccount, m.containerName, m.prefix, defaultHierarchyBlobLoadLimit, m.blobs))
	}

	m.blobLoadAll = true

	if cached, ok := m.cache.blobs.Get(blobsCacheKey(m.CurrentSub.ID, m.currentAccount.Name, m.containerName, m.prefix, true)); ok {
		m.blobs = cached
		m.blobsList.Title = fmt.Sprintf("Blobs (%d)", len(cached))
		m.refreshItems()
	}

	m.SetLoading(blobsPane)
	m.Status = fmt.Sprintf("Loading all blobs in %s/%s", m.currentAccount.Name, m.containerName)
	return m, tea.Batch(m.Spinner.Tick, fetchAllBlobsCmd(m.service, m.cache.blobs, m.currentAccount, m.containerName, m.prefix, m.blobs))
}

func (m *Model) toggleVisualLineMode() {
	if !m.hasContainer {
		m.Status = "Open a container before visual selection"
		return
	}

	if m.visualLineMode {
		m.commitVisualSelection()
		m.visualLineMode = false
		m.visualAnchor = ""
		m.refreshItems()
		m.Status = fmt.Sprintf("Visual mode off. %d marked.", len(m.markedBlobs))
		return
	}

	m.visualLineMode = true
	m.visualAnchor = m.currentBlobName()
	m.refreshItems()
	if m.visualAnchor == "" {
		m.Status = "Visual mode on. Move up/down to select a range."
		return
	}
	selectionCount := len(m.visualSelectionBlobNames())
	m.Status = fmt.Sprintf("Visual mode on. %d in range.", selectionCount)
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
		m.Status = "Open a container before marking blobs"
		return
	}

	item, ok := m.blobsList.SelectedItem().(blobItem)
	if !ok {
		m.Status = "No blob selected"
		return
	}
	if item.blob.IsPrefix {
		m.Status = "Folder selection is not supported yet"
		return
	}

	if _, exists := m.markedBlobs[item.blob.Name]; exists {
		delete(m.markedBlobs, item.blob.Name)
		m.refreshItems()
		m.Status = fmt.Sprintf("Unmarked %s (%d marked)", item.displayName, len(m.markedBlobs))
		return
	}

	m.markedBlobs[item.blob.Name] = item.blob
	m.refreshItems()
	m.Status = fmt.Sprintf("Marked %s (%d marked)", item.displayName, len(m.markedBlobs))
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

	visibleItems := m.blobsList.VisibleItems()
	if len(visibleItems) == 0 {
		return nil
	}

	items := make([]blobItem, 0, len(visibleItems))
	for _, item := range visibleItems {
		blobEntry, ok := item.(blobItem)
		if !ok {
			continue
		}
		items = append(items, blobEntry)
	}
	if len(items) == 0 {
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

	anchorIdx := -1
	currentIdx := -1
	for i, item := range items {
		if anchorIdx < 0 && item.blob.Name == anchor {
			anchorIdx = i
		}
		if currentIdx < 0 && item.blob.Name == current {
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

	return items[start : end+1]
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
		m.Status = fmt.Sprintf("Unknown marked action: %s", action)
		return m, nil
	}
}

func (m Model) startDownloadMarkedBlobs() (Model, tea.Cmd) {
	if !m.hasAccount || !m.hasContainer {
		m.Status = "Open a container before downloading"
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
			m.Status = "Select blobs with space or visual mode before downloading"
			return m, nil
		}
		blobNames = []string{item.blob.Name}
	}

	if m.downloadDir == "" {
		m.Notify(appshell.LevelError, "no download directory available — set download_dir in ~/.config/lazyaz/config.yaml")
		return m, nil
	}
	destinationRoot := filepath.Join(m.downloadDir, m.currentAccount.Name, m.containerName)
	m.SetLoading(blobsPane)
	m.LastErr = ""
	m.Status = fmt.Sprintf("Downloading %d blob(s) to %s", len(blobNames), destinationRoot)
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
