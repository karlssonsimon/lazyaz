package blobapp

import (
	"strings"

	"github.com/karlssonsimon/lazyaz/internal/jumplist"

	tea "charm.land/bubbletea/v2"
)

// blobNavSnapshot captures the user's position in the Blob explorer:
// account + (optionally) container + the pane the user is focused on.
// Pane focus matters for ctrl+o: backing out of container X to the
// containers/accounts list is a distinct navigational stop from being
// inside container X, even when the underlying selection is unchanged.
type blobNavSnapshot struct {
	accountName   string
	containerName string
	focusedPane   int
}

func (s blobNavSnapshot) Description() string {
	parts := []string{"blob"}
	if s.accountName != "" {
		parts = append(parts, s.accountName)
	}
	if s.containerName != "" {
		parts = append(parts, s.containerName)
	}
	parts = append(parts, paneLabel(s.focusedPane))
	return strings.Join(parts, " / ")
}

func paneLabel(pane int) string {
	switch pane {
	case accountsPane:
		return "accounts"
	case containersPane:
		return "containers"
	case blobsPane:
		return "blobs"
	case previewPane:
		return "preview"
	default:
		return "?"
	}
}

// CurrentNav captures the active position. Returns nil only when no
// subscription is set — the accounts-list view (focus=accountsPane,
// hasAccount=false) is a meaningful jump target so ctrl+o can walk
// back to it after the user drills into an account.
func (m Model) CurrentNav() jumplist.NavSnapshot {
	if !m.HasSubscription {
		return nil
	}
	snap := blobNavSnapshot{focusedPane: m.focus}
	if m.hasAccount {
		snap.accountName = m.currentAccount.Name
	}
	if m.hasContainer {
		snap.containerName = m.containerName
	}
	return snap
}

// ApplyNav restores a captured position. applyingNav suppresses
// RecordJumpMsg from the drill-in helpers we call so restoration
// doesn't re-record the entries we're traversing — which would
// truncate forward history and trap the user in an oscillation.
//
// A snapshot with empty accountName represents the pre-drill state
// (user was on the accounts list itself); restoring is just a focus
// change.
func (m *Model) ApplyNav(snap jumplist.NavSnapshot) tea.Cmd {
	s, ok := snap.(blobNavSnapshot)
	if !ok {
		return nil
	}
	m.applyingNav = true
	defer func() { m.applyingNav = false }()
	if s.accountName == "" {
		if s.focusedPane >= accountsPane && s.focusedPane <= previewPane {
			m.transitionTo(s.focusedPane, false)
		}
		return nil
	}
	cmd := m.SetPendingNav(PendingNav{
		AccountName:   s.accountName,
		ContainerName: s.containerName,
	})
	// Restore the pane focus after the selection state is back.
	// SetPendingNav may have moved focus during the drill-in; force it
	// back to the snapshot's pane.
	if s.focusedPane >= accountsPane && s.focusedPane <= previewPane {
		m.transitionTo(s.focusedPane, false)
	}
	return cmd
}

// NavSnapshotFromPending mirrors sbapp's helper — lets the parent
// record a destination snapshot when opening a Blob tab with a
// pending navigation, even before the eager fast-forward applies.
// Chooses the deepest pane implied by the nav (blobs if a container
// is specified, otherwise containers).
func NavSnapshotFromPending(p PendingNav) jumplist.NavSnapshot {
	if p.AccountName == "" {
		return nil
	}
	pane := containersPane
	if p.ContainerName != "" {
		pane = blobsPane
	}
	return blobNavSnapshot{
		accountName:   p.AccountName,
		containerName: p.ContainerName,
		focusedPane:   pane,
	}
}

func recordJumpForCurrent(m Model) tea.Cmd {
	if m.applyingNav {
		return nil
	}
	// Suppress jump records while a PendingNav is still in flight. The
	// parent (app.openBlobTabWithNav) records the final destination
	// directly; any RecordJumpMsgs that eagerNavigate / advancePendingNav
	// would emit for intermediate hops are noise the user never actually
	// traverses, and pollute ctrl+o history with phantom stops.
	if m.pendingNav.hasTarget() {
		return nil
	}
	snap := m.CurrentNav()
	if snap == nil {
		return nil
	}
	return func() tea.Msg { return jumplist.RecordJumpMsg{Snap: snap} }
}

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
