package main

import (
	"flag"
	"fmt"
	"os"
	"sync/atomic"

	"github.com/karlssonsimon/lazyaz/internal/app"
	"github.com/karlssonsimon/lazyaz/internal/azure"
	"github.com/karlssonsimon/lazyaz/internal/azure/blob"
	"github.com/karlssonsimon/lazyaz/internal/azure/keyvault"
	"github.com/karlssonsimon/lazyaz/internal/azure/servicebus"
	"github.com/karlssonsimon/lazyaz/internal/cache"
	"github.com/karlssonsimon/lazyaz/internal/keymap"
	"github.com/karlssonsimon/lazyaz/internal/safego"
	"github.com/karlssonsimon/lazyaz/internal/ui"

	tea "charm.land/bubbletea/v2"
)

var version = "dev"

func main() {
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.BoolVar(showVersion, "v", false, "print version and exit (shorthand)")
	flag.Parse()

	if *showVersion {
		fmt.Println(version)
		return
	}

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
	model := app.NewModel(
		blob.NewService(cred),
		servicebus.NewService(cred),
		keyvault.NewService(cred),
		cfg,
		db,
		km,
	)

	program := tea.NewProgram(model, tea.WithFilter(ui.MouseEventFilter))

	// Background workers can't unwind into Run, so route their panics
	// here: Kill puts the terminal back to normal and makes Run return,
	// and only then is it safe to print the trace where the user can
	// actually read and copy it.
	var crash atomic.Pointer[backgroundPanic]
	safego.SetPanicHandler(func(recovered any, stack []byte) {
		crash.CompareAndSwap(nil, &backgroundPanic{recovered: recovered, stack: stack})
		program.Kill()
	})

	_, runErr := program.Run()

	if c := crash.Load(); c != nil {
		fmt.Fprintf(os.Stderr, "lazyaz crashed in a background worker: %v\n\n%s\n", c.recovered, c.stack)
		os.Exit(1)
	}
	if runErr != nil {
		fmt.Fprintf(os.Stderr, "application error: %v\n", runErr)
		os.Exit(1)
	}
}

// backgroundPanic carries a panic from a safego worker back to main so
// it can be printed after the terminal has been restored.
type backgroundPanic struct {
	recovered any
	stack     []byte
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
