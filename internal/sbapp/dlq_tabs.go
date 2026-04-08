package sbapp

import (
	"fmt"

	"charm.land/lipgloss/v2"
)

// dlqTabsHeight is the rendered height of the active/DLQ tab strip
// shown at the top of the detail pane when peeking. One line for the
// tabs themselves, plus a blank spacer below for breathing room.
const dlqTabsHeight = 2

// currentMessageCounts returns the active and dead-letter message
// counts for the entity currently being peeked. For queues, the counts
// come from m.currentEntity directly. For topic-subs, we look up the
// sub in the per-topic map. ok is false if no peek is in progress or
// the sub data isn't available yet.
func (m Model) currentMessageCounts() (active, dead int64, ok bool) {
	if !m.hasPeekTarget {
		return 0, 0, false
	}
	if m.currentSubName == "" {
		return m.currentEntity.ActiveMsgCount, m.currentEntity.DeadLetterCount, true
	}
	for _, sub := range m.topicSubsByTopic[m.currentEntity.Name] {
		if sub.Name == m.currentSubName {
			return sub.ActiveMsgCount, sub.DeadLetterCount, true
		}
	}
	return 0, 0, false
}

// renderDLQTabs renders the active/DLQ tab strip for the detail pane.
// Visually two filled tabs with a separator between, designed to read
// as real tabs sitting at the top of the pane:
//
//	 Active (0)  │  DLQ (541)
//
// The selected tab gets a filled background (accent for Active, danger
// for DLQ); the inactive tab is muted with a faint background. The
// strip is padded out to the full pane width so the bar background
// runs to the right edge of the pane content area.
func (m Model) renderDLQTabs(width int) string {
	if !m.hasPeekTarget {
		return ""
	}
	active, dead, _ := m.currentMessageCounts()

	tb := m.Styles.TabBar

	// Base styles, copied from TabBarStyles so each render is isolated.
	activeTabBase := tb.Active.Copy()
	inactiveTabBase := tb.Inactive.Copy()

	// DLQ tab uses danger color regardless of selection so the
	// affordance is visible from across the pane.
	dlqSelected := activeTabBase.Copy().
		Background(m.Styles.Danger.GetForeground()).
		Foreground(lipgloss.Color("0")) // dark text on red fill
	dlqUnselected := inactiveTabBase.Copy().
		Foreground(m.Styles.Danger.GetForeground())

	activeText := fmt.Sprintf(" Active (%d) ", active)
	dlqText := fmt.Sprintf(" DLQ (%d) ", dead)

	var activeRendered, dlqRendered string
	if m.deadLetter {
		activeRendered = inactiveTabBase.Render(activeText)
		dlqRendered = dlqSelected.Render(dlqText)
	} else {
		activeRendered = activeTabBase.Render(activeText)
		dlqRendered = dlqUnselected.Render(dlqText)
	}

	sep := tb.Sep.Render("│")
	bar := activeRendered + sep + dlqRendered

	// Pad/fill to the full content width with the bar background so the
	// tabs sit on a single visual band rather than floating mid-pane.
	if w := lipgloss.Width(bar); w < width {
		bar += tb.Bar.Render(" " + spaces(width-w-1))
	}

	return bar + "\n"
}

func spaces(n int) string {
	if n <= 0 {
		return ""
	}
	out := make([]byte, n)
	for i := range out {
		out[i] = ' '
	}
	return string(out)
}
