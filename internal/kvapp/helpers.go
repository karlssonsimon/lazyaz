package kvapp

import (
	"fmt"

	"github.com/karlssonsimon/lazyaz/internal/ui"
)

// startLoading dismisses any active spinner, marks the pane as loading,
// and pushes a new spinner notification. This prevents orphaned spinners
// when the user navigates away before a load finishes.
func (m *Model) startLoading(pane int, message string) {
	if m.Loading {
		m.ClearLoading()
		m.DismissSpinner(m.loadingSpinnerID)
	}
	m.SetLoading(pane)
	m.loadingSpinnerID = m.NotifySpinner(message)
}

func paneName(pane int) string {
	switch pane {
	case vaultsPane:
		return "vaults"
	case secretsPane:
		return "secrets"
	case versionsPane:
		return "versions"
	default:
		return "items"
	}
}

func (m Model) vaultsPaneTitle() string {
	title := "Vaults"
	if m.HasSubscription {
		title = fmt.Sprintf("Vaults · %s", ui.SubscriptionDisplayName(m.CurrentSub))
	}
	if len(m.vaults) > 0 {
		title = fmt.Sprintf("%s (%d)", title, len(m.vaults))
	}
	return title
}

func (m Model) secretsPaneTitle() string {
	title := "Secrets"
	if m.hasVault {
		title = fmt.Sprintf("Secrets · %s", m.currentVault.Name)
	}
	if len(m.secrets) > 0 {
		title = fmt.Sprintf("%s (%d)", title, len(m.secrets))
	}
	if len(m.markedSecrets) > 0 {
		title = fmt.Sprintf("%s | marked:%d", title, len(m.markedSecrets))
	}
	if m.visualLineMode {
		title = fmt.Sprintf("%s | VISUAL:%d", title, len(m.visualSelectionSecretNames()))
	}
	return title
}

func (m Model) versionsPaneTitle() string {
	title := "Versions"
	if m.hasSecret {
		title = fmt.Sprintf("Versions · %s", m.currentSecret.Name)
	}
	if len(m.versions) > 0 {
		title = fmt.Sprintf("%s (%d)", title, len(m.versions))
	}
	return title
}
