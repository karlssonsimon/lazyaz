package kvapp

import (
	"fmt"

	"github.com/karlssonsimon/lazyaz/internal/appshell"
	"github.com/karlssonsimon/lazyaz/internal/ui"
	"github.com/karlssonsimon/lazyaz/internal/vim"
)

func (m *Model) clearSecretSelectionState() {
	m.visual.Stop()
	if m.marked.Items() == nil {
		m.marked = vim.NewMarkSet()
		return
	}
	m.marked.Clear()
}

func (m *Model) refreshSecretItems() {
	ui.SetItemsPreserveKey(&m.secretsList, secretsToItems(m.secrets), secretItemKey)
	m.refreshSecretSelectionDisplay()
}

func (m *Model) refreshSecretSelectionDisplay() {
	d := ui.NewMarkDelegate(m.Styles.Delegate, m.Styles, secretMarkKey)
	d.Marked = m.marked.Items()
	d.Visual = m.visualSelectionNames()
	m.secretsList.SetDelegate(d)
}

func (m *Model) toggleVisualLineMode() {
	if !m.hasVault {
		m.Notify(appshell.LevelInfo, "Open a vault before visual selection")
		return
	}

	if m.visual.Active() {
		m.commitVisualSelection()
		m.visual.Stop()
		m.refreshSecretSelectionDisplay()
		m.Notify(appshell.LevelInfo, fmt.Sprintf("Visual mode off. %d marked.", m.marked.Len()))
		return
	}

	m.visual.Start(m.currentSecretName())
	m.refreshSecretSelectionDisplay()
	if m.visual.Anchor() == "" {
		m.Notify(appshell.LevelInfo, "Visual mode on. Move up/down to select a range.")
		return
	}
	selectionCount := len(m.visualSelectionSecretNames())
	m.Notify(appshell.LevelInfo, fmt.Sprintf("Visual mode on. %d in range.", selectionCount))
}

func (m *Model) commitVisualSelection() {
	if !m.visual.Active() {
		return
	}
	for _, item := range m.visualSelectionItems() {
		m.marked.Add(item.secret.Name)
	}
}

func (m *Model) swapVisualAnchor() {
	if !m.visual.Active() || m.visual.Anchor() == "" {
		return
	}
	anchorIdx := m.visual.AnchorIdx(m.secretsList.VisibleVersion(), m.findSecretIdx)
	if anchorIdx < 0 {
		return
	}
	oldCursor := m.currentSecretName()
	if oldCursor == "" || oldCursor == m.visual.Anchor() {
		return
	}
	m.visual.SetAnchorWithIdx(oldCursor, m.secretsList.Index(), m.secretsList.VisibleVersion())
	m.secretsList.Select(anchorIdx)
}

// findSecretIdx locates a secret name in the visible list, -1 when
// absent. This is the scan vim.Visual caches per visible version.
func (m *Model) findSecretIdx(name string) int {
	for i, it := range m.secretsList.VisibleItems() {
		if s, ok := it.(secretItem); ok && s.secret.Name == name {
			return i
		}
	}
	return -1
}

func (m *Model) toggleCurrentSecretMark() {
	if !m.hasVault {
		m.Notify(appshell.LevelInfo, "Open a vault before marking secrets")
		return
	}

	item, ok := m.secretsList.SelectedItem().(secretItem)
	if !ok {
		m.Notify(appshell.LevelInfo, "No secret selected")
		return
	}

	if m.marked.Toggle(item.secret.Name) {
		m.refreshSecretSelectionDisplay()
		m.Notify(appshell.LevelInfo, fmt.Sprintf("Marked %s (%d marked)", item.secret.Name, m.marked.Len()))
		return
	}
	m.refreshSecretSelectionDisplay()
	m.Notify(appshell.LevelInfo, fmt.Sprintf("Unmarked %s (%d marked)", item.secret.Name, m.marked.Len()))
}

func (m Model) currentSecretName() string {
	item, ok := m.secretsList.SelectedItem().(secretItem)
	if !ok {
		return ""
	}
	return item.secret.Name
}

func (m Model) visualSelectionItems() []secretItem {
	if m.currentSecretName() == "" {
		return nil
	}

	visible := m.secretsList.VisibleItems()
	if len(visible) == 0 {
		return nil
	}

	start, end, ok := m.visual.Range(m.secretsList.Index(), m.secretsList.VisibleVersion(), m.findSecretIdx)
	if !ok {
		return nil
	}

	items := make([]secretItem, 0, end-start+1)
	for _, it := range visible[start : end+1] {
		if s, ok := it.(secretItem); ok {
			items = append(items, s)
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
		selectedNames[item.secret.Name] = struct{}{}
	}
	return selectedNames
}

func (m Model) visualSelectionSecretNames() []string {
	selectedItems := m.visualSelectionItems()
	if len(selectedItems) == 0 {
		return nil
	}

	unique := make(map[string]struct{}, len(selectedItems))
	for _, item := range selectedItems {
		unique[item.secret.Name] = struct{}{}
	}

	names := make([]string, 0, len(unique))
	for name := range unique {
		names = append(names, name)
	}
	return names
}
