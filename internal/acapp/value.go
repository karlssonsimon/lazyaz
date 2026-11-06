package acapp

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/karlssonsimon/lazyaz/internal/appshell"
	"github.com/karlssonsimon/lazyaz/internal/ui"
	"github.com/karlssonsimon/lazyaz/internal/vim"

	tea "charm.land/bubbletea/v2"
)

// valueGutterMinDigits is the minimum digit width of the line-number
// gutter beside the value.
const valueGutterMinDigits = 3

// valueText is what the value pane shows for the current setting: the
// raw value, or for a Key Vault reference, the reference explained.
func (m Model) valueText() string {
	s := m.currentSetting
	if ref, ok := s.KeyVaultReference(); ok {
		return strings.Join([]string{
			"Key Vault reference",
			"",
			"Vault:   " + ref.VaultName,
			"Secret:  " + ref.SecretName,
			"Version: " + ui.EmptyToDash(ref.Version),
			"URI:     " + ref.VaultURI + "/secrets/" + ref.SecretName,
			"",
			"Enter opens the secret in a Key Vault tab.",
		}, "\n")
	}
	return s.Value
}

// setValueContent renders the current value into the viewport: JSON is
// pretty-printed and highlighted, anything else is shown as is.
func (m *Model) setValueContent() {
	text := m.valueText()
	if m.currentSetting.Value != "" && !m.currentSetting.IsKeyVaultReference() && json.Valid([]byte(m.currentSetting.Value)) {
		text = m.Styles.Syntax.HighlightJSON(m.currentSetting.Value)
	}
	if text == "" {
		text = m.Styles.Muted.Render("(empty value)")
	}
	m.valueViewport.SetContent(text)
	m.valueViewport.SetXOffset(0)
}

// yankValue copies the current setting's raw value to the clipboard.
func (m Model) yankValue() (Model, tea.Cmd) {
	var setting = m.currentSetting
	if !m.viewingValue {
		item, ok := m.settingsList.SelectedItem().(settingItem)
		if !ok {
			return m, nil
		}
		setting = item.setting
	}
	if setting.Value == "" {
		m.Notify(appshell.LevelInfo, "Value is empty")
		return m, nil
	}
	return m.copyToClipboard(setting.Value)
}

// valueTitle is the line above the value: key, label, and content type.
func (m Model) valueTitle() string {
	s := m.currentSetting
	parts := []string{s.DisplayKey()}
	if s.Label != "" {
		parts = append(parts, s.Label)
	}
	if s.ContentType != "" && !s.IsKeyVaultReference() && !s.IsFeatureFlag() {
		parts = append(parts, s.ContentType)
	}
	if s.ReadOnly {
		parts = append(parts, "read-only")
	}
	return strings.Join(parts, " · ")
}

// handleValueKey handles keys while the value pane is focused: viewport
// scrolling with counts, yank, and the way back.
func (m Model) handleValueKey(msg tea.KeyMsg, key string) (Model, tea.Cmd, bool) {
	switch m.vimr.GG(m.Keymap.JumpTopPrefix, key, true) {
	case vim.ChordFired:
		m.valueViewport.GotoTop()
		return m, nil, true
	case vim.ChordArmed:
		return m, nil, true
	}
	if m.vimr.Digit(key) {
		return m, nil, true
	}
	km := m.Keymap
	switch {
	case km.CursorDown.Matches(key):
		m.valueViewport.ScrollDown(m.vimr.TakeCount())
	case km.CursorUp.Matches(key):
		m.valueViewport.ScrollUp(m.vimr.TakeCount())
	case km.HalfPageDown.Matches(key):
		for i := 0; i < m.vimr.TakeCount(); i++ {
			m.valueViewport.HalfPageDown()
		}
	case km.HalfPageUp.Matches(key):
		for i := 0; i < m.vimr.TakeCount(); i++ {
			m.valueViewport.HalfPageUp()
		}
	case km.FullPageDown.Matches(key):
		for i := 0; i < m.vimr.TakeCount(); i++ {
			m.valueViewport.PageDown()
		}
	case km.FullPageUp.Matches(key):
		for i := 0; i < m.vimr.TakeCount(); i++ {
			m.valueViewport.PageUp()
		}
	case km.JumpBottom.Matches(key):
		m.valueViewport.GotoBottom()
	case km.ScrollLineDown.Matches(key):
		m.valueViewport.ScrollDown(m.vimr.TakeCount())
	case km.ScrollLineUp.Matches(key):
		m.valueViewport.ScrollUp(m.vimr.TakeCount())
	case km.NavigateLeft.Matches(key), km.BackspaceUp.Matches(key), km.Cancel.Matches(key):
		m.vimr.ClearCount()
		m.transitionTo(settingsPane)
	default:
		m.vimr.ClearCount()
		return m, nil, false
	}
	return m, nil, true
}

func (m Model) valueStatusHint() string {
	if ref, ok := m.currentSetting.KeyVaultReference(); ok {
		return fmt.Sprintf("Key Vault reference to %s/%s — %s opens it", ref.VaultName, ref.SecretName, m.Keymap.OpenFocused.Short())
	}
	return ""
}
