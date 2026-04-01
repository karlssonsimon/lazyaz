package ui

import (
	"strings"
	"testing"
)

func TestHelpOverlayToggle(t *testing.T) {
	state := HelpOverlayState{}
	state.Toggle()
	if !state.Active {
		t.Fatal("expected help overlay to open")
	}
	state.Toggle()
	if state.Active {
		t.Fatal("expected help overlay to close")
	}
}

func TestRenderHelpOverlayIncludesContent(t *testing.T) {
	styles := NewStyles(FallbackScheme())
	view := RenderHelpOverlay(
		"Help",
		[]HelpSection{{Title: "General", Items: []string{"tab  next focus", "?    toggle help"}}},
		styles,
		100,
		40,
		"base",
	)

	for _, want := range []string{"Help", "General", "toggle help", "?: close | esc close"} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected rendered overlay to contain %q", want)
		}
	}
}
