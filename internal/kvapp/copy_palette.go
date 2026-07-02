package kvapp

import (
	"github.com/karlssonsimon/lazyaz/internal/appshell"
	"github.com/karlssonsimon/lazyaz/internal/ui"

	tea "charm.land/bubbletea/v2"
)

// copyTargets builds the copy palette entries for the current scope.
// Values are identifiers only — secret/key material stays behind the
// explicit yank flows so the palette can't leak it by accident.
func (m Model) copyTargets() []ui.CopyTarget {
	var targets []ui.CopyTarget

	switch item := m.secretsList.SelectedItem().(type) {
	case secretItem:
		targets = append(targets, ui.CopyTarget{Label: "Secret name", Value: item.secret.Name})
	case certItem:
		targets = append(targets, ui.CopyTarget{Label: "Certificate name", Value: item.cert.Name})
	case keyItem:
		targets = append(targets, ui.CopyTarget{Label: "Key name", Value: item.key.Name})
	}
	if m.focus == versionsPane {
		if item, ok := m.versionsList.SelectedItem().(versionItem); ok {
			targets = append(targets, ui.CopyTarget{Label: "Version", Value: item.version.Version})
		}
	}
	if m.hasVault {
		targets = append(targets, ui.CopyTarget{Label: "Vault", Value: m.currentVault.Name})
		if m.currentVault.VaultURI != "" {
			targets = append(targets, ui.CopyTarget{Label: "Vault URI", Value: m.currentVault.VaultURI})
		}
	}
	return targets
}

// openCopyPalette opens the palette, or explains why there's nothing
// to copy yet.
func (m Model) openCopyPalette() (Model, tea.Cmd) {
	targets := m.copyTargets()
	if len(targets) == 0 {
		m.Notify(appshell.LevelInfo, "Nothing to copy here yet")
		return m, nil
	}
	m.copyOverlay.Open(targets)
	return m, nil
}

// copyToClipboard copies text to the system clipboard.
func (m Model) copyToClipboard(text string) (Model, tea.Cmd) {
	return m, func() tea.Msg {
		if err := ui.WriteClipboard(text); err != nil {
			return clipboardMsg{err: err}
		}
		return clipboardMsg{text: text}
	}
}
