package main

import (
	"fmt"
	"os"

	"azure-storage/internal/app"
	"azure-storage/internal/azure"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	cred, err := azure.NewDefaultCredential()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to initialize default azure credential: %v\n", err)
		os.Exit(1)
	}

	program := tea.NewProgram(app.NewModel(azure.NewService(cred)), tea.WithAltScreen())
	if _, err := program.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "application error: %v\n", err)
		os.Exit(1)
	}
}
