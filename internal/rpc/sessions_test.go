package rpc

import "testing"

func TestSessionsCreateGetDelete(t *testing.T) {
	sessions := NewSessions(func() int { return 42 })

	id, value, err := sessions.Create()
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if id == "" {
		t.Fatal("expected session id")
	}
	if value != 42 {
		t.Fatalf("got %d, want 42", value)
	}

	got, ok := sessions.Get(id)
	if !ok {
		t.Fatal("expected session to exist")
	}
	if got != 42 {
		t.Fatalf("got %d, want 42", got)
	}
	if sessions.Len() != 1 {
		t.Fatalf("got len %d, want 1", sessions.Len())
	}

	if !sessions.Delete(id) {
		t.Fatal("expected delete to succeed")
	}
	if sessions.Delete(id) {
		t.Fatal("expected second delete to fail")
	}
	if sessions.Len() != 0 {
		t.Fatalf("got len %d, want 0", sessions.Len())
	}
}
