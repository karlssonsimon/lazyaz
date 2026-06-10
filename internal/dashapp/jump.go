package dashapp

import (
	"strings"

	"github.com/karlssonsimon/lazyaz/internal/jumplist"

	tea "charm.land/bubbletea/v2"
)

// dashNavSnapshot captures the user's position on the dashboard: the
// focused widget (by title — stable across sessions, unlike the index)
// and its cursor row. The dashboard has no drill hierarchy of its own,
// so this exists mainly so cross-tab jumps (ctrl+o back to the
// dashboard) land on the widget the user was actually working in
// instead of a bare tab switch.
type dashNavSnapshot struct {
	subscriptionID string
	widgetTitle    string
	cursor         int
}

func (s dashNavSnapshot) Description() string {
	parts := []string{"dash"}
	if s.widgetTitle != "" {
		parts = append(parts, s.widgetTitle)
	}
	return strings.Join(parts, " / ")
}

// CurrentNav captures the focused widget + cursor. Returns nil when no
// subscription is set (nothing meaningful to come back to).
func (m Model) CurrentNav() jumplist.NavSnapshot {
	if !m.HasSubscription {
		return nil
	}
	snap := dashNavSnapshot{subscriptionID: m.CurrentSub.ID}
	if m.focusedIdx >= 0 && m.focusedIdx < len(m.widgets) {
		snap.widgetTitle = m.widgets[m.focusedIdx].Title()
		if m.focusedIdx < len(m.cursors) {
			snap.cursor = m.cursors[m.focusedIdx]
		}
	}
	return snap
}

// ApplyNav restores the focused widget and cursor. Cursor clamping at
// render time keeps an out-of-range row (data changed since the
// snapshot) harmless.
func (m *Model) ApplyNav(snap jumplist.NavSnapshot) tea.Cmd {
	s, ok := snap.(dashNavSnapshot)
	if !ok {
		return nil
	}
	for i, w := range m.widgets {
		if w.Title() == s.widgetTitle {
			m.focusedIdx = i
			if i < len(m.cursors) {
				m.cursors[i] = s.cursor
			}
			break
		}
	}
	return nil
}

func (m Model) WithAppliedNav(snap jumplist.NavSnapshot) (tea.Model, tea.Cmd) {
	cmd := m.ApplyNav(snap)
	return m, cmd
}
