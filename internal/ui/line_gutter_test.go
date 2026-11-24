package ui

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/viewport"
	"github.com/charmbracelet/x/ansi"
)

func gutterViewport(lines, height, yoffset int) viewport.Model {
	vp := viewport.New()
	vp.SetWidth(40)
	vp.SetHeight(height)
	vp.SetContent(strings.TrimSuffix(strings.Repeat("text\n", lines), "\n"))
	vp.SetYOffset(yoffset)
	return vp
}

func gutterRows(gutter string) []string {
	rows := strings.Split(ansi.Strip(gutter), "\n")
	for i, row := range rows {
		rows[i] = strings.TrimRight(row, " ")
	}
	return rows
}

func TestRenderLineGutterNumbersFromOne(t *testing.T) {
	styles := NewStyles(FallbackScheme())
	got := gutterRows(RenderLineGutter(gutterViewport(10, 3, 4), styles, 3))
	want := []string{"  5 │", "  6 │", "  7 │"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("gutter rows = %q, want %q", got, want)
	}
}

// A viewport holding a slice of a longer document numbers its lines by
// where the slice starts, and is as wide as the largest number needs.
func TestRenderLineGutterFromDocumentLine(t *testing.T) {
	styles := NewStyles(FallbackScheme())
	got := gutterRows(RenderLineGutterFrom(gutterViewport(10, 3, 7), styles, 3, 99_991))
	want := []string{" 99998 │", " 99999 │", "100000 │"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("gutter rows = %q, want %q", got, want)
	}
	if got, want := len(want[0])-len(" │")+3, LineGutterWidth(99_990+10, 3); got != want {
		t.Errorf("gutter is %d cells wide, LineGutterWidth says %d", got, want)
	}
}

// Without a known first line the gutter keeps its shape but shows no
// numbers, and rows past the content stay blank either way.
func TestRenderLineGutterUnnumbered(t *testing.T) {
	styles := NewStyles(FallbackScheme())
	got := gutterRows(RenderLineGutterFrom(gutterViewport(2, 3, 0), styles, 3, 0))
	want := []string{"    │", "    │", "    │"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("gutter rows = %q, want %q", got, want)
	}

	got = gutterRows(RenderLineGutterFrom(gutterViewport(2, 3, 0), styles, 3, 1))
	want = []string{"  1 │", "  2 │", "    │"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("gutter rows = %q, want %q", got, want)
	}
}
