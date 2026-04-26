package sbapp

import (
	"strings"

	"github.com/karlssonsimon/lazyaz/internal/azure/servicebus"
	"github.com/karlssonsimon/lazyaz/internal/jumplist"

	tea "charm.land/bubbletea/v2"
)

// sbNavSnapshot captures the user's position in the Service Bus
// explorer: namespace + (optionally) entity + (for topics) subscription
// + DLQ pane flag + which Miller pane is focused. ApplyNav restores this
// via the existing PendingNav fast-forward path so cache-warmed jumps
// are instant.
type sbNavSnapshot struct {
	namespace   servicebus.Namespace
	entityName  string
	subName     string
	deadLetter  bool
	focusedPane int
}

func (s sbNavSnapshot) Description() string {
	parts := []string{"sb"}
	if s.namespace.Name != "" {
		parts = append(parts, s.namespace.Name)
	}
	if s.entityName != "" {
		parts = append(parts, s.entityName)
	}
	if s.subName != "" {
		parts = append(parts, s.subName)
	}
	if s.deadLetter {
		parts = append(parts, "DLQ")
	}
	parts = append(parts, paneLabel(s.focusedPane))
	return strings.Join(parts, " / ")
}

func paneLabel(pane int) string {
	switch pane {
	case namespacesPane:
		return "namespaces"
	case entitiesPane:
		return "entities"
	case subscriptionsPane:
		return "subscriptions"
	case queueTypePane:
		return "queuetype"
	case messagesPane:
		return "messages"
	case messagePreviewPane:
		return "preview"
	default:
		return "?"
	}
}

// CurrentNav captures the active position. Returns nil only when no
// subscription is set — the namespaces-list view (focus=namespacesPane,
// hasNamespace=false) is a meaningful jump target so ctrl+o can walk
// back to it after the user drills into a namespace.
func (m Model) CurrentNav() jumplist.NavSnapshot {
	if !m.HasSubscription {
		return nil
	}
	snap := sbNavSnapshot{focusedPane: m.focus}
	if m.hasNamespace {
		snap.namespace = m.currentNS
		snap.entityName = m.currentEntity.Name
		snap.subName = m.currentSubName
		snap.deadLetter = m.deadLetter
	}
	return snap
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
//
// A snapshot with empty namespace.Name represents the pre-drill state
// (user was on the namespaces list itself); restoring is just a focus
// change.
func (m *Model) ApplyNav(snap jumplist.NavSnapshot) tea.Cmd {
	s, ok := snap.(sbNavSnapshot)
	if !ok {
		return nil
	}
	m.applyingNav = true
	defer func() { m.applyingNav = false }()
	if s.namespace.Name == "" {
		if s.focusedPane >= namespacesPane && s.focusedPane <= messagePreviewPane {
			m.transitionTo(s.focusedPane)
		}
		return nil
	}
	cmd := m.SetPendingNav(PendingNav{
		Namespace:  s.namespace,
		EntityName: s.entityName,
		SubName:    s.subName,
		DeadLetter: s.deadLetter,
	})
	// Restore pane focus after the drill-in lands.
	if s.focusedPane >= namespacesPane && s.focusedPane <= messagePreviewPane {
		m.transitionTo(s.focusedPane)
	}
	return cmd
}

func (m Model) WithAppliedNav(snap jumplist.NavSnapshot) (tea.Model, tea.Cmd) {
	cmd := m.ApplyNav(snap)
	return m, cmd
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
	// Pick the deepest pane the nav implies so the snapshot records
	// the same focus the drill-in will land on.
	pane := entitiesPane
	switch {
	case p.SubName != "" && p.DeadLetter, p.SubName != "" && !p.DeadLetter:
		pane = messagesPane
	case p.EntityName != "":
		pane = entitiesPane
	}
	return sbNavSnapshot{
		namespace:   p.Namespace,
		entityName:  p.EntityName,
		subName:     p.SubName,
		deadLetter:  p.DeadLetter,
		focusedPane: pane,
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
	// Suppress records while a PendingNav is in flight — the parent
	// records the destination directly in openSBTabWithNav, so the
	// intermediate hops emitted by selectNamespace / selectEntity would
	// just pollute ctrl+o history with phantom stops.
	if m.pendingNav.hasTarget() {
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
