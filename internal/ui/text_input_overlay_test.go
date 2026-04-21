package ui

import "testing"

func TestTextInputClosedByDefault(t *testing.T) {
	var s TextInputState
	if s.Active {
		t.Fatalf("zero value must be inactive")
	}
	if got := s.HandleKey("a"); got.Action != TextInputActionNone {
		t.Fatalf("inactive HandleKey: want None, got %v", got.Action)
	}
}

func TestTextInputOpenSetsFields(t *testing.T) {
	var s TextInputState
	s.Open("Rename blob", "new name", "report.csv", nil)
	if !s.Active {
		t.Fatalf("Open should set Active=true")
	}
	if s.Value != "report.csv" || s.Title != "Rename blob" {
		t.Fatalf("Open did not populate fields: %+v", s)
	}
}

func TestTextInputTypingAppends(t *testing.T) {
	var s TextInputState
	s.Open("title", "", "", nil)
	s.HandleKey("a")
	s.HandleKey("b")
	s.HandleKey("c")
	if s.Value != "abc" {
		t.Fatalf("after typing: want 'abc', got %q", s.Value)
	}
}

func TestTextInputBackspaceRemovesLastRune(t *testing.T) {
	var s TextInputState
	s.Open("title", "", "abc", nil)
	s.HandleKey("backspace")
	if s.Value != "ab" {
		t.Fatalf("after backspace: want 'ab', got %q", s.Value)
	}
	s.HandleKey("backspace")
	s.HandleKey("backspace")
	s.HandleKey("backspace") // empty no-op
	if s.Value != "" {
		t.Fatalf("after extra backspace: want '', got %q", s.Value)
	}
}

func TestTextInputSpaceAppendsSpace(t *testing.T) {
	var s TextInputState
	s.Open("title", "", "ab", nil)
	s.HandleKey("space")
	if s.Value != "ab " {
		t.Fatalf("after space: want 'ab ', got %q", s.Value)
	}
}

func TestTextInputEnterSubmitsValue(t *testing.T) {
	var s TextInputState
	s.Open("title", "", "hello", nil)
	res := s.HandleKey("enter")
	if res.Action != TextInputActionSubmit {
		t.Fatalf("enter: want Submit, got %v", res.Action)
	}
	if res.Value != "hello" {
		t.Fatalf("enter value: want 'hello', got %q", res.Value)
	}
	if s.Active {
		t.Fatalf("submit should close the overlay")
	}
}

func TestTextInputEscCancels(t *testing.T) {
	var s TextInputState
	s.Open("title", "", "hello", nil)
	res := s.HandleKey("esc")
	if res.Action != TextInputActionCancel {
		t.Fatalf("esc: want Cancel, got %v", res.Action)
	}
	if s.Active {
		t.Fatalf("esc should close the overlay")
	}
}

func TestTextInputValidatorBlocksSubmit(t *testing.T) {
	var s TextInputState
	s.Open("title", "", "", func(v string) string {
		if v == "" {
			return "name required"
		}
		return ""
	})
	res := s.HandleKey("enter")
	if res.Action != TextInputActionNone {
		t.Fatalf("validator failure: want None action, got %v", res.Action)
	}
	if !s.Active {
		t.Fatalf("validator failure should keep overlay open")
	}
	if s.Error != "name required" {
		t.Fatalf("error not stored: got %q", s.Error)
	}
}

func TestTextInputValidatorAllowsSubmit(t *testing.T) {
	var s TextInputState
	s.Open("title", "", "ok", func(v string) string {
		if v == "" {
			return "required"
		}
		return ""
	})
	res := s.HandleKey("enter")
	if res.Action != TextInputActionSubmit {
		t.Fatalf("validator pass: want Submit, got %v", res.Action)
	}
}

func TestTextInputTypingClearsError(t *testing.T) {
	var s TextInputState
	s.Open("title", "", "", func(v string) string {
		if v == "" {
			return "required"
		}
		return ""
	})
	s.HandleKey("enter") // produces error
	if s.Error == "" {
		t.Fatalf("expected error to be set")
	}
	s.HandleKey("a")
	if s.Error != "" {
		t.Fatalf("typing should clear error, got %q", s.Error)
	}
}

func TestTextInputCloseIsIdempotent(t *testing.T) {
	var s TextInputState
	s.Open("title", "", "hello", nil)
	s.Close()
	s.Close()
	if s.Active {
		t.Fatalf("close should leave Active false")
	}
}
