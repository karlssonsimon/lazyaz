package sbapp

import (
	"github.com/karlssonsimon/lazyaz/internal/appshell"
	"github.com/karlssonsimon/lazyaz/internal/ui"

	tea "charm.land/bubbletea/v2"
)

// copyTargets builds the copy palette entries for the current scope.
func (m Model) copyTargets() []ui.CopyTarget {
	var targets []ui.CopyTarget

	// The message in view wins; otherwise the one under the cursor.
	if m.viewingMessage {
		targets = append(targets,
			ui.CopyTarget{Label: "Message ID", Value: m.selectedMessage.MessageID},
			ui.CopyTarget{Label: "Message body", Value: m.selectedMessage.FullBody},
		)
	} else if item, ok := m.messageList.SelectedItem().(messageItem); ok && m.focus == messagesPane {
		targets = append(targets,
			ui.CopyTarget{Label: "Message ID", Value: item.message.MessageID},
			ui.CopyTarget{Label: "Message body", Value: item.message.FullBody},
		)
	}
	if item, ok := m.entitiesList.SelectedItem().(entityItem); ok {
		targets = append(targets, ui.CopyTarget{Label: "Entity", Value: item.entity.Name})
	}
	if m.hasNamespace {
		targets = append(targets, ui.CopyTarget{Label: "Namespace", Value: m.currentNS.Name})
		if m.currentNS.FQDN != "" {
			targets = append(targets, ui.CopyTarget{Label: "Namespace FQDN", Value: m.currentNS.FQDN})
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
