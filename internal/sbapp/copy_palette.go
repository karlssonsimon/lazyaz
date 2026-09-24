package sbapp

import (
	"github.com/karlssonsimon/lazyaz/internal/appshell"
	"github.com/karlssonsimon/lazyaz/internal/azure/servicebus"
	"github.com/karlssonsimon/lazyaz/internal/ui"

	tea "charm.land/bubbletea/v2"
)

// copyTargets builds the copy palette entries for the current scope.
func (m Model) copyTargets() []ui.CopyTarget {
	var targets []ui.CopyTarget

	// The message in view wins; otherwise the one under the cursor.
	if m.viewingMessage {
		targets = append(targets, messageCopyTargets(m.selectedMessage)...)
	} else if item, ok := m.messageList.SelectedItem().(messageItem); ok && m.focus == messagesPane {
		targets = append(targets, messageCopyTargets(item.message)...)
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

// messageCopyTargets lists what one message offers to the palette: ID
// and body first, then the two property tables, then each custom
// property on its own so a single value can go straight into a search.
func messageCopyTargets(msg servicebus.PeekedMessage) []ui.CopyTarget {
	targets := []ui.CopyTarget{
		{Label: "Message ID", Value: msg.MessageID},
		{Label: "Message body", Value: msg.FullBody},
		{Label: "Broker properties", Value: brokerPropertiesText(msg)},
	}
	if len(msg.AppProperties) > 0 {
		targets = append(targets, ui.CopyTarget{Label: "Custom properties", Value: customPropertiesText(msg)})
		for _, k := range sortedKeys(msg.AppProperties) {
			targets = append(targets, ui.CopyTarget{Label: "Property " + k, Value: msg.AppProperties[k]})
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
