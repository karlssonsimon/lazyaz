package blobapp

import (
	"strings"

	"github.com/karlssonsimon/lazyaz/internal/jumplist"

	tea "charm.land/bubbletea/v2"
)

// blobNavSnapshot captures the user's position in the Blob explorer:
// account + (optionally) container. ApplyNav restores via the existing
// PendingNav fast-forward path so cache-warmed jumps are instant.
type blobNavSnapshot struct {
	accountName   string
	containerName string
}

func (s blobNavSnapshot) Description() string {
	parts := []string{"blob", s.accountName}
	if s.containerName != "" {
		parts = append(parts, s.containerName)
	}
	return strings.Join(parts, " / ")
}

func (m Model) CurrentNav() jumplist.NavSnapshot {
	if !m.HasSubscription || !m.hasAccount {
		return nil
	}
	return blobNavSnapshot{
		accountName:   m.currentAccount.Name,
		containerName: m.containerName,
	}
}

// ApplyNav restores a captured position. applyingNav suppresses
// RecordJumpMsg from the drill-in helpers we call so restoration
// doesn't re-record the entries we're traversing — which would
// truncate forward history and trap the user in an oscillation.
func (m *Model) ApplyNav(snap jumplist.NavSnapshot) tea.Cmd {
	s, ok := snap.(blobNavSnapshot)
	if !ok {
		return nil
	}
	m.applyingNav = true
	defer func() { m.applyingNav = false }()
	return m.SetPendingNav(PendingNav{
		AccountName:   s.accountName,
		ContainerName: s.containerName,
	})
}

// NavSnapshotFromPending mirrors sbapp's helper — lets the parent
// record a destination snapshot when opening a Blob tab with a
// pending navigation, even before the eager fast-forward applies.
func NavSnapshotFromPending(p PendingNav) jumplist.NavSnapshot {
	if p.AccountName == "" {
		return nil
	}
	return blobNavSnapshot{
		accountName:   p.AccountName,
		containerName: p.ContainerName,
	}
}

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
