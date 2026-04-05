package app

import tea "github.com/charmbracelet/bubbletea"

// TabKind identifies which type of explorer a tab contains.
type TabKind int

const (
	TabBlob TabKind = iota
	TabServiceBus
	TabKeyVault
)

func (k TabKind) String() string {
	switch k {
	case TabBlob:
		return "Blob"
	case TabServiceBus:
		return "Service Bus"
	case TabKeyVault:
		return "Key Vault"
	default:
		return "Unknown"
	}
}

// Tab represents a single tab in the application.
type Tab struct {
	ID    int
	Kind  TabKind
	Model tea.Model
}
