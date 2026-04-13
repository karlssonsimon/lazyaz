package kvapp

import (
	"fmt"

	"github.com/karlssonsimon/lazyaz/internal/appshell"
	"github.com/karlssonsimon/lazyaz/internal/azure/keyvault"
	"github.com/karlssonsimon/lazyaz/internal/ui"
)

func (m *Model) clearSecretSelectionState() {
	m.visualLineMode = false
	m.visualAnchor = ""
	if m.markedSecrets == nil {
		m.markedSecrets = make(map[string]keyvault.Secret)
		return
	}
	for name := range m.markedSecrets {
		delete(m.markedSecrets, name)
	}
}

func (m *Model) refreshSecretItems() {
	prevFilter := m.secretsList.FilterValue()
	m.secretsList.ResetFilter()
	m.secretsList.SetItems(secretsToItems(m.secrets))
	if prevFilter != "" {
		m.secretsList.SetFilterText(prevFilter)
	}
	ui.ClampListSelection(&m.secretsList)
	m.refreshSecretSelectionDisplay()
}

func (m *Model) refreshSecretSelectionDisplay() {
	d := newSecretDelegate(m.Styles.Delegate, m.Styles)
	d.marked = m.markedSecrets
	d.visual = m.visualSelectionNames()
	m.secretsList.SetDelegate(d)
}

func (m *Model) toggleVisualLineMode() {
	if !m.hasVault {
		m.Notify(appshell.LevelInfo, "Open a vault before visual selection")
		return
	}

	if m.visualLineMode {
		m.commitVisualSelection()
		m.visualLineMode = false
		m.visualAnchor = ""
		m.refreshSecretSelectionDisplay()
		m.Notify(appshell.LevelInfo, fmt.Sprintf("Visual mode off. %d marked.", len(m.markedSecrets)))
		return
	}

	m.visualLineMode = true
	m.visualAnchor = m.currentSecretName()
	m.refreshSecretSelectionDisplay()
	if m.visualAnchor == "" {
		m.Notify(appshell.LevelInfo, "Visual mode on. Move up/down to select a range.")
		return
	}
	selectionCount := len(m.visualSelectionSecretNames())
	m.Notify(appshell.LevelInfo, fmt.Sprintf("Visual mode on. %d in range.", selectionCount))
}

func (m *Model) commitVisualSelection() {
	if !m.visualLineMode {
		return
	}
	for _, item := range m.visualSelectionItems() {
		m.markedSecrets[item.secret.Name] = item.secret
	}
}

func (m *Model) swapVisualAnchor() {
	if !m.visualLineMode || m.visualAnchor == "" {
		return
	}
	oldAnchor := m.visualAnchor
	oldCursor := m.currentSecretName()
	if oldCursor == "" || oldCursor == oldAnchor {
		return
	}
	for i, it := range m.secretsList.VisibleItems() {
		if s, ok := it.(secretItem); ok && s.secret.Name == oldAnchor {
			m.secretsList.Select(i)
			m.visualAnchor = oldCursor
			return
		}
	}
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

	if _, exists := m.markedSecrets[item.secret.Name]; exists {
		delete(m.markedSecrets, item.secret.Name)
		m.refreshSecretSelectionDisplay()
		m.Notify(appshell.LevelInfo, fmt.Sprintf("Unmarked %s (%d marked)", item.secret.Name, len(m.markedSecrets)))
		return
	}

	m.markedSecrets[item.secret.Name] = item.secret
	m.refreshSecretSelectionDisplay()
	m.Notify(appshell.LevelInfo, fmt.Sprintf("Marked %s (%d marked)", item.secret.Name, len(m.markedSecrets)))
}

func (m Model) currentSecretName() string {
	item, ok := m.secretsList.SelectedItem().(secretItem)
	if !ok {
		return ""
	}
	return item.secret.Name
}

func (m Model) visualSelectionItems() []secretItem {
	if !m.visualLineMode {
		return nil
	}

	current := m.currentSecretName()
	if current == "" {
		return nil
	}

	anchor := m.visualAnchor
	if anchor == "" {
		anchor = current
	}

	secrets := m.secrets
	if len(secrets) == 0 {
		return nil
	}

	anchorIdx := -1
	currentIdx := -1
	for i, s := range secrets {
		if anchorIdx < 0 && s.Name == anchor {
			anchorIdx = i
		}
		if currentIdx < 0 && s.Name == current {
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

	items := make([]secretItem, 0, end-start+1)
	for _, s := range secrets[start : end+1] {
		items = append(items, secretItem{secret: s})
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
