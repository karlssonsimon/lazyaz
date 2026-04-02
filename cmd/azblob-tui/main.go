package main

import (
	"fmt"
	"os"

	"azure-storage/internal/azure"
	"azure-storage/internal/azure/blob"
	"azure-storage/internal/blobapp"
	"azure-storage/internal/cache"
	"azure-storage/internal/ui"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	cred, err := azure.NewDefaultCredential()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to initialize default azure credential: %v\n", err)
		os.Exit(1)
	}

	db := openCacheDB()
	if db != nil {
		defer db.Close()
	}

	cfg := ui.LoadConfig()
	program := tea.NewProgram(blobapp.NewModel(blob.NewService(cred), cfg, db), tea.WithAltScreen())
	if _, err := program.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "application error: %v\n", err)
		os.Exit(1)
	}
}

func openCacheDB() *cache.DB {
	path, err := cache.DefaultDBPath()
	if err != nil {
		return nil
	}
	db, err := cache.OpenDB(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: cache unavailable: %v\n", err)
		return nil
	}
	return db
}
