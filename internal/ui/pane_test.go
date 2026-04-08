package ui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

// pane style equivalent to Chrome.Pane: rounded border + Padding(0,1).
// Vertical frame size = 2 (top + bottom border, 0 vertical padding).
func testPaneStyle() lipgloss.Style {
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1)
}

func countLines(s string) int {
	if s == "" {
		return 0
	}
	return strings.Count(s, "\n") + 1
}

func TestRenderPaneExactHeight(t *testing.T) {
	styles := Styles{}
	styles.Chrome.Pane = testPaneStyle()
	styles.Chrome.FocusedPane = testPaneStyle()

	cases := []struct {
		name        string
		contentRows int
		frameH      int
	}{
		{"empty content", 0, 20},
		{"short content", 3, 20},
		{"exactly inner", 18, 20}, // 20 - vFrame(2)
		{"one over inner", 19, 20},
		{"way over inner", 200, 20},
		{"many rows over inner", 1000, 30},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lines := make([]string, tc.contentRows)
			for i := range lines {
				lines[i] = "x"
			}
			content := strings.Join(lines, "\n")
			out := RenderPane(content, PaneFrame{Width: 40, Height: tc.frameH}, styles)
			got := countLines(out)
			if got != tc.frameH {
				t.Errorf("rendered %d lines, want exactly %d (frame.Height)", got, tc.frameH)
			}
		})
	}
}

func TestRenderPaneWideContentDoesNotExpandHeight(t *testing.T) {
	// Wide lines that lipgloss might wrap if they overflow Width must
	// not blow up the total rendered height. The pane block should still
	// be exactly Frame.Height tall.
	styles := Styles{}
	styles.Chrome.Pane = testPaneStyle()
	styles.Chrome.FocusedPane = testPaneStyle()

	wide := strings.Repeat("x", 200) // way wider than 40
	out := RenderPane(wide, PaneFrame{Width: 40, Height: 12}, styles)
	if got := countLines(out); got != 12 {
		t.Errorf("wide content blew up height: got %d lines, want 12", got)
	}
}

func TestPaneLayoutWidthsSumToTotal(t *testing.T) {
	style := testPaneStyle()
	cases := []struct {
		total int
		n     int
	}{
		{200, 4},
		{200, 3},
		{201, 4},
		{203, 4},
		{80, 3},
		{80, 4},
	}
	for _, tc := range cases {
		widths := PaneLayout(style, tc.total, tc.n)
		sum := 0
		for _, w := range widths {
			sum += w
		}
		if sum != tc.total {
			t.Errorf("PaneLayout(%d, %d): widths sum to %d, want %d (widths=%v)",
				tc.total, tc.n, sum, tc.total, widths)
		}
	}
}

func TestPaneLayoutRenderedRowFillsTotal(t *testing.T) {
	// Verify that actually rendering panes with PaneLayout's widths produces
	// a horizontally-joined row whose visible width equals the requested total.
	// This is the round-trip check that catches v1↔v2 width-semantic mismatches.
	styles := Styles{}
	styles.Chrome.Pane = testPaneStyle()
	styles.Chrome.FocusedPane = testPaneStyle()

	cases := []struct {
		total int
		n     int
	}{
		{200, 4},
		{120, 3},
		{80, 4},
	}
	for _, tc := range cases {
		widths := PaneLayout(styles.Chrome.Pane, tc.total, tc.n)
		blocks := make([]string, tc.n)
		for i := 0; i < tc.n; i++ {
			blocks[i] = RenderPane("x", PaneFrame{Width: widths[i], Height: 10}, styles)
		}
		row := lipgloss.JoinHorizontal(lipgloss.Top, blocks...)
		// First line of the joined row tells us the rendered width.
		firstLine := strings.Split(row, "\n")[0]
		got := lipgloss.Width(firstLine)
		if got != tc.total {
			t.Errorf("PaneLayout(%d, %d) → rendered row width %d, want %d (widths=%v)",
				tc.total, tc.n, got, tc.total, widths)
		}
	}
}

func TestPaneInnerHeight(t *testing.T) {
	style := testPaneStyle() // vFrame = 2
	cases := []struct {
		total int
		want  int
	}{
		{0, 0},
		{1, 0},
		{2, 0},
		{3, 1},
		{20, 18},
		{50, 48},
	}
	for _, tc := range cases {
		if got := PaneInnerHeight(style, tc.total); got != tc.want {
			t.Errorf("PaneInnerHeight(%d) = %d, want %d", tc.total, got, tc.want)
		}
	}
}
