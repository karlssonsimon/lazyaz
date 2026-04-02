package kvapp

import (
	"fmt"

	"azure-storage/internal/azure"
	"azure-storage/internal/ui"
)

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

func subscriptionDisplayName(sub azure.Subscription) string {
	return ui.SubscriptionDisplayName(sub)
}

func (m Model) vaultsPaneTitle() string {
	title := "Vaults"
	if m.hasSubscription {
		title = fmt.Sprintf("Vaults · %s", subscriptionDisplayName(m.currentSub))
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
