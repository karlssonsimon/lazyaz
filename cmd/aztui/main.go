package main

import (
	"fmt"
	"os"

	"azure-storage/internal/azure"
	"azure-storage/internal/azure/blob"
	"azure-storage/internal/azure/keyvault"
	"azure-storage/internal/azure/servicebus"
	"azure-storage/internal/app"
	"azure-storage/internal/ui"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	cred, err := azure.NewDefaultCredential()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to initialize default azure credential: %v\n", err)
		os.Exit(1)
	}

	cfg := ui.LoadConfig("aztui")
	model := app.NewModel(
		blob.NewService(cred),
		servicebus.NewService(cred),
		keyvault.NewService(cred),
		cfg,
	)

	program := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := program.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "application error: %v\n", err)
		os.Exit(1)
	}
}
