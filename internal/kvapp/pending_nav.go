package kvapp

import (
	"github.com/karlssonsimon/lazyaz/internal/azure/keyvault"

	tea "charm.land/bubbletea/v2"
)

// PendingNav describes a navigation target for the Key Vault tab to
// land on. Empty VaultName means "no nav"; empty SecretName stops at
// the vault.
type PendingNav struct {
	VaultName  string
	SecretName string
}

func (p PendingNav) hasTarget() bool { return p.VaultName != "" }

// SetPendingNav records the intent and fast-forwards through cached
// layers so the user lands on the destination without watching staged
// fetches. Refresh fetches still run via Init for freshness.
func (m *Model) SetPendingNav(p PendingNav) tea.Cmd {
	m.pendingNav = p
	updated, cmd := m.eagerNavigate()
	*m = updated
	return cmd
}

// advancePendingNav drives one step forward toward the target. Called
// from load handlers' done paths so the chain progresses naturally.
func (m Model) advancePendingNav() (Model, tea.Cmd) {
	if !m.pendingNav.hasTarget() {
		return m, nil
	}
	target := m.pendingNav

	// Step 1: select the vault if not already.
	if !m.hasVault || m.currentVault.Name != target.VaultName {
		var match keyvault.Vault
		var found bool
		for _, v := range m.vaults {
			if v.Name == target.VaultName {
				match = v
				found = true
				break
			}
		}
		if !found {
			if len(m.vaults) > 0 {
				// Vault list loaded but target isn't there — give up
				// rather than spin forever.
				m.pendingNav = PendingNav{}
			}
			return m, nil
		}
		updated, cmd := m.selectVault(match)
		return updated, cmd
	}

	// Step 2: drill into the secret.
	if target.SecretName == "" {
		m.pendingNav = PendingNav{}
		return m, nil
	}
	if len(m.secrets) == 0 {
		return m, nil
	}
	for _, s := range m.secrets {
		if s.Name == target.SecretName {
			updated, cmd := m.selectSecret(s)
			updated.pendingNav = PendingNav{}
			return updated, cmd
		}
	}
	m.pendingNav = PendingNav{}
	return m, nil
}

// eagerNavigate walks as far down the pending target as the cache
// allows synchronously. The selectVault/selectSecret helpers hydrate
// from cache when warm.
func (m Model) eagerNavigate() (Model, tea.Cmd) {
	if !m.pendingNav.hasTarget() || !m.HasSubscription {
		return m, nil
	}
	target := m.pendingNav
	var cmds []tea.Cmd

	if len(m.vaults) == 0 {
		if cached, ok := m.cache.vaults.Get(m.CurrentSub.ID); ok {
			m.vaults = cached
		}
	}
	if len(m.vaults) == 0 {
		return m, nil
	}

	var vault keyvault.Vault
	found := false
	for _, v := range m.vaults {
		if v.Name == target.VaultName {
			vault = v
			found = true
			break
		}
	}
	if !found {
		return m, nil
	}

	updated, cmd := m.selectVault(vault)
	m = updated
	if cmd != nil {
		cmds = append(cmds, cmd)
	}

	if target.SecretName == "" {
		m.pendingNav = PendingNav{}
		return m, batchNavCmds(cmds)
	}
	if len(m.secrets) == 0 {
		return m, batchNavCmds(cmds)
	}
	for _, s := range m.secrets {
		if s.Name == target.SecretName {
			updated, cmd = m.selectSecret(s)
			m = updated
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
			m.pendingNav = PendingNav{}
			return m, batchNavCmds(cmds)
		}
	}
	return m, batchNavCmds(cmds)
}

func batchNavCmds(cmds []tea.Cmd) tea.Cmd {
	if len(cmds) == 0 {
		return nil
	}
	if len(cmds) == 1 {
		return cmds[0]
	}
	return tea.Batch(cmds...)
}
