package ui

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/list"
	"charm.land/lipgloss/v2"
)

type testListItem string

func (i testListItem) Title() string       { return string(i) }
func (i testListItem) Description() string { return "" }
func (i testListItem) FilterValue() string { return string(i) }

func TestRenderMillerColumnExactSizeAndNoBoxBorder(t *testing.T) {
	styles := NewStyles(FallbackScheme())
	out := RenderMillerColumn(MillerColumn{
		Title:  "ACCOUNTS",
		Body:   "one\ntwo",
		Footer: "2 of 2",
		Frame:  MillerColumnFrame{Width: 28, Height: 8, Focused: true, RightRule: true},
	}, styles)

	if got := strings.Count(out, "\n") + 1; got != 8 {
		t.Fatalf("column height = %d, want 8", got)
	}
	for _, line := range strings.Split(out, "\n") {
		if got := lipgloss.Width(line); got != 28 {
			t.Fatalf("line width = %d, want 28: %q", got, line)
		}
	}
	for _, border := range []string{"╭", "╮", "╰", "╯"} {
		if strings.Contains(out, border) {
			t.Fatalf("boxed border %q rendered in flat column: %q", border, out)
		}
	}
	if !strings.Contains(out, "│") {
		t.Fatalf("right rule missing: %q", out)
	}
}

func TestMillerListBodyHeightReservesTitleAndFooter(t *testing.T) {
	if got := MillerListBodyHeight(10, true); got != 8 {
		t.Fatalf("body height = %d, want 8", got)
	}
	if got := MillerListBodyHeight(1, true); got != 1 {
		t.Fatalf("body height minimum = %d, want 1", got)
	}
}

func TestRenderMillerListColumnUsesListView(t *testing.T) {
	styles := NewStyles(FallbackScheme())
	l := list.New([]list.Item{testListItem("alpha")}, styles.Delegate, 20, 3)
	l.SetShowTitle(false)
	l.SetShowHelp(false)
	l.SetShowPagination(false)
	l.SetShowStatusBar(false)
	out := RenderMillerListColumn(MillerListColumn{
		List:   &l,
		Title:  "ITEMS",
		Footer: "1 of 1",
		Frame:  MillerColumnFrame{Width: 24, Height: 6},
	}, styles)
	if !strings.Contains(out, "alpha") {
		t.Fatalf("list item missing from column: %q", out)
	}
}

func TestMillerLayoutNarrowThreeColumnsPreservesTotalWidth(t *testing.T) {
	cols := MillerLayout(lipgloss.NewStyle(), 10, true, true)
	if got := cols.Parent + cols.Focused + cols.Child; got != 10 {
		t.Fatalf("width sum = %d, want 10: %+v", got, cols)
	}
	if cols.Parent < 0 || cols.Focused < 0 || cols.Child < 0 {
		t.Fatalf("widths must be non-negative: %+v", cols)
	}
}

func TestMillerLayoutNarrowTwoColumnsPreservesTotalWidth(t *testing.T) {
	cols := MillerLayout(lipgloss.NewStyle(), 6, true, false)
	if got := cols.Parent + cols.Focused + cols.Child; got != 6 {
		t.Fatalf("width sum = %d, want 6: %+v", got, cols)
	}
	if cols.Parent < 0 || cols.Focused < 0 || cols.Child < 0 {
		t.Fatalf("widths must be non-negative: %+v", cols)
	}
}
