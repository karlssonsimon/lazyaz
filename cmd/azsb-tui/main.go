package main

import (
	"fmt"
	"os"

	"azure-storage/internal/azure"
	"azure-storage/internal/sbapp"
	"azure-storage/internal/servicebus"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	cred, err := azure.NewDefaultCredential()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to initialize default azure credential: %v\n", err)
		os.Exit(1)
	}

	cfg := sbapp.LoadConfig()
	program := tea.NewProgram(sbapp.NewModel(servicebus.NewService(cred), cfg), tea.WithAltScreen())
	if _, err := program.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "application error: %v\n", err)
		os.Exit(1)
	}
}
