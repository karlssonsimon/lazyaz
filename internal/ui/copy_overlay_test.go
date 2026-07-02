package ui

import (
	"testing"

	"github.com/karlssonsimon/lazyaz/internal/keymap"
)

func TestCopyOverlayFilterAndApply(t *testing.T) {
	km := keymap.Default()
	var s CopyOverlay
	s.Open([]CopyTarget{
		{Label: "Blob name", Value: "data/logs/2026/app.json"},
		{Label: "Container", Value: "prod-logs"},
		{Label: "Account", Value: "acct"},
	})
	if !s.Active {
		t.Fatal("overlay should be active after Open")
	}

	// Filtering matches on value text too, not just the label.
	for _, r := range "prod" {
		s.HandleKey(string(r), km)
	}
	if got := len(s.Visible()); got != 1 {
		t.Fatalf("filter %q left %d entries, want 1", "prod", got)
	}

	target, picked := s.HandleKey("enter", km)
	if !picked {
		t.Fatal("enter should pick the filtered entry")
	}
	if target.Value != "prod-logs" {
		t.Fatalf("picked value = %q, want prod-logs", target.Value)
	}
	if s.Active {
		t.Fatal("overlay should close after apply")
	}
}

func TestCopyOverlayCancel(t *testing.T) {
	km := keymap.Default()
	var s CopyOverlay
	s.Open([]CopyTarget{{Label: "Account", Value: "acct"}})

	if _, picked := s.HandleKey("esc", km); picked {
		t.Fatal("esc must not pick a target")
	}
	if s.Active {
		t.Fatal("overlay should close on esc with empty query")
	}
}
