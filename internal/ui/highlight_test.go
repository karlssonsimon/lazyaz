package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"charm.land/lipgloss/v2"
)

func hlStyle() lipgloss.Style {
	return lipgloss.NewStyle().Reverse(true)
}

// Highlighting must never change what the line says, only how it looks.
func TestHighlightLinePreservesVisibleText(t *testing.T) {
	tests := []struct {
		name string
		line string
		rng  ColumnRange
	}{
		{"plain text", "hello world", ColumnRange{Start: 6, End: 11}},
		{"already styled", lipgloss.NewStyle().Foreground(lipgloss.Color("#ff0000")).Render("hello world"), ColumnRange{Start: 0, End: 5}},
		{"multibyte", "det är vackert", ColumnRange{Start: 7, End: 14}},
		{"range at line start", "hello world", ColumnRange{Start: 0, End: 5}},
		{"range to line end", "hello world", ColumnRange{Start: 6, End: 11}},
		{"whole line", "hello world", ColumnRange{Start: 0, End: 11}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.rng.Style = hlStyle()
			got := HighlightLine(tt.line, 0, []ColumnRange{tt.rng})

			if want := ansi.Strip(tt.line); ansi.Strip(got) != want {
				t.Errorf("visible text changed:\n got %q\nwant %q", ansi.Strip(got), want)
			}
			if got == tt.line {
				t.Error("line was returned unchanged; expected styling to be applied")
			}
		})
	}
}

func TestHighlightLineHighlightsTheRightColumns(t *testing.T) {
	// Reverse video wraps the span in an SGR 7 sequence; the span
	// between the markers should be exactly the requested columns.
	line := "abcdefghij"
	got := HighlightLine(line, 0, []ColumnRange{{Start: 3, End: 6, Style: hlStyle()}})

	if !strings.Contains(got, "def") {
		t.Errorf("highlighted span missing from %q", got)
	}
	if ansi.Strip(got) != line {
		t.Errorf("visible text = %q, want %q", ansi.Strip(got), line)
	}
	// The highlight must not begin at column 0.
	if strings.HasPrefix(got, "\x1b[7m") {
		t.Errorf("highlight starts at column 0, want column 3: %q", got)
	}
}

func TestHighlightLineMultipleRanges(t *testing.T) {
	line := "alpha beta alpha"
	got := HighlightLine(line, 0, []ColumnRange{
		{Start: 0, End: 5, Style: hlStyle()},
		{Start: 11, End: 16, Style: hlStyle()},
	})

	if ansi.Strip(got) != line {
		t.Errorf("visible text = %q, want %q", ansi.Strip(got), line)
	}
	if n := strings.Count(got, "\x1b[7m"); n != 2 {
		t.Errorf("found %d highlight starts, want 2: %q", n, got)
	}
}

func TestHighlightLineEdgeCases(t *testing.T) {
	line := "hello"

	tests := []struct {
		name   string
		ranges []ColumnRange
	}{
		{"no ranges", nil},
		{"empty range", []ColumnRange{{Start: 2, End: 2, Style: hlStyle()}}},
		{"inverted range", []ColumnRange{{Start: 4, End: 1, Style: hlStyle()}}},
		{"range entirely past the line", []ColumnRange{{Start: 50, End: 60, Style: hlStyle()}}},
		{"negative start clamps", []ColumnRange{{Start: -3, End: 0, Style: hlStyle()}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := HighlightLine(line, 0, tt.ranges)
			if ansi.Strip(got) != line {
				t.Errorf("visible text = %q, want %q", ansi.Strip(got), line)
			}
		})
	}
}

// A range running past the end of the line is clamped rather than
// panicking on a slice bound.
func TestHighlightLineClampsOverlongRange(t *testing.T) {
	line := "hello"
	got := HighlightLine(line, 0, []ColumnRange{{Start: 3, End: 99, Style: hlStyle()}})
	if ansi.Strip(got) != line {
		t.Errorf("visible text = %q, want %q", ansi.Strip(got), line)
	}
}

// Overlapping ranges would corrupt the rebuild, so the earlier one wins.
func TestHighlightLineDropsOverlappingRanges(t *testing.T) {
	line := "abcdefghij"
	got := HighlightLine(line, 0, []ColumnRange{
		{Start: 2, End: 6, Style: hlStyle()},
		{Start: 4, End: 8, Style: hlStyle()},
	})

	if ansi.Strip(got) != line {
		t.Errorf("visible text = %q, want %q", ansi.Strip(got), line)
	}
	if n := strings.Count(got, "\x1b[7m"); n != 1 {
		t.Errorf("found %d highlight starts, want 1 (overlap dropped): %q", n, got)
	}
}

// The rendered result must never exceed the pane width, or it breaks the
// column border.
func TestHighlightLineRespectsWidth(t *testing.T) {
	line := strings.Repeat("x", 40)
	got := HighlightLine(line, 20, []ColumnRange{{Start: 0, End: 40, Style: hlStyle()}})
	if w := ansi.StringWidth(got); w > 20 {
		t.Errorf("rendered width %d, want at most 20", w)
	}
}

func TestHighlightLines(t *testing.T) {
	content := "first line\nsecond line\nthird line"
	got := HighlightLines(content, 0, map[int][]ColumnRange{
		1: {{Start: 0, End: 6, Style: hlStyle()}},
	})

	lines := strings.Split(got, "\n")
	if len(lines) != 3 {
		t.Fatalf("got %d lines, want 3", len(lines))
	}
	if lines[0] != "first line" || lines[2] != "third line" {
		t.Error("untouched rows were modified")
	}
	if !strings.Contains(lines[1], "\x1b[7m") {
		t.Errorf("target row was not highlighted: %q", lines[1])
	}
	if ansi.Strip(got) != content {
		t.Errorf("visible text changed:\n got %q\nwant %q", ansi.Strip(got), content)
	}
}

// Row indexes outside the content are ignored rather than panicking.
func TestHighlightLinesIgnoresOutOfRangeRows(t *testing.T) {
	content := "only line"
	got := HighlightLines(content, 0, map[int][]ColumnRange{
		-1: {{Start: 0, End: 4, Style: hlStyle()}},
		99: {{Start: 0, End: 4, Style: hlStyle()}},
	})
	if got != content {
		t.Errorf("content changed: %q", got)
	}
}
