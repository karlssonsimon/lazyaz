package ui

import (
	"sort"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"charm.land/lipgloss/v2"
)

// ColumnRange is a half-open span of display columns on one rendered
// line. Columns are cell positions, not byte offsets, because the lines
// being restyled already carry syntax-highlighting escape sequences and
// may hold multibyte runes.
type ColumnRange struct {
	Start int
	End   int
	Style lipgloss.Style
}

// HighlightLine restyles the given column ranges within one rendered
// line, preserving the escape sequences already in it.
//
// A line arrives from the syntax highlighter as text interleaved with
// SGR sequences, so it cannot be sliced by byte offset. ansi.Truncate
// and ansi.Cut slice by display column instead and carry the enclosing
// styles across the cut, which is what keeps the surrounding colors
// intact around a highlighted span.
//
// Overlapping ranges are resolved by taking the earlier one; ranges are
// applied right to left so that earlier column positions stay valid as
// the line is rebuilt.
func HighlightLine(line string, width int, ranges []ColumnRange) string {
	if len(ranges) == 0 {
		return line
	}

	lineWidth := ansi.StringWidth(line)
	if lineWidth == 0 {
		return line
	}

	clean := normalizeRanges(ranges, lineWidth)
	if len(clean) == 0 {
		return line
	}

	// Right to left: rebuilding the tail first leaves the columns of
	// everything to its left unchanged.
	for i := len(clean) - 1; i >= 0; i-- {
		r := clean[i]
		before := ""
		if r.Start > 0 {
			before = ansi.Truncate(line, r.Start, "")
		}
		span := ansi.Strip(ansi.Cut(line, r.Start, r.End))
		after := ansi.Cut(line, r.End, ansi.StringWidth(line))
		line = before + r.Style.Render(span) + after
	}

	if width > 0 && ansi.StringWidth(line) > width {
		line = ansi.Truncate(line, width, "")
	}
	return line
}

// normalizeRanges clamps ranges to the line, drops empty ones, sorts
// them and discards any that overlap a previous range.
func normalizeRanges(ranges []ColumnRange, lineWidth int) []ColumnRange {
	clean := make([]ColumnRange, 0, len(ranges))
	for _, r := range ranges {
		if r.Start < 0 {
			r.Start = 0
		}
		if r.End > lineWidth {
			r.End = lineWidth
		}
		if r.Start >= r.End {
			continue
		}
		clean = append(clean, r)
	}
	if len(clean) < 2 {
		return clean
	}

	sort.Slice(clean, func(i, j int) bool { return clean[i].Start < clean[j].Start })

	out := clean[:1]
	for _, r := range clean[1:] {
		if r.Start < out[len(out)-1].End {
			continue // overlaps the previous range; first one wins
		}
		out = append(out, r)
	}
	return out
}

// HighlightLines applies per-row column ranges to a block of rendered
// lines, keyed by row index. Rows without ranges are left untouched.
func HighlightLines(content string, width int, byRow map[int][]ColumnRange) string {
	if len(byRow) == 0 {
		return content
	}
	lines := strings.Split(content, "\n")
	for row, ranges := range byRow {
		if row < 0 || row >= len(lines) {
			continue
		}
		lines[row] = HighlightLine(lines[row], width, ranges)
	}
	return strings.Join(lines, "\n")
}

// PlainWidth is the display width of raw (unstyled) bytes. Used to turn
// a byte offset inside a buffer into the column where it will appear
// once rendered, which is not the same number as soon as the text holds
// multibyte runes.
func PlainWidth(b []byte) int {
	return ansi.StringWidth(string(b))
}
