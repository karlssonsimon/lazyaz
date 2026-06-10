package kvapp

import (
	"github.com/karlssonsimon/lazyaz/internal/azure/keyvault"
	"github.com/karlssonsimon/lazyaz/internal/ui"

	tea "charm.land/bubbletea/v2"
)

// PendingNav describes a navigation target for the Key Vault tab to
// land on. Empty VaultName means "no nav". Kind selects the middle
// column's type (zero value = secrets, so callers that only know about
// secrets keep working). ItemName drills into that secret/cert/key's
// versions; empty stops at the items column. SelectKey optionally
// places the items-column cursor without drilling (used by jump
// restoration when the user sat on a row without opening it).
type PendingNav struct {
	VaultName string
	Kind      kvKind
	ItemName  string
	SelectKey string
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

// itemsForKind returns the loaded middle-column entries' names for the
// given kind. Used by the pending-nav chain to find drill targets.
func (m Model) itemsForKind(kind kvKind) []string {
	switch kind {
	case kvKindCertificates:
		names := make([]string, len(m.certs))
		for i, c := range m.certs {
			names[i] = c.Name
		}
		return names
	case kvKindKeys:
		names := make([]string, len(m.keys))
		for i, k := range m.keys {
			names[i] = k.Name
		}
		return names
	default:
		names := make([]string, len(m.secrets))
		for i, s := range m.secrets {
			names[i] = s.Name
		}
		return names
	}
}

// selectItemByName drills into the named secret/cert/key of the given
// kind. Caller has verified the name exists in the loaded slice.
func (m Model) selectItemByName(kind kvKind, name string) (Model, tea.Cmd) {
	switch kind {
	case kvKindCertificates:
		for _, c := range m.certs {
			if c.Name == name {
				return m.selectCert(c)
			}
		}
	case kvKindKeys:
		for _, k := range m.keys {
			if k.Name == name {
				return m.selectKey(k)
			}
		}
	default:
		for _, s := range m.secrets {
			if s.Name == name {
				return m.selectSecret(s)
			}
		}
	}
	return m, nil
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

	// Step 2: switch to the target kind. Without this the per-kind load
	// handler never fires, the nav never completes, and hasTarget()
	// suppresses jump recording forever.
	if m.kvKind != target.Kind {
		return m.selectKind(target.Kind)
	}

	// Step 3: drill into the item (or just place the cursor).
	if target.ItemName == "" {
		if target.SelectKey != "" {
			ui.SelectByKey(&m.secretsList, target.SelectKey, middleItemKeyForList(target.Kind))
		}
		m.pendingNav = PendingNav{}
		return m, nil
	}
	names := m.itemsForKind(target.Kind)
	if len(names) == 0 {
		return m, nil // wait for the kind's load to land
	}
	for _, name := range names {
		if name == target.ItemName {
			updated, cmd := m.selectItemByName(target.Kind, name)
			updated.pendingNav = PendingNav{}
			return updated, cmd
		}
	}
	m.pendingNav = PendingNav{}
	return m, nil
}

// eagerNavigate walks as far down the pending target as the cache
// allows synchronously. The selectVault/selectItemByName helpers
// hydrate from cache when warm.
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

	// Kind switch: let the async per-kind load drive advancePendingNav
	// the rest of the way when the cache is cold.
	if m.kvKind != target.Kind {
		updated, cmd = m.selectKind(target.Kind)
		m = updated
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
		return m, batchNavCmds(cmds)
	}

	if target.ItemName == "" {
		if target.SelectKey != "" {
			ui.SelectByKey(&m.secretsList, target.SelectKey, middleItemKeyForList(target.Kind))
		}
		m.pendingNav = PendingNav{}
		return m, batchNavCmds(cmds)
	}
	names := m.itemsForKind(target.Kind)
	if len(names) == 0 {
		return m, batchNavCmds(cmds)
	}
	for _, name := range names {
		if name == target.ItemName {
			updated, cmd = m.selectItemByName(target.Kind, name)
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
