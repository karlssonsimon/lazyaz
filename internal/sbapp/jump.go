package sbapp

import (
	"strings"

	"github.com/karlssonsimon/lazyaz/internal/azure/servicebus"
	"github.com/karlssonsimon/lazyaz/internal/jumplist"

	tea "charm.land/bubbletea/v2"
)

// sbNavSnapshot captures the user's position in the Service Bus
// explorer: namespace + (optionally) entity + (for topics) subscription
// + DLQ pane flag. ApplyNav restores this via the existing PendingNav
// fast-forward path so cache-warmed jumps are instant.
type sbNavSnapshot struct {
	namespace  servicebus.Namespace
	entityName string
	subName    string
	deadLetter bool
}

func (s sbNavSnapshot) Description() string {
	parts := []string{"sb", s.namespace.Name}
	if s.entityName != "" {
		parts = append(parts, s.entityName)
	}
	if s.subName != "" {
		parts = append(parts, s.subName)
	}
	if s.deadLetter {
		parts = append(parts, "DLQ")
	}
	return strings.Join(parts, " / ")
}

// CurrentNav captures the active position. Returns nil before a
// namespace is selected — the explorer at the namespace-list view
// isn't a meaningful jump target.
func (m Model) CurrentNav() jumplist.NavSnapshot {
	if !m.HasSubscription || !m.hasNamespace {
		return nil
	}
	return sbNavSnapshot{
		namespace:  m.currentNS,
		entityName: m.currentEntity.Name,
		subName:    m.currentSubName,
		deadLetter: m.deadLetter,
	}
}

// ApplyNav restores a captured position. Type-asserts the opaque
// snapshot back to sbNavSnapshot; foreign snapshots are silently
// ignored (the parent only routes us snapshots we created, but the
// extra check costs nothing).
//
// The applyingNav flag suppresses RecordJumpMsg emission from the
// drill-in helpers we're about to call — restoring should not append
// the destination as a fresh jump entry (that would truncate forward
// history and trap the user in an oscillation between two adjacent
// entries).
func (m *Model) ApplyNav(snap jumplist.NavSnapshot) tea.Cmd {
	s, ok := snap.(sbNavSnapshot)
	if !ok {
		return nil
	}
	m.applyingNav = true
	defer func() { m.applyingNav = false }()
	return m.SetPendingNav(PendingNav{
		Namespace:  s.namespace,
		EntityName: s.entityName,
		SubName:    s.subName,
		DeadLetter: s.deadLetter,
	})
}

// NavSnapshotFromPending builds a snapshot directly from a PendingNav
// target. The parent app uses this when creating a tab with pending
// navigation: the destination snapshot can be recorded immediately
// even before the eager fast-forward runs (cache might miss, in which
// case CurrentNav would return nil too early to capture).
func NavSnapshotFromPending(p PendingNav) jumplist.NavSnapshot {
	if p.Namespace.Name == "" {
		return nil
	}
	return sbNavSnapshot{
		namespace:  p.Namespace,
		entityName: p.EntityName,
		subName:    p.SubName,
		deadLetter: p.DeadLetter,
	}
}

// recordJumpForCurrent returns a Cmd that emits a RecordJumpMsg with
// the model's CurrentNav snapshot. Called from the drill-in helpers
// (selectNamespace / selectQueue / selectTopic / selectSubscriptionSub)
// after they mutate state, so the destination position gets recorded.
// Returns nil during programmatic restoration (m.applyingNav) so
// jump-list walks don't re-record the entries they're traversing.
func recordJumpForCurrent(m Model) tea.Cmd {
	if m.applyingNav {
		return nil
	}
	snap := m.CurrentNav()
	if snap == nil {
		return nil
	}
	return func() tea.Msg { return jumplist.RecordJumpMsg{Snap: snap} }
}

// appendJumpRecord batches an existing cmd with a fresh jump record
// for m's current navigable position. Used at the end of each drill-in
// helper so callers don't have to remember to do it.
func appendJumpRecord(m Model, cmd tea.Cmd) tea.Cmd {
	rec := recordJumpForCurrent(m)
	if rec == nil {
		return cmd
	}
	if cmd == nil {
		return rec
	}
	return tea.Batch(cmd, rec)
}
