package ui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

func TestRenderAppHeaderIsOneLineAndFullWidth(t *testing.T) {
	styles := NewStyles(FallbackScheme())
	out := RenderAppHeader(HeaderConfig{
		Brand: "lazyaz",
		Path:  []string{"Blob", "acct", "container"},
		Meta:  "connected",
	}, styles, 72)

	if got := strings.Count(out, "\n") + 1; got != AppHeaderHeight {
		t.Fatalf("header height = %d, want %d", got, AppHeaderHeight)
	}
	if got := lipgloss.Width(out); got != 72 {
		t.Fatalf("header width = %d, want 72", got)
	}
	if !strings.Contains(out, "lazyaz") || !strings.Contains(out, "Blob") || !strings.Contains(out, "connected") {
		t.Fatalf("header missing brand/path/meta: %q", out)
	}
}

func TestRenderStatusLineIsOneLineAndOmitsEmptyActions(t *testing.T) {
	styles := NewStyles(FallbackScheme())
	out := RenderStatusLine(StatusLineConfig{
		Mode: "NORMAL",
		Actions: []StatusAction{
			{Key: "j/k", Label: "move"},
			{Key: "", Label: "ignored"},
			{Key: "/", Label: "filter"},
		},
		Message: "ready",
	}, styles, 64)

	if strings.Contains(out, "ignored") {
		t.Fatalf("empty-key action rendered: %q", out)
	}
	if strings.Count(out, "\n") != 0 {
		t.Fatalf("status rendered multiple lines: %q", out)
	}
	if got := lipgloss.Width(out); got != 64 {
		t.Fatalf("status width = %d, want 64", got)
	}
}
