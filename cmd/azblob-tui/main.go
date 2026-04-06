package main

import (
	"fmt"
	"os"

	"github.com/karlssonsimon/lazyaz/internal/azure"
	"github.com/karlssonsimon/lazyaz/internal/azure/blob"
	"github.com/karlssonsimon/lazyaz/internal/blobapp"
	"github.com/karlssonsimon/lazyaz/internal/cache"
	"github.com/karlssonsimon/lazyaz/internal/keymap"
	"github.com/karlssonsimon/lazyaz/internal/ui"

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
	km := keymap.Load(ui.ConfigDir())
	program := tea.NewProgram(blobapp.NewModelWithKeyMap(blob.NewService(cred), cfg, km, db), tea.WithAltScreen())
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
