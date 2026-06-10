package blobapp

import (
	"strings"

	"github.com/karlssonsimon/lazyaz/internal/jumplist"
	"github.com/karlssonsimon/lazyaz/internal/ui"

	tea "charm.land/bubbletea/v2"
)

// blobNavSnapshot captures the user's position in the Blob explorer:
// subscription + account + container + folder prefix + the selected row
// + the focused pane. Precise enough that ctrl+o restores the exact
// spot, not just the scope. Pane focus matters: backing out of
// container X to the containers list is a distinct navigational stop
// from being inside container X.
type blobNavSnapshot struct {
	subscriptionID string
	accountName    string
	containerName  string
	prefix         string
	itemKey        string // selected row in the focused pane ("" when none)
	focusedPane    int
}

func (s blobNavSnapshot) Description() string {
	parts := []string{"blob"}
	if s.accountName != "" {
		parts = append(parts, s.accountName)
	}
	if s.containerName != "" {
		parts = append(parts, s.containerName)
	}
	if s.prefix != "" {
		parts = append(parts, s.prefix)
	}
	if s.itemKey != "" {
		parts = append(parts, s.itemKey)
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
	snap := blobNavSnapshot{
		subscriptionID: m.CurrentSub.ID,
		focusedPane:    m.focus,
	}
	if m.hasAccount {
		snap.accountName = m.currentAccount.Name
	}
	if m.hasContainer {
		snap.containerName = m.containerName
		snap.prefix = m.prefix
	}
	switch m.focus {
	case accountsPane:
		if it, ok := m.accountsList.SelectedItem().(accountItem); ok {
			snap.itemKey = it.account.Name
		}
	case containersPane:
		if it, ok := m.containersList.SelectedItem().(containerItem); ok {
			snap.itemKey = it.container.Name
		}
	case blobsPane, previewPane:
		if it, ok := m.blobsList.SelectedItem().(blobItem); ok {
			snap.itemKey = it.blob.Name
		}
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
// change plus a cursor restore.
func (m *Model) ApplyNav(snap jumplist.NavSnapshot) tea.Cmd {
	s, ok := snap.(blobNavSnapshot)
	if !ok {
		return nil
	}
	m.applyingNav = true
	defer func() { m.applyingNav = false }()

	var cmds []tea.Cmd

	// Best-effort subscription restore when the snapshot was taken
	// under a different subscription. Unknown subscription (changed
	// tenant, list not loaded) → nothing sane to restore into.
	if s.subscriptionID != "" && (!m.HasSubscription || m.CurrentSub.ID != s.subscriptionID) {
		found := false
		for _, sub := range m.Subscriptions {
			if sub.ID == s.subscriptionID {
				updated, cmd := m.selectSubscription(sub)
				*m = updated
				if cmd != nil {
					cmds = append(cmds, cmd)
				}
				found = true
				break
			}
		}
		if !found {
			return nil
		}
	}

	if s.accountName == "" {
		if s.focusedPane >= accountsPane && s.focusedPane <= previewPane {
			m.transitionTo(s.focusedPane, false)
		}
		return batchNavCmds(cmds)
	}

	nav := PendingNav{
		AccountName:   s.accountName,
		ContainerName: s.containerName,
		Prefix:        s.prefix,
	}
	// Restore the exact blob row when the snapshot was taken on the
	// blobs pane (or its preview). selectBlobRow matches prefix+leaf.
	if (s.focusedPane == blobsPane || s.focusedPane == previewPane) && s.itemKey != "" {
		nav.BlobName = strings.TrimPrefix(s.itemKey, s.prefix)
	}
	if cmd := m.SetPendingNav(nav); cmd != nil {
		cmds = append(cmds, cmd)
	}
	// Restore the pane focus after the selection state is back.
	// SetPendingNav may have moved focus during the drill-in; force it
	// back to the snapshot's pane.
	if s.focusedPane >= accountsPane && s.focusedPane <= previewPane {
		m.transitionTo(s.focusedPane, false)
	}
	// Cursor restore for the list panes (cache-warm best effort — the
	// blob row is handled by PendingNav.BlobName through the async
	// load chain instead).
	switch s.focusedPane {
	case accountsPane:
		ui.SelectByKey(&m.accountsList, s.itemKey, accountItemKey)
	case containersPane:
		ui.SelectByKey(&m.containersList, s.itemKey, containerItemKey)
	}
	return batchNavCmds(cmds)
}

func (m Model) WithAppliedNav(snap jumplist.NavSnapshot) (tea.Model, tea.Cmd) {
	cmd := m.ApplyNav(snap)
	return m, cmd
}

// recordDeparture batches cmd with a jump record for the position the
// user is LEAVING (vim records origins, not destinations). Callers
// capture the snapshot via CurrentNav() BEFORE mutating scope. Records
// are suppressed during ApplyNav restoration and while a PendingNav is
// in flight (programmatic hops are not user jumps).
func recordDeparture(m Model, depart jumplist.NavSnapshot, cmd tea.Cmd) tea.Cmd {
	return jumplist.AppendRecord(m.applyingNav, m.pendingNav.hasTarget(), depart, cmd)
}
