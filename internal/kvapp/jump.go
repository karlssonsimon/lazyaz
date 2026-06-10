package kvapp

import (
	"strings"

	"github.com/karlssonsimon/lazyaz/internal/jumplist"
	"github.com/karlssonsimon/lazyaz/internal/ui"

	tea "charm.land/bubbletea/v2"
)

// kvNavSnapshot captures the user's position in the Key Vault explorer:
// subscription + vault + kind + the secret/cert/key whose versions are
// open + the selected row + focused pane. Precise enough that ctrl+o
// restores the exact spot, not just the scope.
type kvNavSnapshot struct {
	subscriptionID string
	vaultName      string
	kind           kvKind
	itemName       string // secret/cert/key whose versions column is bound
	itemKey        string // selected row in the focused pane ("" when none)
	focusedPane    int
}

func (s kvNavSnapshot) Description() string {
	parts := []string{"kv"}
	if s.vaultName != "" {
		parts = append(parts, s.vaultName, s.kind.String())
	}
	if s.itemName != "" {
		parts = append(parts, s.itemName)
	}
	if s.itemKey != "" && s.itemKey != s.itemName {
		parts = append(parts, s.itemKey)
	}
	parts = append(parts, paneLabel(s.focusedPane))
	return strings.Join(parts, " / ")
}

func paneLabel(pane int) string {
	switch pane {
	case vaultsPane:
		return "vaults"
	case kindPane:
		return "kind"
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
	snap := kvNavSnapshot{
		subscriptionID: m.CurrentSub.ID,
		focusedPane:    m.focus,
	}
	if m.hasVault {
		snap.vaultName = m.currentVault.Name
		snap.kind = m.kvKind
		snap.itemName = m.currentVersionOwnerName()
	}
	switch m.focus {
	case vaultsPane:
		if it, ok := m.vaultsList.SelectedItem().(vaultItem); ok {
			snap.itemKey = it.vault.Name
		}
	case secretsPane:
		snap.itemKey = middleItemKeyForList(m.kvKind)(m.secretsList.SelectedItem())
	case versionsPane:
		if it := m.versionsList.SelectedItem(); it != nil {
			snap.itemKey = versionItemKey(it)
		}
	}
	return snap
}

// ApplyNav restores a captured position. applyingNav suppresses
// RecordJumpMsg emission from drill-in helpers so restoration doesn't
// re-record entries we're traversing.
//
// A snapshot with empty vaultName represents the pre-drill state (user
// was on the vaults list itself); restoring it is just a focus change
// plus a cursor restore.
func (m *Model) ApplyNav(snap jumplist.NavSnapshot) tea.Cmd {
	s, ok := snap.(kvNavSnapshot)
	if !ok {
		return nil
	}
	m.applyingNav = true
	defer func() { m.applyingNav = false }()

	var cmds []tea.Cmd

	// Best-effort subscription restore when the snapshot was taken
	// under a different subscription.
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

	if s.vaultName == "" {
		if s.focusedPane >= vaultsPane && s.focusedPane <= versionsPane {
			m.transitionTo(s.focusedPane)
		}
		ui.SelectByKey(&m.vaultsList, s.itemKey, vaultItemKey)
		return batchNavCmds(cmds)
	}

	nav := PendingNav{
		VaultName: s.vaultName,
		Kind:      s.kind,
		ItemName:  s.itemName,
	}
	// When the snapshot sat on the items column without a bound
	// versions item, restoring the cursor row is the deepest target.
	if s.focusedPane == secretsPane && nav.ItemName == "" {
		nav.SelectKey = s.itemKey
	}
	if cmd := m.SetPendingNav(nav); cmd != nil {
		cmds = append(cmds, cmd)
	}
	if s.focusedPane >= vaultsPane && s.focusedPane <= versionsPane {
		m.transitionTo(s.focusedPane)
	}
	// Cache-warm best-effort cursor restores for panes the PendingNav
	// chain doesn't position.
	switch s.focusedPane {
	case secretsPane:
		ui.SelectByKey(&m.secretsList, s.itemKey, middleItemKeyForList(s.kind))
	case versionsPane:
		ui.SelectByKey(&m.versionsList, s.itemKey, versionItemKey)
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
// in flight.
func recordDeparture(m Model, depart jumplist.NavSnapshot, cmd tea.Cmd) tea.Cmd {
	return jumplist.AppendRecord(m.applyingNav, m.pendingNav.hasTarget(), depart, cmd)
}

// StrictAncestorOf reports whether s sits strictly above other on the
// same drill path (subscription → vault → kind → secret/cert/key).
// Equal scope returns false — pane/cursor differences are legitimate
// walk stops. Implements jumplist.ScopeAncestor so ctrl+o/ctrl+i walk
// past h-equivalent hops.
func (s kvNavSnapshot) StrictAncestorOf(other jumplist.NavSnapshot) bool {
	o, ok := other.(kvNavSnapshot)
	if !ok || s.subscriptionID != o.subscriptionID {
		return false
	}
	if s.vaultName != "" {
		if s.vaultName != o.vaultName || s.kind != o.kind {
			return false
		}
		if s.itemName != "" && s.itemName != o.itemName {
			return false
		}
	}
	sameScope := s.vaultName == o.vaultName && s.kind == o.kind && s.itemName == o.itemName
	return !sameScope
}
