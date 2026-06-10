package kvapp

// paneName is a kvapp helper shared across files.

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
