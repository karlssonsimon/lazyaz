package main

import (
	"fmt"
	"os"

	"azure-storage/internal/app"
	"azure-storage/internal/azure"
	"azure-storage/internal/azure/blob"
	"azure-storage/internal/azure/keyvault"
	"azure-storage/internal/azure/servicebus"
	"azure-storage/internal/keymap"
	"azure-storage/internal/ui"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	cred, err := azure.NewDefaultCredential()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to initialize default azure credential: %v\n", err)
		os.Exit(1)
	}

	cfg := ui.LoadConfig()
	km := keymap.Load(ui.ConfigDir())
	runtime, err := app.NewRuntime(
		blob.NewService(cred),
		servicebus.NewService(cred),
		keyvault.NewService(cred),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to start app runtime: %v\n", err)
		os.Exit(1)
	}
	defer runtime.Close()

	model := app.NewModel(runtime, cfg, km)

	program := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := program.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "application error: %v\n", err)
		os.Exit(1)
	}
}
