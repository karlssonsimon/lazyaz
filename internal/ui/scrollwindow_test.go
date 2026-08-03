package ui

import "testing"

// win is a compact constructor for the table tests below.
func win(cursor, offset, height, count, scrolloff int) ScrollWindow {
	return ScrollWindow{Cursor: cursor, Offset: offset, Height: height, Count: count, Scrolloff: scrolloff}
}

// The reported bug: with the cursor on the last visible row, moving down
// must scroll the window by one and leave the cursor on the last row.
// The paginated behavior jumped the window a whole page and put the
// cursor back at the top.
func TestMoveCursorAtBottomScrollsByOne(t *testing.T) {
	w := win(9, 0, 10, 100, 0).MoveCursor(1)

	if w.Cursor != 10 {
		t.Errorf("Cursor = %d, want 10", w.Cursor)
	}
	if w.Offset != 1 {
		t.Errorf("Offset = %d, want 1 (window scrolls by one row)", w.Offset)
	}
	if got := w.Cursor - w.Offset; got != 9 {
		t.Errorf("cursor sits at row %d of the window, want it to stay on row 9", got)
	}
}

func TestMoveCursorAtTopScrollsByOne(t *testing.T) {
	w := win(20, 20, 10, 100, 0).MoveCursor(-1)

	if w.Cursor != 19 {
		t.Errorf("Cursor = %d, want 19", w.Cursor)
	}
	if w.Offset != 19 {
		t.Errorf("Offset = %d, want 19", w.Offset)
	}
}

func TestMoveCursor(t *testing.T) {
	tests := []struct {
		name       string
		start      ScrollWindow
		delta      int
		wantCursor int
		wantOffset int
	}{
		{"inside window does not scroll", win(3, 0, 10, 100, 0), 1, 4, 0},
		{"clamps at first item", win(0, 0, 10, 100, 0), -5, 0, 0},
		{"clamps at last item", win(99, 90, 10, 100, 0), 5, 99, 90},
		{"jump to end pins window to the tail", win(0, 0, 10, 100, 0), 99, 99, 90},
		{"jump to start pins window to the head", win(99, 90, 10, 100, 0), -99, 0, 0},
		{"list shorter than window never scrolls", win(2, 0, 10, 5, 0), 1, 3, 0},
		{"empty list stays put", win(0, 0, 10, 0, 0), 1, 0, 0},
		{"height of one advances offset with cursor", win(5, 5, 1, 100, 0), 1, 6, 6},

		// scrolloff keeps context rows below the cursor, so the window
		// starts moving before the cursor reaches the edge.
		{"scrolloff scrolls early at the bottom", win(6, 0, 10, 100, 3), 1, 7, 1},
		{"scrolloff scrolls early at the top", win(20, 18, 10, 100, 3), -1, 19, 16},

		// At the extremes the hard bounds win, so the cursor can still
		// reach the very first and very last row.
		{"scrolloff does not pad past the start", win(3, 0, 10, 100, 3), -3, 0, 0},
		{"scrolloff does not pad past the end", win(96, 90, 10, 100, 3), 3, 99, 90},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.start.MoveCursor(tt.delta)
			if got.Cursor != tt.wantCursor || got.Offset != tt.wantOffset {
				t.Errorf("MoveCursor(%d) = cursor %d offset %d, want cursor %d offset %d",
					tt.delta, got.Cursor, got.Offset, tt.wantCursor, tt.wantOffset)
			}
		})
	}
}

// Scrolloff can never claim more than half the window, or the two
// constraints would cross and no offset would satisfy both. Capped at
// (height-1)/2 = 4 there are two legal rows for the cursor, 4 and 5, so
// this asserts the band rather than one exact row.
func TestScrolloffLargerThanWindowIsCapped(t *testing.T) {
	w := win(50, 46, 10, 100, 999)
	for i := 0; i < 20; i++ {
		w = w.MoveCursor(1)
		row := w.Cursor - w.Offset
		if row < 4 || row > 5 {
			t.Fatalf("after %d moves the cursor sits at window row %d, want 4 or 5", i+1, row)
		}
	}
	if w.Cursor != 70 {
		t.Errorf("Cursor = %d, want 70", w.Cursor)
	}
}

func TestZMotionsMoveViewNotCursor(t *testing.T) {
	tests := []struct {
		name       string
		start      ScrollWindow
		motion     func(ScrollWindow) ScrollWindow
		wantOffset int
	}{
		{"zt puts cursor on the top row", win(40, 31, 10, 100, 0), ScrollWindow.CursorToTop, 40},
		{"zz centers the cursor", win(40, 31, 10, 100, 0), ScrollWindow.CenterOnCursor, 36},
		{"zb puts cursor on the bottom row", win(40, 40, 10, 100, 0), ScrollWindow.CursorToBottom, 31},

		// Scrolloff offsets zt/zb by the reserved context rows.
		{"zt respects scrolloff", win(40, 31, 10, 100, 3), ScrollWindow.CursorToTop, 37},
		{"zb respects scrolloff", win(40, 40, 10, 100, 3), ScrollWindow.CursorToBottom, 34},

		// Near the edges the window can only go so far; the cursor is
		// left alone rather than dragged along.
		{"zt near the end clamps to the tail", win(97, 90, 10, 100, 0), ScrollWindow.CursorToTop, 90},
		{"zz near the start clamps to the head", win(2, 0, 10, 100, 0), ScrollWindow.CenterOnCursor, 0},
		{"list shorter than window stays at zero", win(3, 0, 10, 5, 0), ScrollWindow.CenterOnCursor, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.motion(tt.start)
			if got.Cursor != tt.start.Cursor {
				t.Errorf("cursor moved to %d, want it to stay at %d", got.Cursor, tt.start.Cursor)
			}
			if got.Offset != tt.wantOffset {
				t.Errorf("Offset = %d, want %d", got.Offset, tt.wantOffset)
			}
		})
	}
}

func TestScrollBy(t *testing.T) {
	tests := []struct {
		name       string
		start      ScrollWindow
		delta      int
		wantCursor int
		wantOffset int
	}{
		{"ctrl+e leaves an interior cursor alone", win(45, 40, 10, 100, 0), 1, 45, 41},
		{"ctrl+y leaves an interior cursor alone", win(45, 40, 10, 100, 0), -1, 45, 39},
		{"ctrl+e pushes a cursor that would fall off the top", win(40, 40, 10, 100, 0), 1, 41, 41},
		{"ctrl+y pushes a cursor that would fall off the bottom", win(49, 40, 10, 100, 0), -1, 48, 39},
		{"ctrl+e pushes against scrolloff", win(43, 40, 10, 100, 3), 1, 44, 41},
		{"ctrl+y pushes against scrolloff", win(46, 40, 10, 100, 3), -1, 45, 39},
		{"cannot scroll above the head", win(3, 0, 10, 100, 0), -1, 3, 0},
		{"cannot scroll past the tail", win(95, 90, 10, 100, 0), 1, 95, 90},
		{"list shorter than window does not scroll", win(2, 0, 10, 5, 0), 1, 2, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.start.ScrollBy(tt.delta)
			if got.Cursor != tt.wantCursor || got.Offset != tt.wantOffset {
				t.Errorf("ScrollBy(%d) = cursor %d offset %d, want cursor %d offset %d",
					tt.delta, got.Cursor, got.Offset, tt.wantCursor, tt.wantOffset)
			}
		})
	}
}

// Normalize is what callers use after the item count changes underneath
// them — a filter narrowing the list, a refresh dropping rows.
func TestNormalize(t *testing.T) {
	tests := []struct {
		name       string
		start      ScrollWindow
		wantCursor int
		wantOffset int
	}{
		{"cursor past the end clamps back", win(500, 490, 10, 100, 0), 99, 90},
		{"offset past the end clamps back", win(5, 500, 10, 100, 0), 5, 5},
		{"negative cursor clamps to zero", win(-4, 0, 10, 100, 0), 0, 0},
		{"negative offset clamps to zero", win(5, -4, 10, 100, 0), 5, 0},
		{"empty list resets both", win(9, 9, 10, 0, 0), 0, 0},
		{"already valid is untouched", win(50, 45, 10, 100, 0), 50, 45},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.start.Normalize()
			if got.Cursor != tt.wantCursor || got.Offset != tt.wantOffset {
				t.Errorf("Normalize() = cursor %d offset %d, want cursor %d offset %d",
					got.Cursor, got.Offset, tt.wantCursor, tt.wantOffset)
			}
		})
	}
}

// A zero or negative height means the pane has no room to draw; the
// arithmetic must not divide by zero or produce a negative offset.
func TestDegenerateHeight(t *testing.T) {
	for _, h := range []int{0, -1} {
		w := win(50, 40, h, 100, 3)
		for name, got := range map[string]ScrollWindow{
			"MoveCursor":     w.MoveCursor(1),
			"ScrollBy":       w.ScrollBy(1),
			"CenterOnCursor": w.CenterOnCursor(),
			"CursorToTop":    w.CursorToTop(),
			"CursorToBottom": w.CursorToBottom(),
			"Normalize":      w.Normalize(),
		} {
			if got.Offset < 0 {
				t.Errorf("height=%d %s produced negative offset %d", h, name, got.Offset)
			}
			if got.Cursor < 0 {
				t.Errorf("height=%d %s produced negative cursor %d", h, name, got.Cursor)
			}
		}
	}
}

// VisibleBounds is what the renderer slices with, so it must never
// produce a range that would panic on a slice expression.
func TestVisibleBounds(t *testing.T) {
	tests := []struct {
		name             string
		win              ScrollWindow
		wantLo, wantHigh int
	}{
		{"full window", win(0, 10, 10, 100, 0), 10, 20},
		{"tail is short", win(0, 95, 10, 100, 0), 95, 100},
		{"list shorter than window", win(0, 0, 10, 3, 0), 0, 3},
		{"empty list", win(0, 0, 10, 0, 0), 0, 0},
		{"offset past the end yields an empty range", win(0, 500, 10, 100, 0), 100, 100},
		{"zero height yields an empty range", win(0, 5, 0, 100, 0), 5, 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lo, hi := tt.win.VisibleBounds()
			if lo != tt.wantLo || hi != tt.wantHigh {
				t.Errorf("VisibleBounds() = (%d, %d), want (%d, %d)", lo, hi, tt.wantLo, tt.wantHigh)
			}
			if lo < 0 || hi < lo || hi > tt.win.Count {
				t.Errorf("VisibleBounds() = (%d, %d) is not a valid slice range for count %d", lo, hi, tt.win.Count)
			}
		})
	}
}
