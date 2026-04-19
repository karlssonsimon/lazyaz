package app

import (
	"github.com/karlssonsimon/lazyaz/internal/blobapp"
	"github.com/karlssonsimon/lazyaz/internal/dashapp"
	"github.com/karlssonsimon/lazyaz/internal/jumplist"
	"github.com/karlssonsimon/lazyaz/internal/kvapp"
	"github.com/karlssonsimon/lazyaz/internal/sbapp"

	tea "charm.land/bubbletea/v2"
)

// jumpEntry is one position in the cross-tab navigation history.
// snap is the app-specific NavSnapshot the originating tab knows how
// to restore via ApplyNav. snap may be a tabHomeSnapshot for entries
// that mean "just be on this tab, no specific position" (e.g., a
// freshly opened tab where the user hasn't drilled in yet).
type jumpEntry struct {
	tabID int
	snap  jumplist.NavSnapshot
}

// tabHomeSnapshot represents "this tab, no specific position". Used
// when recording tab opens / switches so ctrl+o can return to a tab
// the user was on even before they drilled into anything. Restoration
// is purely the tab switch — applyNavToTab no-ops on this type.
type tabHomeSnapshot struct {
	kind TabKind
}

func (t tabHomeSnapshot) Description() string { return "tab: " + t.kind.String() }

// maxJumps is the cap on history length. Mirrors vim's `:set
// jumpoptions` limits — 100 is plenty for an interactive session.
const maxJumps = 100

// recordJump appends a snapshot to the jump list, truncating any
// forward history (vim semantics: making a new jump while walked back
// drops the to-be-overwritten future). Same snapshot back-to-back is
// deduped so re-selecting the same row doesn't bloat the list.
func (m *Model) recordJump(tabID int, snap jumplist.NavSnapshot) {
	if snap == nil {
		return
	}
	if m.jumpIdx >= 0 && m.jumpIdx < len(m.jumps) {
		cur := m.jumps[m.jumpIdx]
		if cur.tabID == tabID && cur.snap.Description() == snap.Description() {
			return
		}
		// Truncate forward history.
		if m.jumpIdx < len(m.jumps)-1 {
			m.jumps = m.jumps[:m.jumpIdx+1]
		}
	}
	m.jumps = append(m.jumps, jumpEntry{tabID: tabID, snap: snap})
	m.jumpIdx = len(m.jumps) - 1
	if len(m.jumps) > maxJumps {
		excess := len(m.jumps) - maxJumps
		m.jumps = m.jumps[excess:]
		m.jumpIdx -= excess
	}
}

// jumpBack walks one step backward through history. Mirrors vim's
// ctrl+o:
//
//   - If the user is "at the end" of the list (jumpIdx points at the
//     newest entry, or there are no entries yet), capture their
//     CURRENT position first so ctrl+i can return.
//   - Then decrement and restore.
//
// Skips entries whose tab has been closed (tabIndexByID returns -1).
func (m *Model) jumpBack() tea.Cmd {
	// Anchor current position so ctrl+i has somewhere to return to.
	// Only when at/past the end of the list — mid-list ctrl+o
	// shouldn't keep growing entries.
	if m.jumpIdx >= len(m.jumps)-1 && len(m.tabs) > 0 {
		if snap := m.tabSnapshotForJump(m.activeIdx); snap != nil {
			m.recordJump(m.tabs[m.activeIdx].ID, snap)
		}
	}
	for m.jumpIdx > 0 {
		m.jumpIdx--
		e := m.jumps[m.jumpIdx]
		if idx := m.tabIndexByID(e.tabID); idx >= 0 {
			return m.applyJumpEntry(idx, e)
		}
	}
	return nil
}

// jumpForward walks one step forward through history. Skips entries
// whose tab has been closed.
func (m *Model) jumpForward() tea.Cmd {
	for m.jumpIdx < len(m.jumps)-1 {
		m.jumpIdx++
		e := m.jumps[m.jumpIdx]
		if idx := m.tabIndexByID(e.tabID); idx >= 0 {
			return m.applyJumpEntry(idx, e)
		}
	}
	return nil
}

// cleanupJumpsForTab drops every jump entry pointing at the given tab
// ID and adjusts jumpIdx so it still points at a valid (or empty)
// position. Called when a tab is closed — keeps the list from filling
// with stale entries that ctrl+o would skip past in surprising ways.
func (m *Model) cleanupJumpsForTab(tabID int) {
	if len(m.jumps) == 0 {
		return
	}
	out := m.jumps[:0]
	removedBeforeIdx := 0
	for i, e := range m.jumps {
		if e.tabID == tabID {
			if i <= m.jumpIdx {
				removedBeforeIdx++
			}
			continue
		}
		out = append(out, e)
	}
	m.jumps = out
	m.jumpIdx -= removedBeforeIdx
	if m.jumpIdx >= len(m.jumps) {
		m.jumpIdx = len(m.jumps) - 1
	}
	if m.jumpIdx < -1 {
		m.jumpIdx = -1
	}
}

// applyJumpEntry switches to the entry's tab if needed and dispatches
// the snapshot to that tab's ApplyNav.
func (m *Model) applyJumpEntry(idx int, e jumpEntry) tea.Cmd {
	if idx != m.activeIdx {
		m.activeIdx = idx
	}
	cmd := m.applyNavToTab(idx, e.snap)
	resizeCmd := m.forwardToActive(tea.WindowSizeMsg{
		Width:  m.width,
		Height: m.childHeight(),
	})
	return tea.Batch(wrapCmd(m.tabs[idx].ID, cmd), resizeCmd)
}

// applyNavToTab type-switches to the right child and forwards the
// snapshot. Only sbapp / blobapp implement ApplyNav today; dashapp /
// kvapp ignore it (their CurrentNav returns nil so they shouldn't
// appear in the jump list anyway).
func (m *Model) applyNavToTab(idx int, snap jumplist.NavSnapshot) tea.Cmd {
	if idx < 0 || idx >= len(m.tabs) {
		return nil
	}
	switch child := m.tabs[idx].Model.(type) {
	case sbapp.Model:
		cmd := child.ApplyNav(snap)
		m.tabs[idx].Model = child
		return cmd
	case blobapp.Model:
		cmd := child.ApplyNav(snap)
		m.tabs[idx].Model = child
		return cmd
	case dashapp.Model, kvapp.Model:
		// Not jump-targets in this round.
	}
	return nil
}

// activeTabSnapshot returns the active tab's current navigable
// position, or nil if it isn't on one. Used at jump-recording sites
// that need to capture state before/after a navigation.
func (m *Model) activeTabSnapshot() jumplist.NavSnapshot {
	if len(m.tabs) == 0 {
		return nil
	}
	switch child := m.tabs[m.activeIdx].Model.(type) {
	case sbapp.Model:
		return child.CurrentNav()
	case blobapp.Model:
		return child.CurrentNav()
	}
	return nil
}

// tabSnapshotForJump returns the best snapshot to use when recording
// a jump entry pointing at the given tab. Prefers the in-tab position
// (sbapp/blobapp CurrentNav); falls back to a tabHomeSnapshot so the
// entry still carries a description and stays in the jump list. This
// is what makes "open a new tab → ctrl+o → previous tab" work even
// before the user drills into anything.
func (m *Model) tabSnapshotForJump(idx int) jumplist.NavSnapshot {
	if idx < 0 || idx >= len(m.tabs) {
		return nil
	}
	switch child := m.tabs[idx].Model.(type) {
	case sbapp.Model:
		if snap := child.CurrentNav(); snap != nil {
			return snap
		}
	case blobapp.Model:
		if snap := child.CurrentNav(); snap != nil {
			return snap
		}
	}
	return tabHomeSnapshot{kind: m.tabs[idx].Kind}
}

// recordTabChange captures snapshots on both sides of a tab change so
// ctrl+o can return to the previous tab and ctrl+i can come back to
// the new one. Called at every place active-tab changes (new tab,
// next/prev/jump tab, cross-tab open). Dedup in recordJump prevents
// adjacent duplicates from accumulating during rapid switches.
func (m *Model) recordTabChange(oldIdx, newIdx int) {
	if oldIdx >= 0 && oldIdx < len(m.tabs) {
		if snap := m.tabSnapshotForJump(oldIdx); snap != nil {
			m.recordJump(m.tabs[oldIdx].ID, snap)
		}
	}
	if newIdx >= 0 && newIdx < len(m.tabs) {
		if snap := m.tabSnapshotForJump(newIdx); snap != nil {
			m.recordJump(m.tabs[newIdx].ID, snap)
		}
	}
}
