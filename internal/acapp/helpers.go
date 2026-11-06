package acapp

func paneName(pane int) string {
	switch pane {
	case storesPane:
		return "stores"
	case settingsPane:
		return "settings"
	case valuePane:
		return "value"
	default:
		return "items"
	}
}
