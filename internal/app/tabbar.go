package app

import (
	"fmt"
	"strings"

	"azure-storage/internal/ui"

	"github.com/charmbracelet/lipgloss"
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

	barLine := lipgloss.NewStyle().
		Width(width).
		Render(bar)

	return barLine
}
