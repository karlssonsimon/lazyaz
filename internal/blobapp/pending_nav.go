package blobapp

import (
	"fmt"

	"github.com/karlssonsimon/lazyaz/internal/azure/blob"

	tea "charm.land/bubbletea/v2"
)

// PendingNav describes a navigation target the dashboard wants the
// Blob tab to land on. Empty AccountName means "no nav"; empty
// ContainerName stops at the account. Prefix and BlobName are optional
// extras: Prefix descends into a subfolder ("a/b/" form), and BlobName
// places the cursor on a specific blob row at that level.
type PendingNav struct {
	AccountName   string
	ContainerName string
	Prefix        string
	BlobName      string
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

	if !m.hasContainer || m.containerName != target.ContainerName {
		if len(m.containers) == 0 {
			return m, nil
		}
		for _, c := range m.containers {
			if c.Name == target.ContainerName {
				updated, cmd := m.selectContainer(c)
				// Keep pendingNav set so subsequent loads (prefix/blob)
				// continue the chain. Clearing it here would stop the
				// drill-in at the container.
				return updated, cmd
			}
		}
		m.pendingNav = PendingNav{}
		return m, nil
	}

	// Step 3: descend into the requested prefix. Skipped when the user
	// is already at that prefix. We trigger a hierarchy fetch the same
	// way as a folder-click in the UI.
	if m.prefix != target.Prefix {
		m.prefix = target.Prefix
		blobsScope := blobsCacheKey(m.CurrentSub.ID, m.currentAccount.Name, m.containerName, m.prefix, false)
		if cached, ok := m.cache.blobs.Get(blobsScope); ok {
			m.blobs = cached
		} else {
			m.blobs = nil
		}
		m.refreshItems()
		if len(m.blobs) == 0 {
			m.startLoading(blobsPane, fmt.Sprintf("Loading entries under %q", displayPrefix(m.prefix)))
			return m, tea.Batch(m.Spinner.Tick, fetchHierarchyBlobsCmd(m.service, m.cache.blobs, m.currentAccount, m.containerName, m.prefix, defaultHierarchyBlobLoadLimit, m.blobs))
		}
	}

	// Step 4: place the cursor on the requested blob row, if any.
	if target.BlobName == "" {
		m.pendingNav = PendingNav{}
		return m, nil
	}
	if len(m.blobs) == 0 {
		return m, nil // wait for blobs to arrive
	}
	m.selectBlobRow(target.Prefix, target.BlobName)
	m.pendingNav = PendingNav{}
	return m, nil
}

// selectBlobRow positions the cursor on the blob with name
// "<prefix><leaf>" in the current blobsList. Folder rows are skipped —
// the target is always a leaf blob. No-op if the row isn't present
// (e.g. the prefix's blob page hasn't loaded yet).
func (m *Model) selectBlobRow(prefix, leaf string) {
	full := prefix + leaf
	for i, it := range m.blobsList.VisibleItems() {
		bi, ok := it.(blobItem)
		if !ok || bi.blob.IsPrefix {
			continue
		}
		if bi.blob.Name == full {
			m.blobsList.Select(i)
			return
		}
	}
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
	containerFound := false
	for _, c := range m.containers {
		if c.Name == target.ContainerName {
			updated, cmd = m.selectContainer(c)
			m = updated
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
			containerFound = true
			break
		}
	}
	if !containerFound {
		return m, batchNavCmds(cmds)
	}

	if target.Prefix == "" && target.BlobName == "" {
		m.pendingNav = PendingNav{}
		return m, batchNavCmds(cmds)
	}

	// Try the prefix's blobs from cache. If hot, set m.prefix and run
	// the blob-row select inline; otherwise leave the chain pending so
	// handleBlobsLoaded can finish it once the fetch returns.
	if target.Prefix != "" {
		m.prefix = target.Prefix
		blobsScope := blobsCacheKey(m.CurrentSub.ID, m.currentAccount.Name, m.containerName, m.prefix, false)
		if cached, ok := m.cache.blobs.Get(blobsScope); ok {
			m.blobs = cached
			m.refreshItems()
		} else {
			m.startLoading(blobsPane, fmt.Sprintf("Loading entries under %q", displayPrefix(m.prefix)))
			cmds = append(cmds, m.Spinner.Tick, fetchHierarchyBlobsCmd(m.service, m.cache.blobs, m.currentAccount, m.containerName, m.prefix, defaultHierarchyBlobLoadLimit, m.blobs))
			return m, batchNavCmds(cmds)
		}
	}

	if target.BlobName == "" {
		m.pendingNav = PendingNav{}
		return m, batchNavCmds(cmds)
	}
	m.selectBlobRow(target.Prefix, target.BlobName)
	m.pendingNav = PendingNav{}
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
