package ui

import (
	"testing"

	"github.com/karlssonsimon/lazyaz/internal/keymap"
)

func TestSearchBarOpenAndAccept(t *testing.T) {
	km := keymap.Default()
	var s SearchBar

	s.Open(SearchForward)
	if !s.InputOpen {
		t.Fatal("input should be open after Open")
	}
	if s.Direction != SearchForward {
		t.Errorf("direction = %v, want SearchForward", s.Direction)
	}

	for _, key := range []string{"e", "r", "r"} {
		if consumed, _ := s.HandleKey(key, km); !consumed {
			t.Fatalf("key %q was not consumed by the open prompt", key)
		}
	}
	if s.Input.Value != "err" {
		t.Fatalf("query = %q, want %q", s.Input.Value, "err")
	}

	consumed, submitted := s.HandleKey("enter", km)
	if !consumed || !submitted {
		t.Fatalf("enter gave consumed=%v submitted=%v, want both true", consumed, submitted)
	}
	if err := s.Accept(); err != nil {
		t.Fatalf("Accept: %v", err)
	}
	if s.InputOpen {
		t.Error("input should be closed after Accept")
	}
	if !s.Active() {
		t.Error("bar should be active after a successful Accept")
	}
	if s.Pattern.Query != "err" {
		t.Errorf("pattern query = %q, want %q", s.Pattern.Query, "err")
	}
}

// A typo must not throw away the search that was already working.
func TestSearchBarAcceptFailureKeepsPreviousPattern(t *testing.T) {
	var s SearchBar

	s.Open(SearchForward)
	s.Input.SetValue("good")
	if err := s.Accept(); err != nil {
		t.Fatalf("Accept: %v", err)
	}

	s.Open(SearchForward)
	s.Input.SetValue("[bad")
	if err := s.Accept(); err == nil {
		t.Fatal("Accept of an invalid regex returned no error")
	}
	if s.Pattern.Query != "good" {
		t.Errorf("pattern query = %q, want the previous %q", s.Pattern.Query, "good")
	}
	if !s.InputOpen {
		t.Error("prompt should stay open after a failed Accept so the query can be fixed")
	}
}

// Esc abandons the prompt but leaves the last executed search repeatable.
func TestSearchBarCancelKeepsExecutedPattern(t *testing.T) {
	km := keymap.Default()
	var s SearchBar

	s.Open(SearchForward)
	s.Input.SetValue("keep")
	if err := s.Accept(); err != nil {
		t.Fatalf("Accept: %v", err)
	}

	s.Open(SearchBackward)
	s.Input.SetValue("discard")
	if consumed, submitted := s.HandleKey("esc", km); !consumed || submitted {
		t.Fatalf("esc gave consumed=%v submitted=%v, want true/false", consumed, submitted)
	}
	if s.InputOpen {
		t.Error("input should be closed after esc")
	}
	if s.Pattern.Query != "keep" {
		t.Errorf("pattern query = %q, want %q", s.Pattern.Query, "keep")
	}
}

func TestSearchBarClosedBarIgnoresKeys(t *testing.T) {
	km := keymap.Default()
	var s SearchBar

	if consumed, submitted := s.HandleKey("x", km); consumed || submitted {
		t.Errorf("closed bar consumed a key: consumed=%v submitted=%v", consumed, submitted)
	}
}

func TestSearchDirection(t *testing.T) {
	if got := SearchForward.Prompt(); got != "/" {
		t.Errorf("forward prompt = %q, want /", got)
	}
	if got := SearchBackward.Prompt(); got != "?" {
		t.Errorf("backward prompt = %q, want ?", got)
	}
	if SearchForward.Opposite() != SearchBackward || SearchBackward.Opposite() != SearchForward {
		t.Error("Opposite does not flip the direction")
	}
}

func TestSearchBarClear(t *testing.T) {
	var s SearchBar
	s.Open(SearchForward)
	s.Input.SetValue("gone")
	if err := s.Accept(); err != nil {
		t.Fatalf("Accept: %v", err)
	}

	s.Clear()
	if s.Active() {
		t.Error("bar still active after Clear")
	}
	if s.InputOpen {
		t.Error("input still open after Clear")
	}
}
