package ui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
)

// List wraps bubbles' list.Model with a scroll offset so the view moves
// a row at a time instead of a page at a time.
//
// bubbles renders the window [Page*PerPage, Page*PerPage+PerPage), so an
// offset that is not a multiple of PerPage cannot be expressed: stepping
// off the bottom row flips to the next page and drops the cursor at the
// top. List keeps its own offset and renders the window itself.
//
// The embedded list.Model still owns items, filtering and delegates, and
// its methods are promoted, so callers use Index, VisibleItems, SetItems
// and the rest exactly as before. Only Paginator.Page loses its meaning
// — PerPage stays valid as "rows that fit", which is what it always was.
type List struct {
	list.Model

	// bubbles has no Delegate() getter, so SetDelegate and the
	// constructor keep a copy for rendering.
	delegate  list.ItemDelegate
	offset    int
	scrolloff int

	// countCache holds len(VisibleItems). With a filter active that call
	// copies the whole matched slice — megabytes per keystroke at 200k
	// rows — so it is resolved once per frame and reused by the cursor
	// moves in between. RenderWindow refreshes it from the slice it
	// already holds, so whatever is drawn always comes from a true
	// count; the key below only has to cover the gap between frames.
	countCache    int
	countCacheKey listCountKey

	// visibleVersion counts mutations of the visible item set. Bumped by
	// the shadowed mutators below, so every path that can reshape what
	// the user sees bumps it automatically — vim.Visual caches its
	// anchor-index resolution against this number instead of comparing
	// filter signatures by hand.
	visibleVersion int
}

// listCountKey captures the inputs that change how many items a filter
// matches. A refresh that swaps in the same number of different items
// under the same filter text can slip through, which would leave one
// cursor move working from a stale count before the next render
// corrects it — worth it to keep keystrokes off an O(n) copy.
type listCountKey struct {
	items  int
	state  list.FilterState
	filter string
}

// NewList builds a List around a bubbles list with the same arguments.
func NewList(items []list.Item, delegate list.ItemDelegate, width, height int) List {
	return List{
		Model:    list.New(items, delegate, width, height),
		delegate: delegate,
	}
}

// VisibleVersion identifies the current visible item set. Any mutation
// that can change which items are visible — new items, filter edits —
// yields a new version. Cache resolved indices against it.
func (l *List) VisibleVersion() int {
	return l.visibleVersion
}

// SetItems shadows list.Model.SetItems to version the visible set.
func (l *List) SetItems(items []list.Item) tea.Cmd {
	l.visibleVersion++
	return l.Model.SetItems(items)
}

// SetFilterText shadows list.Model.SetFilterText to version the
// visible set.
func (l *List) SetFilterText(s string) {
	l.visibleVersion++
	l.Model.SetFilterText(s)
}

// ResetFilter shadows list.Model.ResetFilter to version the visible set.
func (l *List) ResetFilter() {
	l.visibleVersion++
	l.Model.ResetFilter()
}

// SetDelegate shadows list.Model.SetDelegate to keep the delegate
// reachable for rendering.
func (l *List) SetDelegate(d list.ItemDelegate) {
	l.Model.SetDelegate(d)
	l.delegate = d
}

// SetScrolloff sets how many rows of context are kept between the cursor
// and the edges of the window.
func (l *List) SetScrolloff(n int) {
	l.scrolloff = n
}

// Offset is the index of the first visible row. Mouse hit-testing needs
// it to turn a screen row back into an item.
func (l *List) Offset() int {
	return l.offset
}

// Window snapshots the current scroll state for the motion arithmetic.
func (l *List) Window() ScrollWindow {
	return l.windowOf(l.visibleCount())
}

func (l *List) windowOf(count int) ScrollWindow {
	return ScrollWindow{
		Cursor:    l.Index(),
		Offset:    l.offset,
		Height:    l.rows(),
		Count:     count,
		Scrolloff: l.scrolloff,
	}
}

// visibleCount is len(VisibleItems) without the copy that call makes
// while a filter is active. Unfiltered, Items is the backing slice and
// costs nothing.
func (l *List) visibleCount() int {
	if l.FilterState() == list.Unfiltered {
		return len(l.Items())
	}
	if key := l.countKey(); key == l.countCacheKey {
		return l.countCache
	}
	return l.cacheVisibleCount(len(l.VisibleItems()))
}

func (l *List) cacheVisibleCount(n int) int {
	l.countCache = n
	l.countCacheKey = l.countKey()
	return n
}

func (l *List) countKey() listCountKey {
	return listCountKey{
		items:  len(l.Items()),
		state:  l.FilterState(),
		filter: l.FilterValue(),
	}
}

func (l *List) apply(w ScrollWindow) {
	l.offset = w.Offset
	if w.Cursor != l.Index() {
		// Model.Select, not the shadow below — that one normalizes, and
		// this is already the result of normalizing.
		l.Model.Select(w.Cursor)
	}
}

// Update shadows list.Model.Update so bubbles stays the cursor
// authority — its CursorUp/CursorDown bindings are configured from the
// user's keymap — while the window offset is re-derived here instead of
// coming from its paginator. Moving off the bottom row lands the cursor
// one row further on; Normalize then scrolls the window by the single
// row needed to keep it in view, rather than flipping a page.
func (l List) Update(msg tea.Msg) (List, tea.Cmd) {
	before := l.Index()
	inner, cmd := l.Model.Update(msg)
	l.Model = inner
	if l.Index() != before {
		l.NormalizeWindow()
	}
	return l, cmd
}

// CursorDown shadows list.Model.CursorDown so direct calls scroll the
// window instead of paging it.
func (l *List) CursorDown() {
	l.Model.CursorDown()
	l.NormalizeWindow()
}

// CursorUp shadows list.Model.CursorUp for the same reason.
func (l *List) CursorUp() {
	l.Model.CursorUp()
	l.NormalizeWindow()
}

// Select shadows list.Model.Select so programmatic jumps (jump list,
// dashboard drill-ins, filter restores) bring the window with them.
func (l *List) Select(i int) {
	l.Model.Select(i)
	l.NormalizeWindow()
}

// GoToStart shadows list.Model.GoToStart.
func (l *List) GoToStart() {
	l.Model.GoToStart()
	l.NormalizeWindow()
}

// GoToEnd shadows list.Model.GoToEnd.
func (l *List) GoToEnd() {
	l.Model.GoToEnd()
	l.NormalizeWindow()
}

// MoveCursor moves the cursor by delta rows, scrolling to follow.
func (l *List) MoveCursor(delta int) { l.apply(l.Window().MoveCursor(delta)) }

// SelectIndex moves the cursor to an absolute index, scrolling to follow.
func (l *List) SelectIndex(i int) { l.apply(l.Window().CursorTo(i)) }

// ScrollBy moves the window by delta rows (ctrl+e / ctrl+y), pushing the
// cursor only when the window would leave it behind.
func (l *List) ScrollBy(delta int) { l.apply(l.Window().ScrollBy(delta)) }

// CenterOnCursor is zz.
func (l *List) CenterOnCursor() { l.apply(l.Window().CenterOnCursor()) }

// CursorToTop is zt.
func (l *List) CursorToTop() { l.apply(l.Window().CursorToTop()) }

// CursorToBottom is zb.
func (l *List) CursorToBottom() { l.apply(l.Window().CursorToBottom()) }

// NormalizeWindow re-clamps the window after the item count changes
// underneath it — a filter narrowing the list, a refresh dropping rows,
// a pane resize.
func (l *List) NormalizeWindow() { l.apply(l.Window().Normalize()) }

// rows is how many items fit in the window. bubbles already computes
// this as max(1, availHeight/(delegateHeight+spacing)) and stores it in
// PerPage, which stays meaningful even though Page does not.
func (l *List) rows() int {
	return l.Paginator.PerPage
}

// RenderWindow draws the visible slice of the list, replacing
// list.Model.View, which would re-impose pagination. Title, status bar,
// pagination and help are disabled on every list in this app, so the
// only behavior left to reproduce is the delegate loop and the empty
// state.
//
// The result is exactly the visible rows, unpadded — RenderMillerColumn
// runs the body through fitContent, which pads short output to the
// column height and truncates anything longer.
func (l *List) RenderWindow() string {
	// One VisibleItems call per frame: it is the authoritative count, so
	// refresh the cache from it and drive the whole render off this one
	// slice rather than asking again for the bounds.
	items := l.VisibleItems()
	l.cacheVisibleCount(len(items))

	// The count can have changed since the last cursor move (a filter, a
	// streaming refresh), so re-clamp before slicing.
	window := l.windowOf(len(items)).Normalize()
	l.apply(window)

	if len(items) == 0 {
		if l.FilterState() == list.Filtering {
			return ""
		}
		_, plural := l.StatusBarItemName()
		return l.Styles.NoItems.Render("No " + plural + ".")
	}

	lo, hi := window.VisibleBounds()

	var b strings.Builder
	for i := lo; i < hi; i++ {
		l.delegate.Render(&b, l.Model, i, items[i])
		if i < hi-1 {
			fmt.Fprint(&b, strings.Repeat("\n", l.delegate.Spacing()+1))
		}
	}
	return b.String()
}
