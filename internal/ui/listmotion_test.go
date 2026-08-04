package ui

import (
	"fmt"
	"testing"

	"github.com/karlssonsimon/lazyaz/internal/keymap"
	"github.com/karlssonsimon/lazyaz/internal/vim"

	"charm.land/bubbles/v2/list"
)

type motionItem string

func (m motionItem) FilterValue() string { return string(m) }
func (m motionItem) Title() string       { return string(m) }
func (m motionItem) Description() string { return "" }

// motionList builds a list of n items with a window of exactly want rows.
func motionList(t *testing.T, n int) *List {
	t.Helper()

	items := make([]list.Item, n)
	for i := range items {
		items[i] = motionItem(fmt.Sprintf("item-%03d", i))
	}
	// One row per item, matching how every list in the app is styled,
	// so the window height equals the row count.
	d := list.NewDefaultDelegate()
	d.ShowDescription = false
	d.SetHeight(1)
	d.SetSpacing(0)

	l := NewList(items, d, 40, 10)
	l.SetShowTitle(false)
	l.SetShowHelp(false)
	l.SetShowPagination(false)
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)
	l.SetSize(40, 10)
	return &l
}

func TestHandleListMotionZChord(t *testing.T) {
	km := keymap.Default()
	l := motionList(t, 200)
	rows := l.Window().Height
	l.Select(100)

	var r vim.Resolver

	// `z` alone only opens the chord; nothing moves yet.
	before := l.Offset()
	if got := HandleListMotion(l, km, "z", &r); got != MotionChordOpen {
		t.Fatalf("first z returned %v, want MotionChordOpen", got)
	}
	if l.Offset() != before {
		t.Errorf("offset moved to %d on the opening z, want it to stay at %d", l.Offset(), before)
	}

	// `zz` centers.
	if got := HandleListMotion(l, km, "z", &r); got != MotionHandled {
		t.Fatalf("zz returned %v, want MotionHandled", got)
	}
	// The chord is closed: a bare t is no motion.
	if got := HandleListMotion(l, km, "t", &r); got != MotionNone {
		t.Errorf("t after a completed chord returned %v, want MotionNone", got)
	}
	if want := 100 - (rows-1)/2; l.Offset() != want {
		t.Errorf("zz put the offset at %d, want %d", l.Offset(), want)
	}
	if l.Index() != 100 {
		t.Errorf("zz moved the cursor to %d, want it to stay at 100", l.Index())
	}

	// `zt` puts the cursor line on top.
	HandleListMotion(l, km, "z", &r)
	HandleListMotion(l, km, "t", &r)
	if l.Offset() != 100 {
		t.Errorf("zt put the offset at %d, want 100", l.Offset())
	}

	// `zb` puts it on the bottom row.
	HandleListMotion(l, km, "z", &r)
	HandleListMotion(l, km, "b", &r)
	if want := 100 - rows + 1; l.Offset() != want {
		t.Errorf("zb put the offset at %d, want %d", l.Offset(), want)
	}
}

// A mistyped chord is swallowed rather than passed on, so it cannot
// trigger some unrelated action bound to the second key.
func TestHandleListMotionUnknownChordKeyIsSwallowed(t *testing.T) {
	km := keymap.Default()
	l := motionList(t, 200)
	l.Select(100)

	var r vim.Resolver
	HandleListMotion(l, km, "z", &r)

	before := l.Offset()
	if got := HandleListMotion(l, km, "q", &r); got != MotionHandled {
		t.Errorf("unknown chord key returned %v, want MotionHandled (swallowed)", got)
	}
	// The chord is closed after any second key: a bare t is no motion.
	if got := HandleListMotion(l, km, "t", &r); got != MotionNone {
		t.Errorf("t after a swallowed chord returned %v, want MotionNone", got)
	}
	if l.Offset() != before {
		t.Errorf("offset moved to %d on an unknown chord key, want %d", l.Offset(), before)
	}
}

func TestHandleListMotionScrollAndPage(t *testing.T) {
	km := keymap.Default()
	const start = 100

	// Each case begins with the cursor on the bottom row of the window
	// (zb), so offset == start-rows+1.
	tests := []struct {
		name string
		key  string
		// deltas relative to the starting cursor and offset
		wantCursorDelta func(rows int) int
		wantOffsetDelta func(rows int) int
	}{
		{"ctrl+e scrolls the view down one, cursor stays", "ctrl+e",
			func(int) int { return 0 }, func(int) int { return 1 }},
		{"ctrl+y scrolls up one and pushes the cursor off the bottom", "ctrl+y",
			func(int) int { return -1 }, func(int) int { return -1 }},
		{"ctrl+f moves a full page down", "ctrl+f",
			func(rows int) int { return rows }, func(rows int) int { return rows }},
		{"ctrl+b moves a full page up", "ctrl+b",
			func(rows int) int { return -rows }, func(rows int) int { return -1 }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := motionList(t, 400)
			rows := l.Window().Height
			l.Select(start)
			l.CursorToBottom()

			startOffset := l.Offset()
			if startOffset != start-rows+1 {
				t.Fatalf("setup: offset = %d, want %d", startOffset, start-rows+1)
			}

			if got := HandleListMotion(l, km, tt.key, &vim.Resolver{}); got != MotionHandled {
				t.Fatalf("%s returned %v, want MotionHandled", tt.key, got)
			}
			if want := start + tt.wantCursorDelta(rows); l.Index() != want {
				t.Errorf("cursor = %d, want %d", l.Index(), want)
			}
			if want := startOffset + tt.wantOffsetDelta(rows); l.Offset() != want {
				t.Errorf("offset = %d, want %d", l.Offset(), want)
			}
		})
	}
}

// Keys that are not motions must fall through so the list's own
// bindings still see them.
func TestHandleListMotionIgnoresOtherKeys(t *testing.T) {
	km := keymap.Default()
	l := motionList(t, 200)

	for _, key := range []string{"j", "k", "enter", "/", "q", "G"} {
		if got := HandleListMotion(l, km, key, &vim.Resolver{}); got != MotionNone {
			t.Errorf("key %q returned %v, want MotionNone", key, got)
		}
	}
}

// The Standard keymap deliberately leaves the vim motions unbound
// because ctrl+f is its filter key. Nothing should be consumed there.
func TestStandardKeymapDoesNotStealCtrlF(t *testing.T) {
	km := keymap.Default()
	km.FullPageDown = keymap.Binding{}
	km.FullPageUp = keymap.Binding{}
	km.ScrollPrefix = keymap.Binding{}
	km.ScrollLineDown = keymap.Binding{}
	km.ScrollLineUp = keymap.Binding{}

	l := motionList(t, 200)
	for _, key := range []string{"ctrl+f", "ctrl+b", "ctrl+e", "ctrl+y", "z"} {
		if got := HandleListMotion(l, km, key, &vim.Resolver{}); got != MotionNone {
			t.Errorf("key %q returned %v with the motions unbound, want MotionNone", key, got)
		}
	}
}
