package app

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/karlssonsimon/lazyaz/internal/ui"
)

const (
	tabGap       = "    " // four spaces between tabs
	tabBarHeight = 1
)

// renderTabBar paints a single-line bar:
//
//	 1 ▦ Dashboard    2 ▲ Blob    3 ⌬ Key Vault    4 ⇄ Service Bus
//
// Active tab is marked by a selBg-highlighted card + bold name +
// accent-colored icon. Inactive tabs sit muted on the bar bg.
func renderTabBar(tabs []Tab, activeIdx int, tabStyles ui.TabBarStyles, width int) string {
	if len(tabs) == 0 {
		return tabStyles.Bar.Render(strings.Repeat(" ", width))
	}

	kindCount := map[TabKind]int{}
	for _, t := range tabs {
		kindCount[t.Kind]++
	}
	kindSeq := map[TabKind]int{}

	parts := make([]string, 0, 2*len(tabs))
	gap := tabStyles.Sep.Render(tabGap)
	for i, t := range tabs {
		kindSeq[t.Kind]++
		name := t.Kind.String()
		if kindCount[t.Kind] > 1 {
			name = fmt.Sprintf("%s %d", name, kindSeq[t.Kind])
		}
		number := fmt.Sprintf(" %d ", i+1)
		icon := t.Kind.Icon()

		var rendered string
		if i == activeIdx {
			rendered = tabStyles.ActiveNumber.Render(number) +
				tabStyles.ActiveIcon.Render(icon) +
				tabStyles.Active.Render(" "+name+" ")
		} else {
			rendered = tabStyles.Number.Render(number) +
				tabStyles.InactiveIcon.Render(icon) +
				tabStyles.Inactive.Render(" "+name+" ")
		}
		if i > 0 {
			parts = append(parts, gap)
		}
		parts = append(parts, rendered)
	}

	line := strings.Join(parts, "")
	if w := ansi.StringWidth(line); w < width {
		line += tabStyles.Bar.Render(strings.Repeat(" ", width-w))
	}
	return line
}

// tabIndexAtX returns the tab index for a click at screen column x,
// or -1 if the click doesn't land on any tab label.
func tabIndexAtX(tabs []Tab, activeIdx int, tabStyles ui.TabBarStyles, x int) int {
	if len(tabs) == 0 {
		return -1
	}

	kindCount := map[TabKind]int{}
	for _, t := range tabs {
		kindCount[t.Kind]++
	}
	kindSeq := map[TabKind]int{}

	cursor := 0
	for i, t := range tabs {
		kindSeq[t.Kind]++
		name := t.Kind.String()
		if kindCount[t.Kind] > 1 {
			name = fmt.Sprintf("%s %d", name, kindSeq[t.Kind])
		}
		number := fmt.Sprintf(" %d ", i+1)
		icon := t.Kind.Icon()
		labelText := icon + " " + name + " "
		w := ansi.StringWidth(number) + ansi.StringWidth(labelText)
		if x >= cursor && x < cursor+w {
			return i
		}
		cursor += w
		if i < len(tabs)-1 {
			cursor += ansi.StringWidth(tabGap)
		}
	}
	return -1
}
