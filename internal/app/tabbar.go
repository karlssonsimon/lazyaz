package app

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/karlssonsimon/lazyaz/internal/ui"
)

func renderTabBar(tabs []Tab, activeIdx int, tabStyles ui.TabBarStyles, width int) string {
	if len(tabs) == 0 {
		return ""
	}

	// Count tabs per kind so we can number duplicates.
	kindCount := map[TabKind]int{}
	for _, t := range tabs {
		kindCount[t.Kind]++
	}

	// Assign per-kind sequence numbers.
	kindSeq := map[TabKind]int{}

	var parts []string
	for i, t := range tabs {
		kindSeq[t.Kind]++
		label := t.Kind.String()
		if kindCount[t.Kind] > 1 {
			label = fmt.Sprintf("%s %d", label, kindSeq[t.Kind])
		}
		label = fmt.Sprintf(" %d:%s ", i+1, label)

		if i == activeIdx {
			parts = append(parts, tabStyles.Active.Render(label))
		} else {
			parts = append(parts, tabStyles.Inactive.Render(label))
		}

		if i < len(tabs)-1 {
			parts = append(parts, tabStyles.Sep.Render("│"))
		}
	}

	bar := strings.Join(parts, "")

	barLine := tabStyles.Bar.
		Width(width).
		Render(bar)

	return barLine
}

// tabIndexAtX returns the tab index for a click at screen column x,
// or -1 if the click doesn't land on any tab label.
func tabIndexAtX(tabs []Tab, activeIdx int, tabStyles ui.TabBarStyles, x int) int {
	if len(tabs) == 0 {
		return -1
	}

	// Count tabs per kind so we can number duplicates (same logic as render).
	kindCount := map[TabKind]int{}
	for _, t := range tabs {
		kindCount[t.Kind]++
	}
	kindSeq := map[TabKind]int{}

	cursor := 0
	for i, t := range tabs {
		kindSeq[t.Kind]++
		label := t.Kind.String()
		if kindCount[t.Kind] > 1 {
			label = fmt.Sprintf("%s %d", label, kindSeq[t.Kind])
		}
		label = fmt.Sprintf(" %d:%s ", i+1, label)

		var rendered string
		if i == activeIdx {
			rendered = tabStyles.Active.Render(label)
		} else {
			rendered = tabStyles.Inactive.Render(label)
		}
		w := ansi.StringWidth(rendered)
		if x >= cursor && x < cursor+w {
			return i
		}
		cursor += w

		// Account for separator.
		if i < len(tabs)-1 {
			sep := tabStyles.Sep.Render("│")
			cursor += ansi.StringWidth(sep)
		}
	}
	return -1
}
