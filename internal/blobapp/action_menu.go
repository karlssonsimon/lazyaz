package blobapp

import (
	"fmt"
	"strings"

	"github.com/karlssonsimon/lazyaz/internal/appshell"
	"github.com/karlssonsimon/lazyaz/internal/azure/blob"
	"github.com/karlssonsimon/lazyaz/internal/keymap"
	"github.com/karlssonsimon/lazyaz/internal/ui"

	tea "charm.land/bubbletea/v2"
	"github.com/atotto/clipboard"
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
	actionYankBlobName
	actionYankBlobContent
	actionToggleMark
	actionToggleVisualLine
	actionDownloadSelection
	actionInspect
	actionRefresh
	actionSubscriptionPicker
	actionThemePicker
	actionHelp
	actionUpload
	actionDeleteCurrent
	actionDeleteMarked
	actionRenameCurrent
	actionCreateContainer
	actionDeleteContainer
	actionCreateDirectory
	actionDeleteDirectory
	actionRenameDirectory
)

// action describes one entry in the action menu.
type action struct {
	id    actionID
	label string
	hint  string // keybinding shown right-aligned in menu
}

// actionMenuState manages the action menu overlay.
type actionMenuState struct {
	ui.SearchableOverlay[action]
}

func (s *actionMenuState) open(actions []action) {
	s.Open(actions, func(a action) string { return a.label })
}

func (s *actionMenuState) close() {
	s.Close()
}

func (s *actionMenuState) handleKey(key string, km keymap.Keymap) (selected bool, act action) {
	switch {
	case km.ThemeUp.Matches(key):
		s.Move(-1)
	case km.ThemeDown.Matches(key):
		s.Move(1)
	case km.ThemeApply.Matches(key):
		if a, ok := s.Selected(); ok {
			s.close()
			return true, a
		}
	case km.ThemeCancel.Matches(key):
		s.Cancel()
	case km.BackspaceUp.Matches(key):
		s.Backspace()
	case key == "ctrl+v":
		if text := ui.ReadClipboard(); text != "" {
			s.TypeText(text)
		}
	default:
		if len(key) == 1 && key[0] >= 32 && key[0] < 127 {
			s.TypeText(key)
		}
	}
	return false, action{}
}

// buildActions returns the context-aware list of actions for the
// current model state. Called when the action menu is opened.
func (m Model) buildActions() []action {
	km := m.Keymap
	var actions []action

	if m.hasContainer && m.focus == blobsPane {
		actions = append(actions, action{actionUpload, "Upload files...", ""})

		// Load mode toggle.
		if m.blobLoadAll {
			actions = append(actions, action{actionHierarchy, "Browse by folder", km.ToggleLoadAll.Short()})
		} else {
			actions = append(actions, action{actionLoadAll, "Load all blobs", km.ToggleLoadAll.Short()})
		}

		// Server-side prefix search (only in hierarchy mode).
		if !m.blobLoadAll {
			actions = append(actions, action{actionPrefixSearch, "Search by prefix", ""})
		}

		// Sort.
		actions = append(actions, action{actionSort, "Sort blobs", km.SortBlobs.Short()})

		// Selection.
		actions = append(actions, action{actionToggleMark, "Toggle mark", km.ToggleMark.Short()})
		actions = append(actions, action{actionToggleVisualLine, "Toggle visual line selection", km.ToggleVisualLine.Short()})

		// Marked-blob actions.
		if len(m.markedBlobs) > 0 || m.visualLineMode {
			actions = append(actions, action{actionDownloadSelection, "Download selection", km.DownloadSelection.Short()})
		}
		if len(m.markedBlobs) > 0 {
			actions = append(actions, action{
				actionDownloadMarked,
				fmt.Sprintf("Download marked (%d)", len(m.markedBlobs)),
				"",
			})
			actions = append(actions, action{actionClearMarks, "Clear all marks", ""})
		}

		// Current-blob actions.
		if item, ok := m.blobsList.SelectedItem().(blobItem); ok && !item.blob.IsPrefix {
			actions = append(actions, action{actionDownloadCurrent, "Download current blob", ""})
			actions = append(actions, action{actionYankBlobName, "Yank blob name", ""})
			if item.blob.Size > 0 && item.blob.Size < 5*1024*1024 {
				actions = append(actions, action{actionYankBlobContent, "Yank blob content", km.YankBlobContent.Short()})
			}
			actions = append(actions, action{actionRenameCurrent, "Rename blob...", ""})
			actions = append(actions, action{actionDeleteCurrent, "Delete blob...", ""})
		}

		// Mutation on the marked selection.
		if len(m.markedBlobs) > 0 {
			actions = append(actions, action{actionDeleteMarked, fmt.Sprintf("Delete marked (%d)...", len(m.markedBlobs)), ""})
		}

		if m.currentAccount.IsHnsEnabled {
			actions = append(actions, action{actionCreateDirectory, "Create folder...", ""})
			if item, ok := m.blobsList.SelectedItem().(blobItem); ok && item.blob.IsPrefix {
				actions = append(actions, action{actionRenameDirectory, "Rename folder...", ""})
				actions = append(actions, action{actionDeleteDirectory, "Delete folder...", ""})
			}
		}
	}

	// App-wide actions — available from any pane.
	actions = append(actions,
		action{actionRefresh, "Refresh", km.RefreshScope.Short()},
		action{actionInspect, "Toggle details panel", km.Inspect.Short()},
		action{actionSubscriptionPicker, "Change subscription", km.SubscriptionPicker.Short()},
	)
	if !m.EmbeddedMode {
		actions = append(actions,
			action{actionThemePicker, "Open theme picker", km.ToggleThemePicker.Short()},
			action{actionHelp, "Toggle help", km.ToggleHelp.Short()},
		)
	}

	// Container-level actions — available when an account is selected.
	if m.hasAccount {
		actions = append(actions, action{actionCreateContainer, "Create container...", ""})
	}
	if m.focus == containersPane {
		if _, ok := m.containersList.SelectedItem().(containerItem); ok {
			actions = append(actions, action{actionDeleteContainer, "Delete container...", ""})
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

	case actionYankBlobName:
		item, ok := m.blobsList.SelectedItem().(blobItem)
		if !ok {
			return m, nil
		}
		return m.copyToClipboard(item.blob.Name)

	case actionYankBlobContent:
		item, ok := m.blobsList.SelectedItem().(blobItem)
		if !ok || item.blob.IsPrefix || item.blob.Size == 0 || item.blob.Size >= 5*1024*1024 {
			return m, nil
		}
		m.Notify(appshell.LevelInfo, fmt.Sprintf("Downloading %s...", item.blob.Name))
		return m, downloadBlobToClipboardCmd(m.service, m.currentAccount, m.containerName, item.blob.Name, item.blob.Size)

	case actionToggleMark:
		m.toggleCurrentBlobMark()
		return m, nil

	case actionToggleVisualLine:
		m.toggleVisualLineMode()
		return m, nil

	case actionDownloadSelection:
		return m.startMarkedAction("download")

	case actionRefresh:
		return m.refresh()

	case actionInspect:
		if m.focus != previewPane {
			m.toggleInspect()
		}
		return m, nil

	case actionSubscriptionPicker:
		m.SubOverlay.Open()
		m.startLoading(-1, "Refreshing subscriptions...")
		return m, tea.Batch(m.Spinner.Tick, fetchSubscriptionsCmd(m.service, m.cache.subscriptions, m.Subscriptions))

	case actionThemePicker:
		if !m.EmbeddedMode && !m.ThemeOverlay.Active {
			m.ThemeOverlay.Open()
		}
		return m, nil

	case actionHelp:
		if !m.EmbeddedMode {
			if m.HelpOverlay.Active {
				m.HelpOverlay.Close()
			} else {
				m.HelpOverlay.Open("Azure Blob Explorer Help", m.HelpSections())
			}
		}
		return m, nil

	case actionUpload:
		return m.openUploadBrowser()

	case actionDeleteCurrent:
		item, ok := m.blobsList.SelectedItem().(blobItem)
		if !ok || item.blob.IsPrefix {
			return m, nil
		}
		name := item.blob.Name
		m.confirmModal.Open("Delete blob?", name+" will be permanently removed.", "delete", "cancel", true)
		m.confirmAction = func() tea.Cmd {
			return deleteBlobCmd(m.service, m.currentAccount, m.containerName, name)
		}
		return m, nil

	case actionDeleteMarked:
		names := make([]string, 0, len(m.markedBlobs))
		for n := range m.markedBlobs {
			names = append(names, n)
		}
		if len(names) == 0 {
			return m, nil
		}
		msg := fmt.Sprintf("%d blobs will be permanently removed.", len(names))
		m.confirmModal.Open(fmt.Sprintf("Delete %d blobs?", len(names)), msg, "delete", "cancel", true)
		m.confirmAction = func() tea.Cmd {
			return deleteMarkedBlobsCmd(m.service, m.currentAccount, m.containerName, names)
		}
		return m, nil

	case actionRenameCurrent:
		item, ok := m.blobsList.SelectedItem().(blobItem)
		if !ok || item.blob.IsPrefix {
			return m, nil
		}
		old := item.blob.Name
		m.textInput.Open("Rename blob", "new blob name", old, func(v string) string {
			if strings.TrimSpace(v) == "" {
				return "name required"
			}
			if v == old {
				return "unchanged"
			}
			return ""
		})
		m.textInputAction = func(v string) tea.Cmd {
			return renameBlobCmd(m.service, m.currentAccount, m.containerName, old, v)
		}
		return m, nil

	case actionCreateContainer:
		m.textInput.Open("Create container", "container name (3-63 lowercase, digits, hyphens)", "", func(v string) string {
			return blob.ValidateContainerName(v)
		})
		account := m.currentAccount
		m.textInputAction = func(v string) tea.Cmd {
			return createContainerCmd(m.service, account, v)
		}
		return m, nil

	case actionDeleteContainer:
		item, ok := m.containersList.SelectedItem().(containerItem)
		if !ok {
			return m, nil
		}
		name := item.container.Name
		m.confirmModal.Open(
			"Delete container?",
			fmt.Sprintf("%s and every blob it contains will be permanently removed.", name),
			"delete", "cancel", true)
		account := m.currentAccount
		m.confirmAction = func() tea.Cmd {
			return deleteContainerCmd(m.service, account, name)
		}
		return m, nil

	case actionCreateDirectory:
		prefix := m.prefix
		m.textInput.Open("Create folder", "folder name", "", func(v string) string {
			v = strings.TrimSpace(v)
			if v == "" {
				return "name required"
			}
			if strings.ContainsAny(v, "\\") {
				return "use forward slashes only"
			}
			return ""
		})
		account := m.currentAccount
		container := m.containerName
		m.textInputAction = func(v string) tea.Cmd {
			full := strings.TrimSuffix(prefix, "/") + "/" + strings.Trim(v, "/")
			full = strings.TrimPrefix(full, "/")
			return createDirectoryCmd(m.service, account, container, full)
		}
		return m, nil

	case actionDeleteDirectory:
		item, ok := m.blobsList.SelectedItem().(blobItem)
		if !ok || !item.blob.IsPrefix {
			return m, nil
		}
		fullPath := strings.TrimSuffix(item.blob.Name, "/")
		m.confirmModal.Open(
			"Delete folder?",
			fmt.Sprintf("%s and every file inside will be permanently removed.", fullPath),
			"delete", "cancel", true)
		account := m.currentAccount
		container := m.containerName
		m.confirmAction = func() tea.Cmd {
			return deleteDirectoryCmd(m.service, account, container, fullPath)
		}
		return m, nil

	case actionRenameDirectory:
		item, ok := m.blobsList.SelectedItem().(blobItem)
		if !ok || !item.blob.IsPrefix {
			return m, nil
		}
		oldPath := strings.TrimSuffix(item.blob.Name, "/")
		m.textInput.Open("Rename folder", "new folder path", oldPath, func(v string) string {
			v = strings.TrimSpace(v)
			if v == "" {
				return "path required"
			}
			if strings.ContainsAny(v, "\\") {
				return "use forward slashes only"
			}
			if strings.Trim(v, "/") == oldPath {
				return "unchanged"
			}
			return ""
		})
		account := m.currentAccount
		container := m.containerName
		m.textInputAction = func(v string) tea.Cmd {
			return renameDirectoryCmd(m.service, account, container, oldPath, strings.Trim(v, "/"))
		}
		return m, nil
	}

	return m, nil
}

// renderActionMenu renders the action menu overlay on top of the base view.
func (m Model) renderActionMenu(base string) string {
	s := &m.actionMenu
	visible := s.Visible()
	items := make([]ui.OverlayItem, len(visible))
	for i, action := range visible {
		items[i] = ui.OverlayItem{
			Label: action.label,
			Hint:  action.hint,
		}
	}
	cfg := ui.OverlayListConfig{
		Title:      "Actions",
		Query:      s.Query,
		CursorView: m.Cursor.View(),
		CloseHint:  m.Keymap.Cancel.Short(),
		Bindings: &ui.OverlayBindings{

			MoveUp:   m.Keymap.ThemeUp,

			MoveDown: m.Keymap.ThemeDown,

			Apply:    m.Keymap.ThemeApply,

			Cancel:   m.Keymap.ThemeCancel,

			Erase:    m.Keymap.BackspaceUp,

		},
		MaxVisible: 10,
		Center:     true,
	}
	return ui.RenderOverlayList(cfg, items, s.CursorIdx, m.Styles, m.Width, m.Height, base)
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
