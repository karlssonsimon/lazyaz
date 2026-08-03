package ui

// ScrollWindow is the arithmetic behind neovim-style scrolling: a
// window of Height rows onto a list of Count rows, with the cursor at
// some absolute index and the window starting at Offset.
//
// It deliberately knows nothing about lists, viewports or rendering, so
// the list panes and the blob preview can share one implementation of
// the motions and one set of tests.
//
// Every method returns a new ScrollWindow; nothing mutates in place.
type ScrollWindow struct {
	Cursor    int // selected row, absolute
	Offset    int // first visible row, absolute
	Height    int // visible rows
	Count     int // total rows
	Scrolloff int // rows of context kept above and below the cursor
}

// MoveCursor moves the cursor by delta and scrolls the window the
// minimum amount needed to keep it in view.
func (w ScrollWindow) MoveCursor(delta int) ScrollWindow {
	w.Cursor += delta
	return w.Normalize()
}

// CursorTo places the cursor at an absolute index, scrolling to follow.
func (w ScrollWindow) CursorTo(index int) ScrollWindow {
	w.Cursor = index
	return w.Normalize()
}

// CenterOnCursor is zz: the cursor keeps its index, the window moves so
// the cursor sits in the middle.
func (w ScrollWindow) CenterOnCursor() ScrollWindow {
	return w.moveViewTo(w.Cursor - (w.Height-1)/2)
}

// CursorToTop is zt: the window moves so the cursor sits on the top row,
// less whatever context Scrolloff reserves above it.
func (w ScrollWindow) CursorToTop() ScrollWindow {
	return w.moveViewTo(w.Cursor - w.context())
}

// CursorToBottom is zb: the window moves so the cursor sits on the
// bottom row, less whatever context Scrolloff reserves below it.
func (w ScrollWindow) CursorToBottom() ScrollWindow {
	return w.moveViewTo(w.Cursor - w.Height + 1 + w.context())
}

// ScrollBy is ctrl+e / ctrl+y: the window moves by delta rows and the
// cursor only follows if the window would otherwise leave it behind.
func (w ScrollWindow) ScrollBy(delta int) ScrollWindow {
	w.Offset = clampInt(w.Offset+delta, 0, w.maxOffset())

	if w.Height < 1 || w.Count < 1 {
		return w.Normalize()
	}

	so := w.context()
	if top := w.Offset + so; w.Cursor < top {
		w.Cursor = top
	}
	if bottom := w.Offset + w.Height - 1 - so; w.Cursor > bottom {
		w.Cursor = bottom
	}
	w.Cursor = clampInt(w.Cursor, 0, w.Count-1)
	return w
}

// Normalize brings a window back into a valid state. Callers use it
// after the row count changes underneath them — a filter narrowing the
// list, a refresh dropping rows, a pane resize.
func (w ScrollWindow) Normalize() ScrollWindow {
	if w.Count < 1 {
		w.Cursor = 0
		w.Offset = 0
		return w
	}

	w.Cursor = clampInt(w.Cursor, 0, w.Count-1)

	if w.Height < 1 {
		w.Offset = clampInt(w.Offset, 0, w.maxOffset())
		return w
	}

	// Keep Scrolloff rows of context on each side of the cursor. The cap
	// in context() guarantees lo <= hi, so the two bounds never cross.
	so := w.context()
	lo := w.Cursor - w.Height + 1 + so
	hi := w.Cursor - so
	w.Offset = clampInt(w.Offset, lo, hi)

	// The list's own edges win over the context rows, so the cursor can
	// still reach the very first and very last row.
	w.Offset = clampInt(w.Offset, 0, w.maxOffset())
	return w
}

// VisibleBounds returns the half-open range of rows to render. It is
// always a valid slice range for a slice of length Count.
func (w ScrollWindow) VisibleBounds() (int, int) {
	if w.Count < 1 || w.Height < 1 {
		return clampInt(w.Offset, 0, maxInt(0, w.Count)), clampInt(w.Offset, 0, maxInt(0, w.Count))
	}
	lo := clampInt(w.Offset, 0, w.Count)
	hi := clampInt(lo+w.Height, lo, w.Count)
	return lo, hi
}

// CursorRow is where the cursor falls within the window, counting from
// the top visible row. Returns -1 when the cursor is out of view.
func (w ScrollWindow) CursorRow() int {
	row := w.Cursor - w.Offset
	if row < 0 || row >= w.Height {
		return -1
	}
	return row
}

// moveViewTo is the shared tail of the z motions: place the window and
// clamp it to the list, leaving the cursor untouched. Near the ends the
// window runs out of room, and vim leaves the cursor where it is rather
// than dragging it along.
func (w ScrollWindow) moveViewTo(offset int) ScrollWindow {
	w.Offset = clampInt(offset, 0, w.maxOffset())
	return w
}

// context is the number of rows kept between the cursor and each edge.
// Scrolloff is capped at half the window: any more and the "keep N above"
// and "keep N below" constraints would cross and leave no valid offset.
func (w ScrollWindow) context() int {
	if w.Height < 1 {
		return 0
	}
	return minInt(maxInt(w.Scrolloff, 0), maxInt(0, (w.Height-1)/2))
}

func (w ScrollWindow) maxOffset() int {
	return maxInt(0, w.Count-w.Height)
}

func clampInt(v, lo, hi int) int {
	if hi < lo {
		lo, hi = hi, lo
	}
	return minInt(hi, maxInt(lo, v))
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
