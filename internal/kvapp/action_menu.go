package kvapp

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/karlssonsimon/lazyaz/internal/appshell"
	"github.com/karlssonsimon/lazyaz/internal/azure/keyvault"
	"github.com/karlssonsimon/lazyaz/internal/fuzzy"
	"github.com/karlssonsimon/lazyaz/internal/keymap"
	"github.com/karlssonsimon/lazyaz/internal/ui"

	"github.com/atotto/clipboard"
	tea "charm.land/bubbletea/v2"
)

type actionID int

const (
	actionYankSecretName actionID = iota
	actionYankSecretValue
	actionYankMarkedAsJSON
	actionClearMarks
)

type action struct {
	id    actionID
	label string
}

type actionMenuState struct {
	active    bool
	cursorIdx int
	query     string
	filtered  []int
	actions   []action
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
	return false, action{}
}

func (m Model) buildActions() []action {
	var actions []action

	if m.focus == secretsPane && m.hasVault {
		if item, ok := m.secretsList.SelectedItem().(secretItem); ok {
			actions = append(actions, action{id: actionYankSecretName, label: "Yank secret name to clipboard"})
			actions = append(actions, action{id: actionYankSecretValue, label: fmt.Sprintf("Yank secret value (%s)", item.secret.Name)})
		}

		if len(m.markedSecrets) > 0 {
			actions = append(actions, action{
				id:    actionYankMarkedAsJSON,
				label: fmt.Sprintf("Yank marked as JSON (%d)", len(m.markedSecrets)),
			})
			actions = append(actions, action{id: actionClearMarks, label: "Clear all marks"})
		}
	}

	return actions
}

func (m Model) executeAction(act action) (Model, tea.Cmd) {
	switch act.id {
	case actionYankSecretName:
		item, ok := m.secretsList.SelectedItem().(secretItem)
		if !ok {
			return m, nil
		}
		return m, func() tea.Msg {
			if err := clipboard.WriteAll(item.secret.Name); err != nil {
				return clipboardMsg{err: err}
			}
			return clipboardMsg{text: item.secret.Name}
		}

	case actionYankSecretValue:
		item, ok := m.secretsList.SelectedItem().(secretItem)
		if !ok {
			return m, nil
		}
		m.SetLoading(m.focus)
		m.loadingSpinnerID = m.NotifySpinner(fmt.Sprintf("Fetching secret value for %s...", item.secret.Name))
		return m, tea.Batch(m.Spinner.Tick, yankSecretValueCmd(m.service, m.currentVault, item.secret.Name, ""))

	case actionYankMarkedAsJSON:
		names := m.sortedMarkedSecretNames()
		if len(names) == 0 {
			return m, nil
		}
		m.SetLoading(m.focus)
		m.loadingSpinnerID = m.NotifySpinner(fmt.Sprintf("Fetching %d secret values...", len(names)))
		return m, tea.Batch(m.Spinner.Tick, yankMarkedSecretsAsJSONCmd(m.service, m.currentVault, names))

	case actionClearMarks:
		count := len(m.markedSecrets)
		for name := range m.markedSecrets {
			delete(m.markedSecrets, name)
		}
		m.refreshSecretItems()
		m.Notify(appshell.LevelInfo, fmt.Sprintf("Cleared %d marks", count))
		return m, nil
	}

	return m, nil
}

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

type clipboardMsg struct {
	text string
	err  error
}

type markedSecretsYankedMsg struct {
	count int
	err   error
}

func yankMarkedSecretsAsJSONCmd(svc *keyvault.Service, vault keyvault.Vault, names []string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		result := make(map[string]string, len(names))
		for _, name := range names {
			value, err := svc.GetSecretValue(ctx, vault, name, "")
			if err != nil {
				return markedSecretsYankedMsg{err: fmt.Errorf("failed to fetch %s: %w", name, err)}
			}
			result[name] = value
		}

		data, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return markedSecretsYankedMsg{err: err}
		}

		if err := clipboard.WriteAll(string(data)); err != nil {
			return markedSecretsYankedMsg{err: err}
		}
		return markedSecretsYankedMsg{count: len(names)}
	}
}

func (m Model) sortedMarkedSecretNames() []string {
	if len(m.markedSecrets) == 0 {
		return nil
	}
	names := make([]string, 0, len(m.markedSecrets))
	for name := range m.markedSecrets {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
