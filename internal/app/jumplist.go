package app

import (
	"github.com/karlssonsimon/lazyaz/internal/jumplist"

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
// abandons the to-be-overwritten future). Truncation happens BEFORE the
// dedup check — jumping again from the position ctrl+o landed on must
// still drop the forward entries even though the snapshot itself is a
// duplicate of the current one. Same snapshot back-to-back is deduped
// so repeated jumps from one spot don't bloat the list.
func (m *Model) recordJump(tabID int, snap jumplist.NavSnapshot) {
	if snap == nil {
		return
	}
	if m.jumpIdx >= 0 && m.jumpIdx < len(m.jumps)-1 {
		m.jumps = m.jumps[:m.jumpIdx+1]
	}
	if m.jumpIdx >= 0 && m.jumpIdx < len(m.jumps) {
		cur := m.jumps[m.jumpIdx]
		if cur.tabID == tabID && cur.snap.Description() == snap.Description() {
			return
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
// Skips entries whose tab has been closed (tabIndexByID returns -1)
// and entries that sit on the active tab's current drill path (see
// entryIsRedundant).
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
		if idx := m.tabIndexByID(e.tabID); idx >= 0 && !m.entryIsRedundant(e) {
			return m.applyJumpEntry(idx, e)
		}
	}
	return nil
}

// jumpForward walks one step forward through history. Skips entries
// whose tab has been closed and entries on the current drill path.
func (m *Model) jumpForward() tea.Cmd {
	for m.jumpIdx < len(m.jumps)-1 {
		m.jumpIdx++
		e := m.jumps[m.jumpIdx]
		if idx := m.tabIndexByID(e.tabID); idx >= 0 && !m.entryIsRedundant(e) {
			return m.applyJumpEntry(idx, e)
		}
	}
	return nil
}

// entryIsRedundant reports whether restoring e would be an
// h-equivalent hop: the entry points at the ACTIVE tab and its scope
// is a strict ancestor of the position the user is already at (e.g.
// "the containers list of the account I'm inside"). Walking stops
// there teach the user that ctrl+o is a worse h — skip them. Entries
// for other tabs always produce a visible tab switch and never skip;
// equal or sibling scopes always land.
func (m *Model) entryIsRedundant(e jumpEntry) bool {
	if len(m.tabs) == 0 || m.tabs[m.activeIdx].ID != e.tabID {
		return false
	}
	anc, ok := e.snap.(jumplist.ScopeAncestor)
	if !ok {
		return false
	}
	live := m.tabSnapshotForJump(m.activeIdx)
	if live == nil {
		return false
	}
	return anc.StrictAncestorOf(live)
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

// applyNavToTab forwards the snapshot to tabs that own navigation state.
func (m *Model) applyNavToTab(idx int, snap jumplist.NavSnapshot) tea.Cmd {
	if idx < 0 || idx >= len(m.tabs) {
		return nil
	}
	if child, ok := m.tabs[idx].Model.(navigationTab); ok {
		updated, cmd := child.WithAppliedNav(snap)
		m.tabs[idx].Model = updated
		return cmd
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
	if child, ok := m.tabs[idx].Model.(navigationTab); ok {
		if snap := child.CurrentNav(); snap != nil {
			return snap
		}
	}
	return tabHomeSnapshot{kind: m.tabs[idx].Kind}
}

// recordTabDeparture captures the position the user is LEAVING when the
// active tab changes (vim records origins, not destinations — the new
// tab's position gets recorded when the user jumps away from it, and
// ctrl+i returns there via the anchor jumpBack writes). Called at every
// place active-tab changes (new tab, jump tab, cross-tab open).
func (m *Model) recordTabDeparture(oldIdx int) {
	if oldIdx >= 0 && oldIdx < len(m.tabs) {
		if snap := m.tabSnapshotForJump(oldIdx); snap != nil {
			m.recordJump(m.tabs[oldIdx].ID, snap)
		}
	}
}
