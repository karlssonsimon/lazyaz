package ui

import (
	"strings"

	"github.com/charmbracelet/bubbles/list"
)

func ApplyFilterState(l *list.Model) {
	if strings.TrimSpace(l.FilterValue()) == "" {
		l.SetFilterState(list.Unfiltered)
		return
	}
	l.SetFilterState(list.FilterApplied)
}

func HalfPageStep(l list.Model) int {
	if l.Paginator.PerPage > 1 {
		if half := l.Paginator.PerPage / 2; half > 0 {
			return half
		}
	}

	if visible := len(l.VisibleItems()); visible > 1 {
		if half := visible / 2; half > 0 {
			return half
		}
	}

	return 1
}

func ClampListSelection(l *list.Model) {
	items := l.Items()
	if len(items) == 0 {
		l.Select(0)
		return
	}

	idx := l.Index()
	if idx < 0 {
		l.Select(0)
		return
	}
	if idx >= len(items) {
		l.Select(len(items) - 1)
	}
}

// SetItemsPreserveIndex replaces list items while keeping the cursor
// at the same index (clamped to the new item count). Use this for
// background refreshes where the user is already browsing the list.
func SetItemsPreserveIndex(l *list.Model, items []list.Item) {
	idx := l.Index()
	l.SetItems(items)
	if n := len(items); n == 0 {
		return
	} else if idx >= n {
		idx = n - 1
	}
	if idx < 0 {
		idx = 0
	}
	l.Select(idx)
}
