package ui

import (
	"github.com/charmbracelet/x/ansi"

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

// The pending count renders as its own segment beside the mode chip,
// not inside it: the chip's styling must close before the digits start.
func TestStatusLineCountOutsideModeChip(t *testing.T) {
	styles := NewStyles(FallbackScheme())

	with := RenderStatusLine(StatusLineConfig{Mode: "NORMAL", Count: 12}, styles, 80)
	stripped := ansi.Strip(with)
	if !strings.Contains(stripped, "NORMAL") || !strings.Contains(stripped, "12") {
		t.Fatalf("mode or count missing: %q", stripped)
	}

	chip := styles.Chrome.StatusMode.Render("NORMAL")
	if !strings.Contains(with, chip) {
		t.Errorf("mode chip no longer renders standalone — the count leaked inside it")
	}

	without := RenderStatusLine(StatusLineConfig{Mode: "NORMAL"}, styles, 80)
	if strings.Contains(ansi.Strip(without), "12") {
		t.Errorf("count rendered with none pending")
	}
}
