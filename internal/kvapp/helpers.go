package kvapp

// startLoading and paneName are kvapp helpers shared across files.

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
	case kindPane:
		return "kind"
	case secretsPane:
		return "secrets"
	case versionsPane:
		return "versions"
	default:
		return "items"
	}
}

