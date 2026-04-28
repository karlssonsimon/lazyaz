package ui

import (
	"strings"
	"testing"
)

func countItems(visible []OverlayItem) int {
	n := 0
	for _, v := range visible {
		if !v.IsHeader {
			n++
		}
	}
	return n
}

func firstItemLabel(visible []OverlayItem) string {
	for _, v := range visible {
		if !v.IsHeader {
			return v.Label
		}
	}
	return ""
}

func TestHelpOverlayOpenClose(t *testing.T) {
	state := HelpOverlayState{}
	sections := []HelpSection{{Title: "Test", Items: []string{"key  desc"}}}

	state.Open("Help", sections)
	if !state.Active {
		t.Fatal("expected help overlay to open")
	}
	state.Close()
	if state.Active {
		t.Fatal("expected help overlay to close")
	}
}

func TestRenderHelpOverlayIncludesContent(t *testing.T) {
	styles := NewStyles(FallbackScheme())
	state := HelpOverlayState{}
	state.Open("Help", []HelpSection{{Title: "General", Items: []string{"tab  next focus", "?  toggle help"}}})

	view := RenderHelpOverlay(state, "esc", "█", styles, nil, 100, 40, "base")

	for _, want := range []string{"HELP", "GENERAL", "toggle help", "esc"} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected rendered overlay to contain %q", want)
		}
	}
}

func TestHelpOverlaySearch(t *testing.T) {
	state := HelpOverlayState{}
	state.Open("Help", []HelpSection{
		{Title: "Navigation", Items: []string{"tab  next focus", "/  filter pane"}},
		{Title: "Actions", Items: []string{"d  download", "r  refresh"}},
	})

	bindings := HelpKeyBindings{
		Up:    noopMatcher{},
		Down:  noopMatcher{},
		Close: noopMatcher{},
		Erase: keyMatcher{"backspace"},
	}
	// Type "down" to filter.
	state.HandleKey("d", bindings)
	state.HandleKey("o", bindings)
	state.HandleKey("w", bindings)
	state.HandleKey("n", bindings)

	if got := countItems(state.visible); got != 1 {
		t.Fatalf("expected 1 filtered item, got %d", got)
	}
	if !strings.Contains(firstItemLabel(state.visible), "download") {
		t.Fatalf("expected filtered item to be download, got %q", firstItemLabel(state.visible))
	}

	// Backspace to clear filter; back to grouped view with all items.
	state.HandleKey("backspace", bindings)
	state.HandleKey("backspace", bindings)
	state.HandleKey("backspace", bindings)
	state.HandleKey("backspace", bindings)
	if got := countItems(state.visible); got != 4 {
		t.Fatalf("expected 4 items after clearing filter, got %d", got)
	}
}

func TestHelpOverlaySearchBySection(t *testing.T) {
	state := HelpOverlayState{}
	state.Open("Help", []HelpSection{
		{Title: "Navigation", Items: []string{"tab  next focus", "/  filter pane"}},
		{Title: "Actions", Items: []string{"d  download", "r  refresh"}},
	})

	bindings := HelpKeyBindings{Up: noopMatcher{}, Down: noopMatcher{}, Close: noopMatcher{}}
	// Type "actions" to filter by section name.
	for _, ch := range "actions" {
		state.HandleKey(string(ch), bindings)
	}

	if got := countItems(state.visible); got != 2 {
		t.Fatalf("expected 2 filtered items for section 'Actions', got %d", got)
	}
}

// noopMatcher is a KeyMatcher that never matches (for tests).
type noopMatcher struct{}

func (noopMatcher) Matches(string) bool { return false }

// keyMatcher matches a specific literal key (for tests).
type keyMatcher struct{ key string }

func (k keyMatcher) Matches(s string) bool { return s == k.key }
