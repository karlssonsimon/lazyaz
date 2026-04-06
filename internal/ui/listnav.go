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

// SetItemsPreserveKey replaces list items while keeping the cursor
// pointed at the same item by identity (via keyOf), not by numeric index.
// If the previously selected item is no longer present, the cursor clamps
// to the same numeric index as before — matching SetItemsPreserveIndex's
// fallback. Filter text/state is preserved by list.SetItems.
//
// Use this for streaming refreshes where items may reorder or disappear
// as pages arrive, so the cursor sticks to the thing the user was looking
// at rather than whichever item happens to land in the same slot.
func SetItemsPreserveKey(l *list.Model, items []list.Item, keyOf func(list.Item) string) {
	prevIdx := l.Index()
	prevKey := ""
	if selected := l.SelectedItem(); selected != nil {
		prevKey = keyOf(selected)
	}

	l.SetItems(items)

	n := len(items)
	if n == 0 {
		return
	}

	if prevKey != "" {
		for i, it := range items {
			if keyOf(it) == prevKey {
				l.Select(i)
				return
			}
		}
	}

	// Previous item is gone (or no prior selection) — clamp index.
	if prevIdx >= n {
		prevIdx = n - 1
	}
	if prevIdx < 0 {
		prevIdx = 0
	}
	l.Select(prevIdx)
}
