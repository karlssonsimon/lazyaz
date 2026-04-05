package main

import (
	"flag"
	"fmt"
	"os"

	"azure-storage/internal/azure"
	"azure-storage/internal/azure/keyvault"
	"azure-storage/internal/cache"
	kvrpc "azure-storage/internal/kvapp/rpc"
)

func main() {
	socketPath := flag.String("socket", "", "unix socket path to listen on")
	cachePath := flag.String("cache-db", "", "path to sqlite cache database")
	flag.Parse()
	if *socketPath == "" {
		fmt.Fprintln(os.Stderr, "missing required --socket path")
		os.Exit(1)
	}
	cred, err := azure.NewDefaultCredential()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to initialize default azure credential: %v\n", err)
		os.Exit(1)
	}
	db := openServerCacheDB(*cachePath)
	if db != nil {
		defer db.Close()
	}
	server, err := kvrpc.NewServer(*socketPath, keyvault.NewService(cred), db)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to start key vault daemon: %v\n", err)
		os.Exit(1)
	}
	defer server.Close()
	if err := server.Serve(); err != nil {
		fmt.Fprintf(os.Stderr, "key vault daemon stopped: %v\n", err)
		os.Exit(1)
	}
}

func openServerCacheDB(explicitPath string) *cache.DB {
	path := explicitPath
	if path == "" {
		var err error
		path, err = cache.DefaultServerDBPath("kv")
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: cache path unavailable: %v\n", err)
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
