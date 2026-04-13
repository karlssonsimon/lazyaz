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
	actionYankSecretVersion
	actionToggleMark
	actionToggleVisualLine
	actionInspect
	actionRefresh
	actionSubscriptionPicker
	actionThemePicker
	actionHelp
)

type action struct {
	id    actionID
	label string
	hint  string // keybinding shown right-aligned in menu
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
	km := m.Keymap
	var actions []action

	if m.focus == secretsPane && m.hasVault {
		if item, ok := m.secretsList.SelectedItem().(secretItem); ok {
			actions = append(actions, action{actionYankSecretName, "Yank secret name", ""})
			actions = append(actions, action{actionYankSecretValue, fmt.Sprintf("Yank secret value (%s)", item.secret.Name), km.YankSecret.Short()})
		}

		// Selection.
		actions = append(actions,
			action{actionToggleMark, "Toggle mark", km.ToggleMark.Short()},
			action{actionToggleVisualLine, "Toggle visual line selection", km.ToggleVisualLine.Short()},
		)

		if len(m.markedSecrets) > 0 {
			actions = append(actions, action{
				actionYankMarkedAsJSON,
				fmt.Sprintf("Yank marked as JSON (%d)", len(m.markedSecrets)),
				"",
			})
			actions = append(actions, action{actionClearMarks, "Clear all marks", ""})
		}
	}

	if m.focus == versionsPane && m.hasSecret {
		if item, ok := m.versionsList.SelectedItem().(versionItem); ok {
			actions = append(actions, action{actionYankSecretVersion, fmt.Sprintf("Yank version value (%s)", item.version.Version), km.YankSecret.Short()})
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
		m.startLoading(m.focus, fmt.Sprintf("Fetching secret value for %s...", item.secret.Name))
		return m, tea.Batch(m.Spinner.Tick, yankSecretValueCmd(m.service, m.currentVault, item.secret.Name, ""))

	case actionYankMarkedAsJSON:
		names := m.sortedMarkedSecretNames()
		if len(names) == 0 {
			return m, nil
		}
		m.startLoading(m.focus, fmt.Sprintf("Fetching %d secret values...", len(names)))
		return m, tea.Batch(m.Spinner.Tick, yankMarkedSecretsAsJSONCmd(m.service, m.currentVault, names))

	case actionClearMarks:
		count := len(m.markedSecrets)
		for name := range m.markedSecrets {
			delete(m.markedSecrets, name)
		}
		m.refreshSecretSelectionDisplay()
		m.Notify(appshell.LevelInfo, fmt.Sprintf("Cleared %d marks", count))
		return m, nil

	case actionYankSecretVersion:
		item, ok := m.versionsList.SelectedItem().(versionItem)
		if !ok {
			return m, nil
		}
		m.startLoading(m.focus, fmt.Sprintf("Fetching secret value for %s (version %s)...", m.currentSecret.Name, item.version.Version))
		return m, tea.Batch(m.Spinner.Tick, yankSecretValueCmd(m.service, m.currentVault, m.currentSecret.Name, item.version.Version))

	case actionToggleMark:
		if m.focus == secretsPane {
			m.toggleCurrentSecretMark()
		}
		return m, nil

	case actionToggleVisualLine:
		if m.focus == secretsPane {
			m.toggleVisualLineMode()
		}
		return m, nil

	case actionRefresh:
		return m.refresh()

	case actionInspect:
		m.toggleInspect()
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
				m.HelpOverlay.Open("Key Vault Explorer Help", m.HelpSections())
			}
		}
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
			Hint:  s.actions[si].hint,
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
