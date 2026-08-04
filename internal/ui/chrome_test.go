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

// The pending count trails the hints. Appended last it cannot shift
// them, and it sits beside the content instead of floating at the far
// edge of a wide terminal.
func TestStatusLineCountTrailsHints(t *testing.T) {
	styles := NewStyles(FallbackScheme())
	cfg := StatusLineConfig{
		Mode:    "NORMAL",
		Actions: []StatusAction{{Key: "j/k", Label: "move"}, {Key: "?", Label: "help"}},
	}

	without := ansi.Strip(RenderStatusLine(cfg, styles, 200))
	cfg.Count = 12
	with := ansi.Strip(RenderStatusLine(cfg, styles, 200))

	countAt := strings.Index(with, "12")
	if countAt < 0 {
		t.Fatalf("count missing: %q", with)
	}
	if lastHint := strings.Index(with, "help"); countAt < lastHint {
		t.Errorf("count at %d sits before the last hint at %d", countAt, lastHint)
	}
	// Right after the content, not at the terminal edge.
	if countAt > len(with)-1-40 {
		t.Errorf("count at column %d of %d — floating at the far edge again", countAt, len(with))
	}
	if strings.Contains(without, "12") {
		t.Fatal("count rendered with none pending")
	}

	// The content before it must sit at identical positions.
	for _, seg := range []string{"NORMAL", "j/k", "move", "help"} {
		if strings.Index(with, seg) != strings.Index(without, seg) {
			t.Errorf("%q shifted from %d to %d when the count appeared",
				seg, strings.Index(without, seg), strings.Index(with, seg))
		}
	}
}

// The gap filler's own padding must not push the right segment past
// width — messages were silently losing their last two characters.
func TestStatusLineRightMessageNotTruncated(t *testing.T) {
	styles := NewStyles(FallbackScheme())
	line := RenderStatusLine(StatusLineConfig{
		Mode:    "NORMAL",
		Message: "Copied to clipboard",
	}, styles, 80)

	stripped := strings.TrimRight(ansi.Strip(line), " ")
	if !strings.HasSuffix(stripped, "Copied to clipboard") {
		t.Errorf("right message truncated: %q", stripped)
	}
	if w := ansi.StringWidth(line); w != 80 {
		t.Errorf("line width = %d, want 80", w)
	}
}
