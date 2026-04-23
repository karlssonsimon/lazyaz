package ui

import "testing"

func TestFormClosedByDefault(t *testing.T) {
	var s FormOverlayState
	if s.Active {
		t.Fatalf("zero value must be inactive")
	}
	if got := s.HandleKey("a"); got.Action != FormActionNone {
		t.Fatalf("inactive HandleKey: want None, got %v", got.Action)
	}
}

func TestFormOpenIgnoresEmptyFields(t *testing.T) {
	var s FormOverlayState
	s.Open("Create secret", nil)
	if s.Active {
		t.Fatalf("Open with no fields must leave overlay inactive")
	}
}

func TestFormOpenSetsFields(t *testing.T) {
	var s FormOverlayState
	s.Open("Create secret", []FormField{
		{Label: "Name", Placeholder: "db-password"},
		{Label: "Value", Placeholder: "hunter2"},
	})
	if !s.Active {
		t.Fatalf("Open should set Active=true")
	}
	if s.Title != "Create secret" || len(s.Fields) != 2 || s.Focus != 0 {
		t.Fatalf("Open did not populate state: %+v", s)
	}
}

func TestFormTypingScopedToFocusedField(t *testing.T) {
	var s FormOverlayState
	s.Open("title", []FormField{{Label: "a"}, {Label: "b"}})
	s.HandleKey("x")
	s.HandleKey("y")
	if s.Fields[0].Value != "xy" {
		t.Fatalf("field 0: want 'xy', got %q", s.Fields[0].Value)
	}
	if s.Fields[1].Value != "" {
		t.Fatalf("field 1 should be untouched, got %q", s.Fields[1].Value)
	}
}

func TestFormTabAdvancesFocusWithWrap(t *testing.T) {
	var s FormOverlayState
	s.Open("title", []FormField{{Label: "a"}, {Label: "b"}, {Label: "c"}})
	s.HandleKey("tab")
	if s.Focus != 1 {
		t.Fatalf("after tab: want focus 1, got %d", s.Focus)
	}
	s.HandleKey("tab")
	s.HandleKey("tab") // wraps
	if s.Focus != 0 {
		t.Fatalf("after 3 tabs: want focus 0 (wrapped), got %d", s.Focus)
	}
}

func TestFormShiftTabRewindsFocusWithWrap(t *testing.T) {
	var s FormOverlayState
	s.Open("title", []FormField{{Label: "a"}, {Label: "b"}})
	s.HandleKey("shift+tab")
	if s.Focus != 1 {
		t.Fatalf("shift+tab from first: want wrap to last (1), got %d", s.Focus)
	}
}

func TestFormBackspaceRemovesFromFocusedField(t *testing.T) {
	var s FormOverlayState
	s.Open("title", []FormField{{Label: "a", Value: "abc"}, {Label: "b", Value: "zzz"}})
	s.HandleKey("backspace")
	if s.Fields[0].Value != "ab" {
		t.Fatalf("field 0: want 'ab', got %q", s.Fields[0].Value)
	}
	if s.Fields[1].Value != "zzz" {
		t.Fatalf("field 1 should be untouched, got %q", s.Fields[1].Value)
	}
}

func TestFormEscCancels(t *testing.T) {
	var s FormOverlayState
	s.Open("title", []FormField{{Label: "a", Value: "ok"}})
	res := s.HandleKey("esc")
	if res.Action != FormActionCancel {
		t.Fatalf("esc: want Cancel, got %v", res.Action)
	}
	if s.Active {
		t.Fatalf("esc should close the overlay")
	}
}

func TestFormEnterSubmitsAllValues(t *testing.T) {
	var s FormOverlayState
	s.Open("title", []FormField{
		{Label: "Name", Value: "db-password"},
		{Label: "Value", Value: "hunter2"},
	})
	res := s.HandleKey("enter")
	if res.Action != FormActionSubmit {
		t.Fatalf("enter: want Submit, got %v", res.Action)
	}
	if len(res.Values) != 2 || res.Values[0] != "db-password" || res.Values[1] != "hunter2" {
		t.Fatalf("enter values: got %+v", res.Values)
	}
	if s.Active {
		t.Fatalf("submit should close the overlay")
	}
}

func TestFormValidatorBlocksSubmitAndFocusesFirstFailure(t *testing.T) {
	required := func(v string) string {
		if v == "" {
			return "required"
		}
		return ""
	}
	var s FormOverlayState
	s.Open("title", []FormField{
		{Label: "Name", Value: "ok", Validate: required},
		{Label: "Value", Value: "", Validate: required},
	})
	s.Focus = 0 // simulate user on field 0 when hitting enter
	res := s.HandleKey("enter")
	if res.Action != FormActionNone {
		t.Fatalf("validator failure: want None, got %v", res.Action)
	}
	if !s.Active {
		t.Fatalf("validator failure should keep overlay open")
	}
	if s.Fields[0].Error != "" {
		t.Fatalf("field 0 should have no error, got %q", s.Fields[0].Error)
	}
	if s.Fields[1].Error != "required" {
		t.Fatalf("field 1 error: want 'required', got %q", s.Fields[1].Error)
	}
	if s.Focus != 1 {
		t.Fatalf("focus should move to first failing field (1), got %d", s.Focus)
	}
}

func TestFormTypingClearsOnlyFocusedError(t *testing.T) {
	required := func(v string) string {
		if v == "" {
			return "required"
		}
		return ""
	}
	var s FormOverlayState
	s.Open("title", []FormField{
		{Label: "Name", Validate: required},
		{Label: "Value", Validate: required},
	})
	s.HandleKey("enter") // both fields error; focus lands on 0
	if s.Fields[0].Error == "" || s.Fields[1].Error == "" {
		t.Fatalf("expected both fields to have errors")
	}
	s.HandleKey("a")
	if s.Fields[0].Error != "" {
		t.Fatalf("typing should clear focused field error, got %q", s.Fields[0].Error)
	}
	if s.Fields[1].Error == "" {
		t.Fatalf("typing should NOT clear other field's error")
	}
}

func TestFormValidatorAllowsSubmit(t *testing.T) {
	required := func(v string) string {
		if v == "" {
			return "required"
		}
		return ""
	}
	var s FormOverlayState
	s.Open("title", []FormField{{Label: "a", Value: "ok", Validate: required}})
	res := s.HandleKey("enter")
	if res.Action != FormActionSubmit {
		t.Fatalf("validator pass: want Submit, got %v", res.Action)
	}
}

func TestFormSpaceAppendsToFocused(t *testing.T) {
	var s FormOverlayState
	s.Open("title", []FormField{{Label: "a", Value: "x"}})
	s.HandleKey("space")
	if s.Fields[0].Value != "x " {
		t.Fatalf("after space: want 'x ', got %q", s.Fields[0].Value)
	}
}

func TestFormCloseIsIdempotent(t *testing.T) {
	var s FormOverlayState
	s.Open("title", []FormField{{Label: "a", Value: "x"}})
	s.Close()
	s.Close()
	if s.Active {
		t.Fatalf("close should leave Active false")
	}
}
