package blobapp

import (
	"fmt"
	"strings"
	"testing"

	"github.com/karlssonsimon/lazyaz/internal/azure/blob"

	tea "charm.land/bubbletea/v2"
)

// scrollModel builds a blobs pane holding n blobs, rendered once so the
// list has picked up its real window height.
func scrollModel(t *testing.T, n int) Model {
	t.Helper()

	m := NewModel(nil, testConfig, nil)
	m.SubOverlay.Close()
	m.Width, m.Height = 120, 30
	m.hasAccount = true
	m.currentAccount.Name = "acct"
	m.hasContainer = true
	m.containerName = "cont"
	m.focus = blobsPane

	m.blobs = make([]blob.BlobEntry, n)
	for i := range m.blobs {
		m.blobs[i] = blob.BlobEntry{Name: fmt.Sprintf("blob-%04d", i)}
	}
	m.refreshItems()
	// These cases are about the paging bug at the window edge, so they
	// run with no scrolloff. Context rows get their own test below.
	m.blobsList.SetScrolloff(0)
	m.resize()
	_ = m.View() // SetSize runs during render; the window height comes from it

	if rows := m.blobsList.Window().Height; rows < 3 {
		t.Fatalf("test needs a window of at least 3 rows, got %d", rows)
	}
	return m
}

func pressKey(m Model, key string) Model {
	updated, _ := m.Update(tea.KeyPressMsg{Code: rune(key[0]), Text: key})
	return updated.(Model)
}

// The reported bug. With the cursor on the last visible row, pressing
// down used to flip to the next page and drop the cursor at the top.
// It must scroll a single row and leave the cursor on the bottom row.
func TestCursorDownAtWindowBottomScrollsOneRow(t *testing.T) {
	m := scrollModel(t, 500)
	rows := m.blobsList.Window().Height

	m.blobsList.Select(rows - 1)
	if got := m.blobsList.Offset(); got != 0 {
		t.Fatalf("setup: offset = %d, want 0", got)
	}

	m = pressKey(m, "j")

	if got := m.blobsList.Index(); got != rows {
		t.Errorf("Index() = %d, want %d", got, rows)
	}
	if got := m.blobsList.Offset(); got != 1 {
		t.Errorf("Offset() = %d, want 1 — the window should scroll one row, not page", got)
	}
	if got := m.blobsList.Index() - m.blobsList.Offset(); got != rows-1 {
		t.Errorf("cursor is on window row %d, want it to stay on the bottom row %d", got, rows-1)
	}
}

// The same at the top edge, going back up.
func TestCursorUpAtWindowTopScrollsOneRow(t *testing.T) {
	m := scrollModel(t, 500)
	rows := m.blobsList.Window().Height

	m.blobsList.Select(rows * 3)
	m.blobsList.CursorToTop()
	top := m.blobsList.Offset()
	if top != rows*3 {
		t.Fatalf("setup: offset = %d, want %d", top, rows*3)
	}

	m = pressKey(m, "k")

	if got := m.blobsList.Index(); got != rows*3-1 {
		t.Errorf("Index() = %d, want %d", got, rows*3-1)
	}
	if got := m.blobsList.Offset(); got != top-1 {
		t.Errorf("Offset() = %d, want %d — the window should scroll one row", got, top-1)
	}
}

// Moving within the window must not scroll at all.
func TestCursorInsideWindowDoesNotScroll(t *testing.T) {
	m := scrollModel(t, 500)

	m.blobsList.Select(0)
	for i := 0; i < m.blobsList.Window().Height-1; i++ {
		m = pressKey(m, "j")
		if got := m.blobsList.Offset(); got != 0 {
			t.Fatalf("after %d moves offset = %d, want 0 while the cursor is still inside the window", i+1, got)
		}
	}
}

// The rendered pane must show the scrolled window, not just the model
// state — this is what catches a render path that ignores the offset.
func TestRenderedWindowFollowsTheOffset(t *testing.T) {
	m := scrollModel(t, 500)
	rows := m.blobsList.Window().Height

	m.blobsList.Select(rows - 1)
	m = pressKey(m, "j")

	view := m.View().Content
	if strings.Contains(view, "blob-0000") {
		t.Error("first blob is still rendered after the window scrolled past it")
	}
	if !strings.Contains(view, fmt.Sprintf("blob-%04d", rows)) {
		t.Errorf("newly scrolled-in blob-%04d is missing from the view", rows)
	}
}

// A list shorter than the window never scrolls, and the cursor still
// reaches the last item.
func TestShortListNeverScrolls(t *testing.T) {
	m := scrollModel(t, 3)

	for i := 0; i < 10; i++ {
		m = pressKey(m, "j")
	}
	if got := m.blobsList.Offset(); got != 0 {
		t.Errorf("Offset() = %d, want 0 for a list shorter than the window", got)
	}
	if got := m.blobsList.Index(); got != 2 {
		t.Errorf("Index() = %d, want 2 (last item)", got)
	}
}

// With scrolloff set, the window starts moving before the cursor reaches
// the edge, keeping that many rows of context below it.
func TestScrolloffKeepsContextRows(t *testing.T) {
	const scrolloff = 3

	m := scrollModel(t, 500)
	m.blobsList.SetScrolloff(scrolloff)
	rows := m.blobsList.Window().Height

	m.blobsList.Select(0)
	for i := 0; i < rows; i++ {
		m = pressKey(m, "j")
	}

	cursorRow := m.blobsList.Index() - m.blobsList.Offset()
	if cursorRow != rows-1-scrolloff {
		t.Errorf("cursor sits on window row %d, want %d — %d context rows should remain below it",
			cursorRow, rows-1-scrolloff, scrolloff)
	}
}

// Scrolloff must not stop the cursor reaching the very last item.
func TestScrolloffStillReachesTheEnd(t *testing.T) {
	m := scrollModel(t, 60)
	m.blobsList.SetScrolloff(3)

	for i := 0; i < 200; i++ {
		m = pressKey(m, "j")
	}

	if got := m.blobsList.Index(); got != 59 {
		t.Errorf("Index() = %d, want 59 — scrolloff should not pad past the end of the list", got)
	}
}
