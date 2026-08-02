package safego

import (
	"strings"
	"sync"
	"testing"
)

func TestGoRoutesPanicToHandler(t *testing.T) {
	var (
		wg        sync.WaitGroup
		recovered any
		stack     []byte
	)
	wg.Add(1)
	SetPanicHandler(func(r any, s []byte) {
		recovered = r
		stack = s
		wg.Done()
	})
	t.Cleanup(func() { SetPanicHandler(nil) })

	Go(func() { panic("worker exploded") })
	wg.Wait()

	if recovered != "worker exploded" {
		t.Errorf("recovered = %v, want %q", recovered, "worker exploded")
	}
	if !strings.Contains(string(stack), "safego.Go") {
		t.Errorf("stack does not point at the safego goroutine:\n%s", stack)
	}
}

// A goroutine that returns normally must not touch the handler —
// otherwise every completed background fetch would look like a crash.
func TestGoDoesNotInvokeHandlerOnCleanReturn(t *testing.T) {
	var (
		wg     sync.WaitGroup
		called bool
		mu     sync.Mutex
	)
	SetPanicHandler(func(any, []byte) {
		mu.Lock()
		called = true
		mu.Unlock()
	})
	t.Cleanup(func() { SetPanicHandler(nil) })

	wg.Add(1)
	Go(func() { wg.Done() })
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	if called {
		t.Error("handler ran for a goroutine that returned normally")
	}
}

// The handler runs on the panicking goroutine, so a panic in one worker
// must not stop another worker from reporting its own.
func TestGoHandlesConcurrentPanics(t *testing.T) {
	const workers = 8

	var (
		wg    sync.WaitGroup
		mu    sync.Mutex
		count int
	)
	wg.Add(workers)
	SetPanicHandler(func(any, []byte) {
		mu.Lock()
		count++
		mu.Unlock()
		wg.Done()
	})
	t.Cleanup(func() { SetPanicHandler(nil) })

	for i := 0; i < workers; i++ {
		Go(func() { panic("boom") })
	}
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	if count != workers {
		t.Errorf("handler ran %d times, want %d", count, workers)
	}
}

func TestCRLF(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"bare newlines", "a\nb\n", "a\r\nb\r\n"},
		{"already crlf is not doubled", "a\r\nb\r\n", "a\r\nb\r\n"},
		{"mixed", "a\r\nb\nc", "a\r\nb\r\nc"},
		{"no newlines", "abc", "abc"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CRLF(tt.in); got != tt.want {
				t.Errorf("CRLF(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
