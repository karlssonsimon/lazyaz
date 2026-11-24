package acapp

import (
	"fmt"
	"strings"

	"github.com/karlssonsimon/lazyaz/internal/azure/appconfig"
	"github.com/karlssonsimon/lazyaz/internal/keymap"
	"github.com/karlssonsimon/lazyaz/internal/ui"

	tea "charm.land/bubbletea/v2"
)

type actionID int

const (
	actionEditValue actionID = iota
	actionEditInEditor
	actionCreate
	actionCreateFlag
	actionToggleFlag
	actionDelete
	actionYankValue
	actionCopyPalette
	actionOpenKeyVaultReference
	actionInspect
	actionRefresh
	actionSubscriptionPicker
	actionThemePicker
	actionHelp
)

type action struct {
	id    actionID
	label string
	hint  string
}

type actionMenuState struct {
	ui.SearchableOverlay[action]
}

func (s *actionMenuState) open(actions []action) {
	s.Open(actions, func(a action) string { return a.label })
}

func (s *actionMenuState) handleKey(key string, km keymap.Keymap) (selected bool, act action) {
	switch {
	case km.ThemeUp.Matches(key):
		s.Move(-1)
		return false, action{}
	case km.ThemeDown.Matches(key):
		s.Move(1)
		return false, action{}
	case km.ThemeApply.Matches(key):
		if a, ok := s.Selected(); ok {
			s.Close()
			return true, a
		}
		return false, action{}
	case km.ThemeCancel.Matches(key):
		s.Cancel()
		return false, action{}
	case key == "ctrl+v":
		if text := ui.ReadClipboard(); text != "" {
			s.TypeText(text)
		}
		return false, action{}
	}
	s.HandleQueryKey(key)
	return false, action{}
}

// buildActions lists what the menu offers for the focused pane.
func (m Model) buildActions() []action {
	km := m.Keymap
	var actions []action

	if setting, ok := m.actionSetting(); ok {
		label := setting.DisplayKey()
		if setting.Label != "" {
			label += " (" + setting.Label + ")"
		}
		if ff, ok := appconfig.ParseFeatureFlag(setting); ok {
			toggle := action{actionToggleFlag, fmt.Sprintf("Turn flag %s %s", ff.ID, onOff(!ff.Enabled)), ""}
			if setting.ReadOnly {
				toggle.label += " — locked, read-only"
			}
			actions = append(actions, toggle)
		}
		inline := action{actionEditValue, fmt.Sprintf("Edit value of %s", label), ""}
		editor := action{actionEditInEditor, fmt.Sprintf("Edit value of %s in $EDITOR", label), ""}
		if setting.ReadOnly {
			inline.label += " — locked, read-only"
			editor.label += " — locked, read-only"
		}
		// JSON and multi-line values are better off in a real editor;
		// offer it first for those.
		if _, isJSON := editorText(setting); isJSON || strings.Contains(setting.Value, "\n") {
			actions = append(actions, editor, inline)
		} else {
			actions = append(actions, inline, editor)
		}
		actions = append(actions, action{actionYankValue, "Yank value", km.YankSecret.Short()})
		if ref, ok := setting.KeyVaultReference(); ok {
			actions = append(actions, action{actionOpenKeyVaultReference, fmt.Sprintf("Open %s/%s in Key Vault", ref.VaultName, ref.SecretName), ""})
		}
		del := action{actionDelete, fmt.Sprintf("Delete %s", label), ""}
		if setting.ReadOnly {
			del.label += " — locked, read-only"
		}
		actions = append(actions, del)
	}
	if m.hasStore && m.focus != storesPane {
		actions = append(actions,
			action{actionCreate, fmt.Sprintf("Create key-value in %s", m.currentStore.Name), ""},
			action{actionCreateFlag, fmt.Sprintf("Create feature flag in %s", m.currentStore.Name), ""},
		)
	}
	actions = append(actions,
		action{actionCopyPalette, "Copy palette", km.CopyPalette.Short()},
		action{actionInspect, "Toggle details panel", km.Inspect.Short()},
		action{actionRefresh, "Refresh", km.RefreshScope.Short()},
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
	case actionEditValue:
		return m.openEditValueForm()
	case actionEditInEditor:
		return m.openEditValueInEditor()
	case actionCreate:
		return m.openCreateForm()
	case actionCreateFlag:
		return m.openCreateFlagForm()
	case actionToggleFlag:
		return m.openToggleFlagConfirm()
	case actionDelete:
		return m.openDeleteConfirm()
	case actionYankValue:
		return m.yankValue()
	case actionCopyPalette:
		return m.openCopyPalette()
	case actionOpenKeyVaultReference:
		return m.openKeyVaultReference()
	case actionInspect:
		m.toggleInspect()
		return m, nil
	case actionRefresh:
		return m.refresh()
	case actionSubscriptionPicker:
		m.SubOverlay.Open()
		return m, nil
	case actionThemePicker:
		m.ThemeOverlay.Open()
		return m, nil
	case actionHelp:
		if m.HelpOverlay.Active {
			m.HelpOverlay.Close()
		} else {
			m.HelpOverlay.Open("Azure App Configuration Explorer Help", m.HelpSections())
		}
		return m, nil
	}
	return m, nil
}

func (m Model) renderActionMenu(base string) string {
	s := &m.actionMenu
	visible := s.Visible()
	items := make([]ui.OverlayItem, len(visible))
	for i, a := range visible {
		items[i] = ui.OverlayItem{Label: a.label, Hint: a.hint}
	}
	cfg := ui.OverlayListConfig{
		Title:       "Actions",
		Query:       s.Query,
		QueryCursor: s.QueryCaret,
		Cursor:      m.Cursor,
		CloseHint:   m.Keymap.Cancel.Short(),
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
