// Package safego runs background work in goroutines that must not take
// the whole process down when they fail.
//
// Bubble Tea already recovers panics raised inside a tea.Cmd: it kills
// the program, restores the terminal and surfaces the failure through
// Program.Run. Goroutines the app starts on its own sit outside that
// net — the cache broker's fetch worker, the blob upload worker, the
// activity ticker, the Service Bus lock releases. An unrecovered panic
// there kills the process on the spot, so Run never returns and its
// cleanup never happens: the terminal is left in raw mode, on the
// alternate screen, with mouse reporting still on, and the stack trace
// is written into the alternate buffer where it vanishes the moment the
// user runs `reset`.
package safego

import (
	"fmt"
	"os"
	"runtime/debug"
	"strings"
	"sync"
)

// PanicHandler receives a panic that escaped a goroutine started by Go,
// along with the stack captured at the point of recovery. It runs on the
// panicking goroutine and must not panic itself.
type PanicHandler func(recovered any, stack []byte)

var (
	mu      sync.RWMutex
	handler PanicHandler
)

// SetPanicHandler installs the process-wide handler for panics escaping
// Go. Install it before starting any background work — typically right
// after the Bubble Tea program is constructed, so the handler can kill
// the program and let it restore the terminal.
func SetPanicHandler(h PanicHandler) {
	mu.Lock()
	defer mu.Unlock()
	handler = h
}

// Go runs fn in a new goroutine, routing any panic to the handler
// installed by SetPanicHandler rather than letting it abort the process.
func Go(fn func()) {
	go func() {
		defer func() {
			r := recover()
			if r == nil {
				return
			}
			report(r, debug.Stack())
		}()
		fn()
	}()
}

func report(recovered any, stack []byte) {
	mu.RLock()
	h := handler
	mu.RUnlock()

	if h != nil {
		h(recovered, stack)
		return
	}

	// No handler installed — a panic before the program started, or in a
	// test. Carrying on would leave the app running on whatever broken
	// state caused the panic, so report and stop. Line endings are
	// normalized in case the terminal is still in raw mode.
	fmt.Fprint(os.Stderr, CRLF(fmt.Sprintf("panic in background goroutine: %v\n\n%s\n", recovered, stack)))
	os.Exit(1)
}

// CRLF rewrites bare newlines so a message stays readable on a terminal
// that is still in raw mode, where a lone \n moves down a row without
// returning the cursor to column zero.
func CRLF(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\n", "\r\n")
}
