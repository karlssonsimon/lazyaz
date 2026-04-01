package app

import (
	"fmt"
	"strings"

	"azure-storage/internal/ui"

	"github.com/charmbracelet/lipgloss"
)

func renderTabBar(tabs []Tab, activeIdx int, palette ui.Palette, width int) string {
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

	activeStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(palette.SelectedText)).
		Background(lipgloss.Color(palette.SelectedBg)).
		Padding(0, 1)

	inactiveStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(palette.Text)).
		Padding(0, 1)

	sepStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(palette.Border))

	var parts []string
	for i, t := range tabs {
		kindSeq[t.Kind]++
		label := t.Kind.String()
		if kindCount[t.Kind] > 1 {
			label = fmt.Sprintf("%s %d", label, kindSeq[t.Kind])
		}
		label = fmt.Sprintf(" %d:%s ", i+1, label)

		if i == activeIdx {
			parts = append(parts, activeStyle.Render(label))
		} else {
			parts = append(parts, inactiveStyle.Render(label))
		}

		if i < len(tabs)-1 {
			parts = append(parts, sepStyle.Render("│"))
		}
	}

	bar := strings.Join(parts, "")

	barLine := lipgloss.NewStyle().
		Width(width).
		Render(bar)

	return barLine
}
