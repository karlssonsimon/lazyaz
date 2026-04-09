package blobapp

import (
	"fmt"

	"github.com/karlssonsimon/lazyaz/internal/appshell"
	"github.com/karlssonsimon/lazyaz/internal/fuzzy"
	"github.com/karlssonsimon/lazyaz/internal/keymap"
	"github.com/karlssonsimon/lazyaz/internal/ui"

	"github.com/atotto/clipboard"
	tea "charm.land/bubbletea/v2"
)

// actionID identifies an action in the action menu. Each value
// corresponds to a concrete handler in executeAction.
type actionID int

const (
	actionLoadAll actionID = iota
	actionHierarchy
	actionPrefixSearch
	actionSort
	actionDownloadMarked
	actionClearMarks
	actionDownloadCurrent
	actionCopyBlobName
)

// action describes one entry in the action menu.
type action struct {
	id    actionID
	label string
}

// actionMenuState manages the action menu overlay.
type actionMenuState struct {
	active    bool
	cursorIdx int
	query     string
	filtered  []int    // indices into the current actions slice
	actions   []action // built at open time from context
}

func (s *actionMenuState) open(actions []action) {
	s.active = true
	s.cursorIdx = 0
	s.query = ""
	s.filtered = nil
	s.actions = actions
}

func (s *actionMenuState) close() {
	*s = actionMenuState{}
}

func (s *actionMenuState) refilter() {
	if s.query == "" {
		s.filtered = nil
		s.cursorIdx = 0
		return
	}
	s.filtered = fuzzy.Filter(s.query, s.actions, func(a action) string {
		return a.label
	})
	if s.cursorIdx >= len(s.filtered) {
		s.cursorIdx = max(0, len(s.filtered)-1)
	}
}

func (s *actionMenuState) selectedAction() (action, bool) {
	list := s.actions
	if s.filtered != nil {
		if len(s.filtered) == 0 {
			return action{}, false
		}
		idx := s.filtered[s.cursorIdx]
		return list[idx], true
	}
	if s.cursorIdx < len(list) {
		return list[s.cursorIdx], true
	}
	return action{}, false
}

func (s *actionMenuState) visibleCount() int {
	if s.filtered != nil {
		return len(s.filtered)
	}
	return len(s.actions)
}

func (s *actionMenuState) handleKey(key string, km keymap.Keymap) (selected bool, act action) {
	switch {
	case km.ThemeUp.Matches(key):
		if s.cursorIdx > 0 {
			s.cursorIdx--
		}
	case km.ThemeDown.Matches(key):
		if s.cursorIdx < s.visibleCount()-1 {
			s.cursorIdx++
		}
	case km.ThemeApply.Matches(key):
		if a, ok := s.selectedAction(); ok {
			s.close()
			return true, a
		}
	case km.ThemeCancel.Matches(key):
		if s.query != "" {
			s.query = ""
			s.filtered = nil
			s.cursorIdx = 0
		} else {
			s.close()
		}
	case km.BackspaceUp.Matches(key):
		if len(s.query) > 0 {
			s.query = s.query[:len(s.query)-1]
			s.refilter()
		}
	default:
		if len(key) == 1 && key[0] >= 32 && key[0] < 127 {
			s.query += key
			s.refilter()
		}
	}
	return false, action{}
}

// buildActions returns the context-aware list of actions for the
// current model state. Called when the action menu is opened.
func (m Model) buildActions() []action {
	var actions []action

	if m.hasContainer {
		// Load mode toggle.
		if m.blobLoadAll {
			actions = append(actions, action{id: actionHierarchy, label: "Browse by folder"})
		} else {
			actions = append(actions, action{id: actionLoadAll, label: "Load all blobs"})
		}

		// Server-side prefix search (only in hierarchy mode).
		if !m.blobLoadAll {
			actions = append(actions, action{id: actionPrefixSearch, label: "Server prefix search"})
		}

		// Sort.
		actions = append(actions, action{id: actionSort, label: "Sort blobs"})

		// Marked-blob actions.
		if len(m.markedBlobs) > 0 {
			actions = append(actions, action{
				id:    actionDownloadMarked,
				label: fmt.Sprintf("Download marked (%d)", len(m.markedBlobs)),
			})
			actions = append(actions, action{id: actionClearMarks, label: "Clear all marks"})
		}

		// Current-blob actions.
		if item, ok := m.blobsList.SelectedItem().(blobItem); ok && !item.blob.IsPrefix {
			actions = append(actions, action{id: actionDownloadCurrent, label: "Download current blob"})
			actions = append(actions, action{id: actionCopyBlobName, label: "Copy blob name to clipboard"})
		}
	}

	return actions
}

// executeAction runs the selected action and returns the updated model
// and any command to execute.
func (m Model) executeAction(act action) (Model, tea.Cmd) {
	switch act.id {
	case actionLoadAll, actionHierarchy:
		return m.toggleBlobLoadAllMode()

	case actionPrefixSearch:
		return m.openPrefixSearchInput()

	case actionSort:
		m.sortOverlay.open(m.blobSortField, m.blobSortDesc)
		return m, nil

	case actionDownloadMarked:
		return m.startMarkedAction("download")

	case actionClearMarks:
		count := len(m.markedBlobs)
		for name := range m.markedBlobs {
			delete(m.markedBlobs, name)
		}
		m.refreshItems()
		m.Notify(appshell.LevelInfo, fmt.Sprintf("Cleared %d marks", count))
		return m, nil

	case actionDownloadCurrent:
		item, ok := m.blobsList.SelectedItem().(blobItem)
		if !ok || item.blob.IsPrefix {
			return m, nil
		}
		// Mark it temporarily and download.
		m.markedBlobs[item.blob.Name] = item.blob
		m.refreshItems()
		return m.startMarkedAction("download")

	case actionCopyBlobName:
		item, ok := m.blobsList.SelectedItem().(blobItem)
		if !ok {
			return m, nil
		}
		return m.copyToClipboard(item.blob.Name)
	}

	return m, nil
}

// renderActionMenu renders the action menu overlay on top of the base view.
func (m Model) renderActionMenu(base string) string {
	s := &m.actionMenu
	indices := s.filtered
	if indices == nil {
		indices = make([]int, len(s.actions))
		for i := range s.actions {
			indices[i] = i
		}
	}
	items := make([]ui.OverlayItem, len(indices))
	for ci, si := range indices {
		items[ci] = ui.OverlayItem{
			Label: s.actions[si].label,
		}
	}
	cfg := ui.OverlayListConfig{
		Title:      "Actions",
		Query:      s.query,
		CursorView: m.Cursor.View(),
		CloseHint:  m.Keymap.Cancel.Short(),
		MaxVisible: 10,
		Center:     true,
	}
	return ui.RenderOverlayList(cfg, items, s.cursorIdx, m.Styles.Overlay, m.Width, m.Height, base)
}

// copyToClipboard copies text to the system clipboard.
func (m Model) copyToClipboard(text string) (Model, tea.Cmd) {
	return m, func() tea.Msg {
		if err := writeClipboard(text); err != nil {
			return clipboardMsg{err: err}
		}
		return clipboardMsg{text: text}
	}
}

type clipboardMsg struct {
	text string
	err  error
}

// writeClipboard writes to the system clipboard. Extracted for testability.
func writeClipboard(text string) error {
	// Use atotto/clipboard if available, otherwise return an error.
	// For now, use the same package the kvapp uses.
	return clipboard.WriteAll(text)
}
