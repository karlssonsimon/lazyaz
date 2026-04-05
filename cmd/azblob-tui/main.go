package main

import (
	"flag"
	"fmt"
	"os"

	"azure-storage/internal/azure"
	"azure-storage/internal/azure/blob"
	"azure-storage/internal/blobapp"
	blobrpc "azure-storage/internal/blobapp/rpc"
	"azure-storage/internal/cache"
	"azure-storage/internal/keymap"
	"azure-storage/internal/ui"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	socketPath := flag.String("socket", "", "connect to an existing azblobd unix socket")
	cachePath := flag.String("cache-db", "", "path to sqlite cache database for local mode")
	flag.Parse()

	cfg := ui.LoadConfig()
	km := keymap.Load(ui.ConfigDir())

	if *socketPath != "" {
		client, err := blobrpc.Dial(*socketPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to connect to blob daemon: %v\n", err)
			os.Exit(1)
		}
		defer client.Close()
		program := tea.NewProgram(blobapp.NewRPCModel(client, cfg, km), tea.WithAltScreen())
		if _, err := program.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "application error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	cred, err := azure.NewDefaultCredential()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to initialize default azure credential: %v\n", err)
		os.Exit(1)
	}

	db := openCacheDB(*cachePath)
	if db != nil {
		defer db.Close()
	}

	program := tea.NewProgram(blobapp.NewModelWithKeyMap(blob.NewService(cred), cfg, km, db), tea.WithAltScreen())
	if _, err := program.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "application error: %v\n", err)
		os.Exit(1)
	}
}

func openCacheDB(explicitPath string) *cache.DB {
	path := explicitPath
	if path == "" {
		var err error
		path, err = cache.DefaultDBPath()
		if err != nil {
			return nil
		}
	}
	db, err := cache.OpenDB(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: cache unavailable: %v\n", err)
		return nil
	}
	return db
}
