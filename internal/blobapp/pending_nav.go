package blobapp

import (
	"github.com/karlssonsimon/lazyaz/internal/azure/blob"

	tea "charm.land/bubbletea/v2"
)

// PendingNav describes a navigation target the dashboard wants the
// Blob tab to land on. Empty AccountName means "no nav"; empty
// ContainerName stops at the account.
type PendingNav struct {
	AccountName   string
	ContainerName string
}

func (p PendingNav) hasTarget() bool { return p.AccountName != "" }

// SetPendingNav records the intent and immediately fast-forwards
// through cached layers so the user lands on the destination without
// watching staged fetches. Refresh fetches still run via Init for
// freshness.
func (m *Model) SetPendingNav(p PendingNav) tea.Cmd {
	m.pendingNav = p
	updated, cmd := m.eagerNavigate()
	*m = updated
	return cmd
}

// advancePendingNav drives one step forward toward the target. Called
// from each load handler's done path so the chain progresses naturally
// when fetches arrive.
func (m Model) advancePendingNav() (Model, tea.Cmd) {
	if !m.pendingNav.hasTarget() {
		return m, nil
	}
	target := m.pendingNav

	// Step 1: select the account if not already.
	if !m.hasAccount || m.currentAccount.Name != target.AccountName {
		var match blob.Account
		var found bool
		for _, a := range m.accounts {
			if a.Name == target.AccountName {
				match = a
				found = true
				break
			}
		}
		if !found {
			if len(m.accounts) > 0 {
				// Account list loaded but target isn't there — give up
				// rather than spin forever.
				m.pendingNav = PendingNav{}
			}
			return m, nil
		}
		updated, cmd := m.selectAccount(match)
		return updated, cmd
	}

	// Step 2: drill into the container.
	if target.ContainerName == "" {
		m.pendingNav = PendingNav{}
		return m, nil
	}

	if len(m.containers) == 0 {
		return m, nil
	}

	for _, c := range m.containers {
		if c.Name == target.ContainerName {
			updated, cmd := m.selectContainer(c)
			updated.pendingNav = PendingNav{}
			return updated, cmd
		}
	}
	m.pendingNav = PendingNav{}
	return m, nil
}

// eagerNavigate walks as far down the pending target as the cache
// allows. selectAccount and selectContainer hydrate from cache when
// the brokers already have the data (the dashboard typically warmed
// them), so this returns instantly with the user on the destination.
// The fetch commands these helpers return run in the background to
// refresh.
func (m Model) eagerNavigate() (Model, tea.Cmd) {
	if !m.pendingNav.hasTarget() || !m.HasSubscription {
		return m, nil
	}
	target := m.pendingNav
	var cmds []tea.Cmd

	if len(m.accounts) == 0 {
		if cached, ok := m.cache.accounts.Get(m.CurrentSub.ID); ok {
			m.accounts = cached
		}
	}
	if len(m.accounts) == 0 {
		return m, nil // wait for accounts fetch
	}

	var account blob.Account
	found := false
	for _, a := range m.accounts {
		if a.Name == target.AccountName {
			account = a
			found = true
			break
		}
	}
	if !found {
		return m, nil
	}

	updated, cmd := m.selectAccount(account)
	m = updated
	if cmd != nil {
		cmds = append(cmds, cmd)
	}

	if target.ContainerName == "" {
		m.pendingNav = PendingNav{}
		return m, batchNavCmds(cmds)
	}
	if len(m.containers) == 0 {
		return m, batchNavCmds(cmds)
	}
	for _, c := range m.containers {
		if c.Name == target.ContainerName {
			updated, cmd = m.selectContainer(c)
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
