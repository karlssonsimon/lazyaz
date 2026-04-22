package kvapp

import (
	"strings"

	"github.com/karlssonsimon/lazyaz/internal/jumplist"

	tea "charm.land/bubbletea/v2"
)

// kvNavSnapshot captures the user's position in the Key Vault explorer:
// vault + (optionally) secret + focused pane. Version selection isn't
// part of the snapshot — it's just which row is highlighted in the
// versions list, and the scope's history entry restores that naturally.
type kvNavSnapshot struct {
	vaultName   string
	secretName  string
	focusedPane int
}

func (s kvNavSnapshot) Description() string {
	parts := []string{"kv"}
	if s.vaultName != "" {
		parts = append(parts, s.vaultName)
	}
	if s.secretName != "" {
		parts = append(parts, s.secretName)
	}
	parts = append(parts, paneLabel(s.focusedPane))
	return strings.Join(parts, " / ")
}

func paneLabel(pane int) string {
	switch pane {
	case vaultsPane:
		return "vaults"
	case secretsPane:
		return "secrets"
	case versionsPane:
		return "versions"
	default:
		return "?"
	}
}

// CurrentNav captures the active position. Returns nil only when no
// subscription is set — the vaults-list view (focus=vaultsPane,
// hasVault=false) is a meaningful jump target in its own right, so
// ctrl+o can walk back to it after the user drills into a vault.
func (m Model) CurrentNav() jumplist.NavSnapshot {
	if !m.HasSubscription {
		return nil
	}
	snap := kvNavSnapshot{focusedPane: m.focus}
	if m.hasVault {
		snap.vaultName = m.currentVault.Name
	}
	if m.hasSecret {
		snap.secretName = m.currentSecret.Name
	}
	return snap
}

// ApplyNav restores a captured position. applyingNav suppresses
// RecordJumpMsg emission from drill-in helpers so restoration doesn't
// re-record entries we're traversing.
//
// A snapshot with empty vaultName represents the pre-drill state (user
// was on the vaults list itself); restoring it is just a focus change.
func (m *Model) ApplyNav(snap jumplist.NavSnapshot) tea.Cmd {
	s, ok := snap.(kvNavSnapshot)
	if !ok {
		return nil
	}
	m.applyingNav = true
	defer func() { m.applyingNav = false }()
	if s.vaultName == "" {
		if s.focusedPane >= vaultsPane && s.focusedPane <= versionsPane {
			m.transitionTo(s.focusedPane)
		}
		return nil
	}
	cmd := m.SetPendingNav(PendingNav{
		VaultName:  s.vaultName,
		SecretName: s.secretName,
	})
	if s.focusedPane >= vaultsPane && s.focusedPane <= versionsPane {
		m.transitionTo(s.focusedPane)
	}
	return cmd
}

// NavSnapshotFromPending builds a snapshot from a PendingNav target.
// Mirrors the blobapp/sbapp helper so external openers can record a
// destination snapshot even before the eager fast-forward applies.
// (kvapp has no cross-tab open message today, but exporting this keeps
// the pattern uniform if one is added later.)
func NavSnapshotFromPending(p PendingNav) jumplist.NavSnapshot {
	if p.VaultName == "" {
		return nil
	}
	pane := secretsPane
	if p.SecretName != "" {
		pane = versionsPane
	}
	return kvNavSnapshot{
		vaultName:   p.VaultName,
		secretName:  p.SecretName,
		focusedPane: pane,
	}
}

func recordJumpForCurrent(m Model) tea.Cmd {
	if m.applyingNav {
		return nil
	}
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
