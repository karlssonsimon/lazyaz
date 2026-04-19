package sbapp

import (
	"github.com/karlssonsimon/lazyaz/internal/azure/servicebus"
	"github.com/karlssonsimon/lazyaz/internal/ui"

	tea "charm.land/bubbletea/v2"
)

// PendingNav describes a navigation target the dashboard wants the
// Service Bus tab to land on. It's set once via SetPendingNav, then the
// state machine in advancePendingNav drives the selection forward as
// each fetch (namespaces → entities → topic subs) completes.
//
// Empty fields short-circuit the corresponding step:
//   - EntityName == "" stops at the namespace
//   - SubName == "" treats EntityName as a queue (skip subscription step)
//   - DeadLetter == false ends on the queue type picker; true peeks DLQ
type PendingNav struct {
	Namespace  servicebus.Namespace
	EntityName string
	SubName    string
	DeadLetter bool
}

// hasTarget reports whether any navigation work is left.
func (p PendingNav) hasTarget() bool {
	return p.Namespace.Name != ""
}

// SetPendingNav records a navigation intent and immediately tries to
// fast-forward through any cached layers so the user lands on the
// destination without watching fetches stage in. The returned tea.Cmd
// carries any side-effect commands the eager navigation produced
// (typically background fetches that refresh stale cache entries).
//
// If a layer isn't in the cache, the eager walk stops there and the
// regular advancePendingNav (called from each load handler) picks up
// once the fetch arrives. Same end state, just slower.
func (m *Model) SetPendingNav(p PendingNav) tea.Cmd {
	m.pendingNav = p
	updated, cmd := m.eagerNavigate()
	*m = updated
	return cmd
}

// eagerNavigate walks as far down the pending-nav target as the cache
// allows, calling the existing select* helpers so all the usual state
// bookkeeping (history snapshots, list items, focus transitions) runs.
// The select helpers fire fetches for refresh; we batch and return
// those commands so they run in the background.
func (m Model) eagerNavigate() (Model, tea.Cmd) {
	if !m.pendingNav.hasTarget() || !m.HasSubscription {
		return m, nil
	}
	target := m.pendingNav
	var cmds []tea.Cmd

	// Hydrate namespaces from the shared broker if we don't have them
	// yet (the dashboard typically warmed this).
	if len(m.namespaces) == 0 {
		if cached, ok := m.cache.namespaces.Get(m.CurrentSub.ID); ok {
			m.namespaces = cached
			ui.SetItemsPreserveKey(&m.namespacesList, namespacesToItems(cached), namespaceItemKey)
		}
	}
	if len(m.namespaces) == 0 {
		return m, nil // wait for namespace fetch
	}

	var targetNS servicebus.Namespace
	found := false
	for _, ns := range m.namespaces {
		if ns.Name == target.Namespace.Name {
			targetNS = ns
			found = true
			break
		}
	}
	if !found {
		return m, nil
	}

	updated, cmd := m.selectNamespace(targetNS)
	m = updated
	if cmd != nil {
		cmds = append(cmds, cmd)
	}

	if target.EntityName == "" {
		m.pendingNav = PendingNav{}
		return m, batchCmds(cmds)
	}
	if len(m.entities) == 0 {
		return m, batchCmds(cmds) // wait for entity fetch
	}

	var targetEntity servicebus.Entity
	found = false
	for _, e := range m.entities {
		if e.Name == target.EntityName {
			targetEntity = e
			found = true
			break
		}
	}
	if !found {
		return m, batchCmds(cmds)
	}

	if targetEntity.Kind == servicebus.EntityQueue {
		updated, cmd = m.selectQueue(targetEntity)
		m = updated
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
		if target.DeadLetter {
			updated, cmd = m.peekMessages(true)
			m = updated
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		}
		m.pendingNav = PendingNav{}
		return m, batchCmds(cmds)
	}

	if targetEntity.Kind == servicebus.EntityTopic {
		updated, cmd = m.selectTopic(targetEntity)
		m = updated
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
		if target.SubName == "" {
			m.pendingNav = PendingNav{}
			return m, batchCmds(cmds)
		}
		if len(m.subscriptions) == 0 {
			return m, batchCmds(cmds) // wait for topic-subs fetch
		}
		var targetSub servicebus.TopicSubscription
		found = false
		for _, s := range m.subscriptions {
			if s.Name == target.SubName {
				targetSub = s
				found = true
				break
			}
		}
		if !found {
			return m, batchCmds(cmds)
		}
		updated, cmd = m.selectSubscriptionSub(targetEntity.Name, targetSub)
		m = updated
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
		if target.DeadLetter {
			updated, cmd = m.peekMessages(true)
			m = updated
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		}
		m.pendingNav = PendingNav{}
		return m, batchCmds(cmds)
	}
	return m, batchCmds(cmds)
}

func batchCmds(cmds []tea.Cmd) tea.Cmd {
	if len(cmds) == 0 {
		return nil
	}
	if len(cmds) == 1 {
		return cmds[0]
	}
	return tea.Batch(cmds...)
}

// advancePendingNav inspects the current model state against the
// pending target and takes the next step toward it. Returns the next
// command to run (typically a fetch) or nil if no progress is possible
// right now (waiting for the next load to complete).
//
// Called from the tail of each load handler (handleNamespacesLoaded,
// handleEntitiesLoaded, handleTopicSubscriptionsLoaded).
func (m Model) advancePendingNav() (Model, tea.Cmd) {
	if !m.pendingNav.hasTarget() {
		return m, nil
	}
	target := m.pendingNav

	// Step 1: select the namespace if we haven't already.
	if !m.hasNamespace || m.currentNS.Name != target.Namespace.Name {
		// Confirm the target namespace actually exists in the loaded
		// list. If not, the dashboard pointed at something that no
		// longer exists — clear the pending target to avoid spinning.
		var match servicebus.Namespace
		var found bool
		for _, ns := range m.namespaces {
			if ns.Name == target.Namespace.Name {
				match = ns
				found = true
				break
			}
		}
		if !found {
			// Namespaces haven't loaded yet, or the target is gone.
			// If list is non-empty and target is missing, give up.
			if len(m.namespaces) > 0 {
				m.pendingNav = PendingNav{}
			}
			return m, nil
		}
		updated, cmd := m.selectNamespace(match)
		return updated, cmd
	}

	// Step 2: drill into the entity (if requested).
	if target.EntityName == "" {
		// Just wanted to land on the namespace. Done.
		m.pendingNav = PendingNav{}
		return m, nil
	}

	// Wait until entities are loaded for this namespace.
	if len(m.entities) == 0 {
		return m, nil
	}

	var entity servicebus.Entity
	var found bool
	for _, e := range m.entities {
		if e.Name == target.EntityName {
			entity = e
			found = true
			break
		}
	}
	if !found {
		m.pendingNav = PendingNav{}
		return m, nil
	}

	// Queue path: bind queue type picker, then optionally peek DLQ.
	if entity.Kind == servicebus.EntityQueue {
		// If we haven't bound to this queue yet, do it.
		if !m.hasPeekTarget || m.currentEntity.Name != entity.Name || m.currentSubName != "" {
			updated, cmd := m.selectQueue(entity)
			// If a DLQ landing is requested, transition further.
			if target.DeadLetter {
				updated2, cmd2 := updated.peekMessages(true)
				updated2.pendingNav = PendingNav{}
				return updated2, tea.Batch(cmd, cmd2)
			}
			updated.pendingNav = PendingNav{}
			return updated, cmd
		}
		// Already bound — switch to DLQ pane if requested.
		if target.DeadLetter && !m.deadLetter {
			updated, cmd := m.peekMessages(true)
			updated.pendingNav = PendingNav{}
			return updated, cmd
		}
		m.pendingNav = PendingNav{}
		return m, nil
	}

	// Topic path: load subscriptions, then bind to the requested sub.
	if entity.Kind == servicebus.EntityTopic {
		// Step 3: select the topic to load its subscriptions.
		if m.currentEntity.Name != entity.Name || !m.isTopicSelected() {
			return m.selectTopic(entity)
		}
		// Wait for topic subs to load.
		if len(m.subscriptions) == 0 {
			return m, nil
		}
		// Find the target subscription.
		if target.SubName == "" {
			// No specific sub — leave on the subscriptions pane.
			m.pendingNav = PendingNav{}
			return m, nil
		}
		var sub servicebus.TopicSubscription
		var foundSub bool
		for _, s := range m.subscriptions {
			if s.Name == target.SubName {
				sub = s
				foundSub = true
				break
			}
		}
		if !foundSub {
			m.pendingNav = PendingNav{}
			return m, nil
		}
		// Bind the queue type picker to this subscription.
		if !m.hasPeekTarget || m.currentSubName != sub.Name {
			updated, cmd := m.selectSubscriptionSub(entity.Name, sub)
			if target.DeadLetter {
				updated2, cmd2 := updated.peekMessages(true)
				updated2.pendingNav = PendingNav{}
				return updated2, tea.Batch(cmd, cmd2)
			}
			updated.pendingNav = PendingNav{}
			return updated, cmd
		}
		// Already bound — switch to DLQ if requested.
		if target.DeadLetter && !m.deadLetter {
			updated, cmd := m.peekMessages(true)
			updated.pendingNav = PendingNav{}
			return updated, cmd
		}
		m.pendingNav = PendingNav{}
		return m, nil
	}

	m.pendingNav = PendingNav{}
	return m, nil
}
