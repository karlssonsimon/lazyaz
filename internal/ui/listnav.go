package ui

import (
	"strings"

	"charm.land/bubbles/v2/list"
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

// ListState captures the user-facing state of a [list.Model] at a point
// in time: where the cursor is (by stable item key) and what the filter
// text was. Use SnapshotListState and RestoreListState to persist state
// across a scope change so navigating back to a previously visited scope
// lands the user where they left off.
//
// The cursor is stored by key rather than numeric index so that items
// arriving in a different order after a refresh still resolve to the
// correct row.
type ListState struct {
	CursorKey string
	Filter    string
}

// SnapshotListState captures the current cursor and filter of the given
// list. The returned ListState can later be passed to RestoreListState
// to put the list back in the same user-facing state.
func SnapshotListState(l *list.Model, keyOf func(list.Item) string) ListState {
	var cursorKey string
	if sel := l.SelectedItem(); sel != nil {
		cursorKey = keyOf(sel)
	}
	return ListState{
		CursorKey: cursorKey,
		Filter:    l.FilterValue(),
	}
}

// RestoreListState applies a previously-captured [ListState] to the list.
// The caller is expected to have already populated the list's items via
// SetItems (or similar). Restoration happens in two steps:
//
//  1. If state.Filter is non-empty, it is reapplied via list.SetFilterText,
//     which re-runs the filter against the new items. Otherwise the filter
//     is cleared.
//  2. The cursor is placed on the item whose key matches state.CursorKey.
//     If no such item exists (e.g. it was deleted server-side since the
//     snapshot), the cursor is reset to the top.
//
// Passing the zero value ListState{} is valid and means "fresh scope":
// the list ends up unfiltered with the cursor at the top. This is the
// common case when a user navigates into a container/vault/namespace
// they've never visited before — the stale cursor index from the
// previous scope must not carry over.
func RestoreListState(l *list.Model, state ListState, keyOf func(list.Item) string) {
	if state.Filter != "" {
		l.SetFilterText(state.Filter)
	} else {
		l.ResetFilter()
	}

	if state.CursorKey != "" {
		items := l.VisibleItems()
		for i, it := range items {
			if keyOf(it) == state.CursorKey {
				l.Select(i)
				return
			}
		}
	}

	// No saved cursor key, or the saved item is no longer present —
	// start at the top.
	l.Select(0)
}

// SetItemsPreserveKey replaces list items while keeping the cursor
// pointed at the same item by identity (via keyOf), not by numeric index.
// If the previously selected item is no longer present, the cursor clamps
// to the same numeric index as before — matching SetItemsPreserveIndex's
// fallback.
//
// If a filter is active, it is re-applied synchronously against the new
// items via list.SetFilterText — bubbles list's SetItems otherwise
// returns a tea.Cmd to re-run the filter asynchronously, which would
// briefly show an empty list between frames.
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
	prevFilter := l.FilterValue()
	wasFiltering := l.FilterState() == list.Filtering

	l.SetItems(items)
	if prevFilter != "" {
		l.SetFilterText(prevFilter)
		if wasFiltering {
			l.SetFilterState(list.Filtering)
		}
	}

	visible := l.VisibleItems()
	n := len(visible)
	if n == 0 {
		return
	}

	if prevKey != "" {
		for i, it := range visible {
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
