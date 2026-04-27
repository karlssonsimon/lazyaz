package ui

import (
	"time"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
)

const doubleClickThreshold = 400 * time.Millisecond

// ClickTracker detects double-clicks. Embed in app models.
type ClickTracker struct {
	lastClickTime time.Time
	lastClickX    int
	lastClickY    int
}

// Click records a click and returns true if it was a double-click
// (two clicks within the threshold at the same position).
func (c *ClickTracker) Click(x, y int) bool {
	now := time.Now()
	double := now.Sub(c.lastClickTime) < doubleClickThreshold &&
		c.lastClickX == x && c.lastClickY == y
	c.lastClickTime = now
	c.lastClickX = x
	c.lastClickY = y
	return double
}

// TabBarHeight is the height of the tab bar rendered by the parent tabapp.
// Child apps in embedded mode add this to their Y calculations so absolute
// screen coordinates map correctly to their layout.
const TabBarHeight = 1

var lastMouseEvent time.Time

// MouseEventFilter debounces rapid trackpad scroll/motion events to
// prevent overwhelming the event loop. Pass to tea.WithFilter when
// creating the program.
func MouseEventFilter(m tea.Model, msg tea.Msg) tea.Msg {
	switch msg.(type) {
	case tea.MouseWheelMsg, tea.MouseMotionMsg:
		now := time.Now()
		if now.Sub(lastMouseEvent) < 15*time.Millisecond {
			return nil
		}
		lastMouseEvent = now
	}
	return msg
}

// VisiblePane describes a pane's on-screen position for mouse hit-testing.
type VisiblePane struct {
	Index int // logical pane index (e.g. accountsPane, containersPane)
	X     int // screen X of the pane's left edge
	Width int // total block width of the pane
}

// PaneAtX returns the VisiblePane whose X range contains screenX, or nil.
func PaneAtX(panes []VisiblePane, screenX int) *VisiblePane {
	for i := range panes {
		if screenX >= panes[i].X && screenX < panes[i].X+panes[i].Width {
			return &panes[i]
		}
	}
	return nil
}

// listHeaderRows returns the number of rows the list.View() renders
// above the actual items. Measured empirically: the bubbles list renders
// an empty title row (when filtering is enabled but idle), the status
// bar, and then a blank separator line before items start.
func listHeaderRows(l *list.Model) int {
	rows := 0
	if l.FilteringEnabled() {
		rows++ // empty title/filter row
	}
	if l.ShowStatusBar() {
		rows++ // "N items" row
		rows++ // blank separator between status bar and items
	}
	return rows
}

// ListItemAtY returns the item index in the list's VisibleItems for a
// click at the given Y offset within the list's rendered content area
// (i.e. the list.View() output area). itemHeight is the rendered height
// of each item (delegate Height + Spacing). Returns -1 if out of range.
func ListItemAtY(l *list.Model, y, itemHeight int) int {
	y -= listHeaderRows(l)
	if y < 0 || itemHeight <= 0 {
		return -1
	}
	offset := y / itemHeight
	first := l.Paginator.Page * l.Paginator.PerPage
	idx := first + offset
	visible := len(l.VisibleItems())
	if idx < 0 || idx >= visible {
		return -1
	}
	return idx
}

// MillerColumnContentYStart returns the absolute Y where a flat Miller
// column's body starts, directly under the external column title row.
func MillerColumnContentYStart(areaY int) int {
	return areaY + 1
}
