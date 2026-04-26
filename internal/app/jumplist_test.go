package app

import (
	"testing"

	"github.com/karlssonsimon/lazyaz/internal/jumplist"
)

// fakeSnap is a minimal NavSnapshot implementation for testing the
// jump-list semantics in isolation from the real app models.
type fakeSnap struct{ desc string }

func (f fakeSnap) Description() string { return f.desc }

// newJumpModel returns a Model with just the fields the jumplist
// helpers touch — the rest of the struct stays zero-valued.
func newJumpModel() Model {
	return Model{jumpIdx: -1}
}

func TestRecordJumpAppends(t *testing.T) {
	m := newJumpModel()
	m.recordJump(1, fakeSnap{"a"})
	m.recordJump(1, fakeSnap{"b"})
	if len(m.jumps) != 2 {
		t.Fatalf("len(jumps) = %d, want 2", len(m.jumps))
	}
	if m.jumpIdx != 1 {
		t.Errorf("jumpIdx = %d, want 1", m.jumpIdx)
	}
}

func TestRecordJumpDedupsConsecutiveSame(t *testing.T) {
	m := newJumpModel()
	m.recordJump(1, fakeSnap{"a"})
	m.recordJump(1, fakeSnap{"a"})
	if len(m.jumps) != 1 {
		t.Errorf("len(jumps) = %d, want 1 (deduped)", len(m.jumps))
	}
}

func TestRecordJumpDedupsAcrossSameDescriptionDifferentTab(t *testing.T) {
	// Same description but different tab IDs is two different
	// destinations — should NOT dedup.
	m := newJumpModel()
	m.recordJump(1, fakeSnap{"a"})
	m.recordJump(2, fakeSnap{"a"})
	if len(m.jumps) != 2 {
		t.Errorf("len(jumps) = %d, want 2 (different tabs)", len(m.jumps))
	}
}

func TestRecordJumpTruncatesForwardHistory(t *testing.T) {
	m := newJumpModel()
	m.recordJump(1, fakeSnap{"a"})
	m.recordJump(1, fakeSnap{"b"})
	m.recordJump(1, fakeSnap{"c"})
	// Walk back twice (jumpIdx = 0).
	m.jumpIdx = 0
	// New jump should drop b and c.
	m.recordJump(1, fakeSnap{"d"})
	if len(m.jumps) != 2 {
		t.Errorf("len(jumps) = %d, want 2 (forward truncated)", len(m.jumps))
	}
	if m.jumps[1].snap.Description() != "d" {
		t.Errorf("jumps[1] = %q, want d", m.jumps[1].snap.Description())
	}
	if m.jumpIdx != 1 {
		t.Errorf("jumpIdx = %d, want 1", m.jumpIdx)
	}
}

func TestRecordJumpIgnoresNil(t *testing.T) {
	m := newJumpModel()
	m.recordJump(1, nil)
	if len(m.jumps) != 0 {
		t.Errorf("nil snap added entry: %+v", m.jumps)
	}
}

func TestRecordJumpCapsAtMax(t *testing.T) {
	m := newJumpModel()
	for i := 0; i < maxJumps+10; i++ {
		m.recordJump(1, fakeSnap{desc: string(rune('a'+i%26)) + string(rune('0'+i/26))})
	}
	if len(m.jumps) != maxJumps {
		t.Errorf("len(jumps) = %d, want %d", len(m.jumps), maxJumps)
	}
	if m.jumpIdx != maxJumps-1 {
		t.Errorf("jumpIdx = %d, want %d", m.jumpIdx, maxJumps-1)
	}
}

func TestRecordJumpAfterCapKeepsLatest(t *testing.T) {
	m := newJumpModel()
	for i := 0; i < maxJumps+5; i++ {
		// Distinct descriptions so dedup doesn't interfere.
		m.recordJump(1, fakeSnap{desc: string(rune('a'+i%26)) + string(rune('0'+i/26))})
	}
	// First entry should be the 6th original (5 evicted).
	if m.jumps[0].snap.Description() == "a0" {
		t.Errorf("expected oldest entries evicted, jumps[0] = %s", m.jumps[0].snap.Description())
	}
}

// jumpBack/jumpForward exercise tab-resolution which requires a real
// tabs slice; the helpers below don't need to round-trip through
// applyNavToTab (no children registered), so we test the index walk
// only by inserting a stub-friendly setup.
func TestJumpBackAdvancesIndex(t *testing.T) {
	m := newJumpModel()
	// Pretend the entries point at tab IDs that don't exist — jumpBack
	// will walk all the way to the beginning skipping each one.
	m.jumps = []jumpEntry{
		{tabID: 99, snap: fakeSnap{"a"}},
		{tabID: 99, snap: fakeSnap{"b"}},
	}
	m.jumpIdx = 1
	cmd := m.jumpBack()
	if cmd != nil {
		t.Errorf("expected nil cmd when no tabs match, got %v", cmd)
	}
	// All entries skipped → jumpIdx ends at 0 (last position visited).
	if m.jumpIdx != 0 {
		t.Errorf("jumpIdx = %d, want 0", m.jumpIdx)
	}
}

func TestJumpForwardDoesNothingPastEnd(t *testing.T) {
	m := newJumpModel()
	m.jumps = []jumpEntry{{tabID: 99, snap: fakeSnap{"a"}}}
	m.jumpIdx = 0
	if cmd := m.jumpForward(); cmd != nil {
		t.Errorf("expected nil cmd at end of list, got %v", cmd)
	}
	if m.jumpIdx != 0 {
		t.Errorf("jumpIdx = %d, want 0 (no movement)", m.jumpIdx)
	}
}

func TestJumpBackDoesNothingAtBeginning(t *testing.T) {
	m := newJumpModel()
	if cmd := m.jumpBack(); cmd != nil {
		t.Errorf("expected nil cmd on empty list, got %v", cmd)
	}
	if m.jumpIdx != -1 {
		t.Errorf("jumpIdx = %d, want -1", m.jumpIdx)
	}
}

// Verify the type-switch in applyNavToTab handles the dashapp/kvapp
// "no-op" path without panicking.
func TestApplyNavToTabUnknownTypeIsNoop(t *testing.T) {
	m := newJumpModel()
	// No tabs at all → no panic, returns nil.
	cmd := m.applyNavToTab(0, fakeSnap{"x"})
	if cmd != nil {
		t.Errorf("expected nil for missing tab, got %v", cmd)
	}
}

// The RecordJumpMsg type lives in the jumplist package — confirm a
// caller can construct one without importing app internals.
func TestRecordJumpMsgIsExported(t *testing.T) {
	_ = jumplist.RecordJumpMsg{Snap: fakeSnap{"x"}}
}

func TestCleanupJumpsForTabRemovesEntries(t *testing.T) {
	m := newJumpModel()
	m.recordJump(1, fakeSnap{"a"})
	m.recordJump(2, fakeSnap{"b"})
	m.recordJump(1, fakeSnap{"c"})
	// jumps=[(1,a), (2,b), (1,c)], jumpIdx=2
	m.cleanupJumpsForTab(1)
	if len(m.jumps) != 1 {
		t.Fatalf("len(jumps) = %d, want 1", len(m.jumps))
	}
	if m.jumps[0].snap.Description() != "b" {
		t.Errorf("survivor = %s, want b", m.jumps[0].snap.Description())
	}
}

func TestCleanupJumpsForTabAdjustsJumpIdx(t *testing.T) {
	m := newJumpModel()
	m.recordJump(1, fakeSnap{"a"}) // idx 0
	m.recordJump(2, fakeSnap{"b"}) // idx 1
	m.recordJump(1, fakeSnap{"c"}) // idx 2
	m.recordJump(2, fakeSnap{"d"}) // idx 3
	m.jumpIdx = 2                  // simulate user walked back
	// Removing tab 1 should drop entries at idx 0 and 2. jumpIdx
	// was at idx 2 (one of the removed) — should clamp to len-1 of
	// remaining (which is 1, with surviving entries [b, d]).
	m.cleanupJumpsForTab(1)
	if len(m.jumps) != 2 {
		t.Fatalf("len(jumps) = %d, want 2", len(m.jumps))
	}
	if m.jumpIdx >= len(m.jumps) || m.jumpIdx < 0 {
		t.Errorf("jumpIdx = %d out of range for len %d", m.jumpIdx, len(m.jumps))
	}
}

func TestCleanupJumpsForTabEmpty(t *testing.T) {
	m := newJumpModel()
	m.cleanupJumpsForTab(99) // no panic on empty list
	if len(m.jumps) != 0 || m.jumpIdx != -1 {
		t.Errorf("unexpected mutation: jumps=%v jumpIdx=%d", m.jumps, m.jumpIdx)
	}
}
