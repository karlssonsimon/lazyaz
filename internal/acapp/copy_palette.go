package acapp

import (
	"github.com/karlssonsimon/lazyaz/internal/appshell"
	"github.com/karlssonsimon/lazyaz/internal/azure/appconfig"
	"github.com/karlssonsimon/lazyaz/internal/ui"

	tea "charm.land/bubbletea/v2"
)

// copyTargets lists what the palette offers: the setting under the
// cursor (or in view), then the store.
func (m Model) copyTargets() []ui.CopyTarget {
	var targets []ui.CopyTarget

	var setting appconfig.Setting
	have := false
	if m.viewingValue && m.focus == valuePane {
		setting, have = m.currentSetting, true
	} else if item, ok := m.settingsList.SelectedItem().(settingItem); ok && m.focus != storesPane {
		setting, have = item.setting, true
	}
	if have {
		targets = append(targets,
			ui.CopyTarget{Label: "Key", Value: setting.Key},
			ui.CopyTarget{Label: "Value", Value: setting.Value},
		)
		if setting.Label != "" {
			targets = append(targets, ui.CopyTarget{Label: "Label", Value: setting.Label})
		}
		if ref, ok := setting.KeyVaultReference(); ok {
			targets = append(targets,
				ui.CopyTarget{Label: "Vault", Value: ref.VaultName},
				ui.CopyTarget{Label: "Secret", Value: ref.SecretName},
				ui.CopyTarget{Label: "Secret URI", Value: ref.VaultURI + "/secrets/" + ref.SecretName},
			)
		}
	}
	if m.hasStore {
		targets = append(targets,
			ui.CopyTarget{Label: "Store", Value: m.currentStore.Name},
			ui.CopyTarget{Label: "Endpoint", Value: m.currentStore.Endpoint},
		)
	}
	return targets
}

func (m Model) openCopyPalette() (Model, tea.Cmd) {
	targets := m.copyTargets()
	if len(targets) == 0 {
		m.Notify(appshell.LevelInfo, "Nothing to copy here yet")
		return m, nil
	}
	m.copyOverlay.Open(targets)
	return m, nil
}

func (m Model) copyToClipboard(text string) (Model, tea.Cmd) {
	return m, func() tea.Msg {
		if err := ui.WriteClipboard(text); err != nil {
			return clipboardMsg{err: err}
		}
		return clipboardMsg{text: text}
	}
}
