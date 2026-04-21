package ui

import "testing"

func TestConfirmModalClosedByDefault(t *testing.T) {
	var s ConfirmModalState
	if s.Active {
		t.Fatalf("zero value must be inactive")
	}
	if got := s.HandleKey("y"); got != ConfirmActionNone {
		t.Fatalf("inactive HandleKey: want None, got %v", got)
	}
}

func TestConfirmModalOpenSetsFields(t *testing.T) {
	var s ConfirmModalState
	s.Open("Delete blob?", "report.csv will be permanently removed.", "delete", "cancel", true)
	if !s.Active {
		t.Fatalf("Open should set Active=true")
	}
	if s.Title != "Delete blob?" || s.Message == "" || !s.Destructive {
		t.Fatalf("Open did not populate fields: %+v", s)
	}
}

func TestConfirmModalKeysConfirmAndClose(t *testing.T) {
	for _, key := range []string{"y", "Y", "enter"} {
		var s ConfirmModalState
		s.Open("title", "msg", "ok", "cancel", false)
		if got := s.HandleKey(key); got != ConfirmActionConfirm {
			t.Fatalf("key %q: want Confirm, got %v", key, got)
		}
		if s.Active {
			t.Fatalf("key %q: should close on confirm", key)
		}
	}
}

func TestConfirmModalKeysCancelAndClose(t *testing.T) {
	for _, key := range []string{"n", "N", "esc"} {
		var s ConfirmModalState
		s.Open("title", "msg", "ok", "cancel", false)
		if got := s.HandleKey(key); got != ConfirmActionCancel {
			t.Fatalf("key %q: want Cancel, got %v", key, got)
		}
		if s.Active {
			t.Fatalf("key %q: should close on cancel", key)
		}
	}
}

func TestConfirmModalUnrecognizedKeyIsNoOp(t *testing.T) {
	var s ConfirmModalState
	s.Open("title", "msg", "ok", "cancel", false)
	if got := s.HandleKey("j"); got != ConfirmActionNone {
		t.Fatalf("unrecognized key: want None, got %v", got)
	}
	if !s.Active {
		t.Fatalf("unrecognized key should not close")
	}
}

func TestConfirmModalCloseIsIdempotent(t *testing.T) {
	var s ConfirmModalState
	s.Open("title", "msg", "ok", "cancel", false)
	s.Close()
	s.Close()
	if s.Active {
		t.Fatalf("close should leave Active false")
	}
}
