package sbapp

import (
	"strings"

	"github.com/karlssonsimon/lazyaz/internal/azure/servicebus"
	"github.com/karlssonsimon/lazyaz/internal/jumplist"
	"github.com/karlssonsimon/lazyaz/internal/ui"

	tea "charm.land/bubbletea/v2"
)

// sbNavSnapshot captures the user's position in the Service Bus
// explorer: subscription + namespace + (optionally) entity + (for
// topics) topic subscription + DLQ flag + the selected row + focused
// pane. ApplyNav restores this via the existing PendingNav fast-forward
// path so cache-warmed jumps are instant.
type sbNavSnapshot struct {
	subscriptionID string
	namespace      servicebus.Namespace
	entityName     string
	subName        string
	deadLetter     bool
	itemKey        string // selected row in the focused pane ("" when none)
	focusedPane    int
}

func (s sbNavSnapshot) Description() string {
	parts := []string{"sb"}
	if s.namespace.Name != "" {
		parts = append(parts, s.namespace.Name)
	}
	if s.entityName != "" {
		parts = append(parts, s.entityName)
	}
	if s.subName != "" {
		parts = append(parts, s.subName)
	}
	if s.deadLetter {
		parts = append(parts, "DLQ")
	}
	if s.itemKey != "" {
		parts = append(parts, s.itemKey)
	}
	parts = append(parts, paneLabel(s.focusedPane))
	return strings.Join(parts, " / ")
}

func paneLabel(pane int) string {
	switch pane {
	case namespacesPane:
		return "namespaces"
	case entitiesPane:
		return "entities"
	case subscriptionsPane:
		return "subscriptions"
	case queueTypePane:
		return "queuetype"
	case messagesPane:
		return "messages"
	case messagePreviewPane:
		return "preview"
	default:
		return "?"
	}
}

// CurrentNav captures the active position. Returns nil only when no
// subscription is set — the namespaces-list view (focus=namespacesPane,
// hasNamespace=false) is a meaningful jump target so ctrl+o can walk
// back to it after the user drills into a namespace.
func (m Model) CurrentNav() jumplist.NavSnapshot {
	if !m.HasSubscription {
		return nil
	}
	snap := sbNavSnapshot{
		subscriptionID: m.CurrentSub.ID,
		focusedPane:    m.focus,
	}
	if m.hasNamespace {
		snap.namespace = m.currentNS
		snap.entityName = m.currentEntity.Name
		snap.subName = m.currentSubName
		snap.deadLetter = m.deadLetter
	}
	switch m.focus {
	case namespacesPane:
		if it, ok := m.namespacesList.SelectedItem().(namespaceItem); ok {
			snap.itemKey = it.namespace.Name
		}
	case entitiesPane:
		if it, ok := m.entitiesList.SelectedItem().(entityItem); ok {
			snap.itemKey = it.entity.Name
		}
	case subscriptionsPane:
		if it, ok := m.subscriptionsList.SelectedItem().(subscriptionItem); ok {
			snap.itemKey = it.sub.Name
		}
	case messagesPane, messagePreviewPane:
		if it, ok := m.messageList.SelectedItem().(messageItem); ok {
			snap.itemKey = messageOperationKey(it.message)
		}
	}
	return snap
}

// ApplyNav restores a captured position. The applyingNav flag
// suppresses RecordJumpMsg emission from the drill-in helpers we're
// about to call — restoring should not append the destination as a
// fresh jump entry.
//
// A snapshot with empty namespace.Name represents the pre-drill state
// (user was on the namespaces list itself); restoring is just a focus
// change plus a cursor restore.
func (m *Model) ApplyNav(snap jumplist.NavSnapshot) tea.Cmd {
	s, ok := snap.(sbNavSnapshot)
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

	if s.namespace.Name == "" {
		if s.focusedPane >= namespacesPane && s.focusedPane <= messagePreviewPane {
			m.transitionTo(s.focusedPane)
		}
		ui.SelectByKey(&m.namespacesList, s.itemKey, namespaceItemKey)
		return batchCmds(cmds)
	}

	if cmd := m.SetPendingNav(PendingNav{
		Namespace:  s.namespace,
		EntityName: s.entityName,
		SubName:    s.subName,
		DeadLetter: s.deadLetter,
	}); cmd != nil {
		cmds = append(cmds, cmd)
	}
	// Restore pane focus after the drill-in lands.
	if s.focusedPane >= namespacesPane && s.focusedPane <= messagePreviewPane {
		m.transitionTo(s.focusedPane)
	}
	// Cache-warm best-effort cursor restores. Messages are not
	// re-peeked on restore (peek is a deliberate action), so the
	// message-row key usually has nothing to land on — harmless no-op.
	switch s.focusedPane {
	case entitiesPane:
		ui.SelectByKey(&m.entitiesList, s.itemKey, entityItemKey)
	case subscriptionsPane:
		ui.SelectByKey(&m.subscriptionsList, s.itemKey, subscriptionItemKey)
	case messagesPane, messagePreviewPane:
		ui.SelectByKey(&m.messageList, s.itemKey, messageItemKey)
	}
	return batchCmds(cmds)
}

func (m Model) WithAppliedNav(snap jumplist.NavSnapshot) (tea.Model, tea.Cmd) {
	cmd := m.ApplyNav(snap)
	return m, cmd
}

// recordDeparture batches cmd with a jump record for the position the
// user is LEAVING (vim records origins, not destinations). Callers
// capture the snapshot via CurrentNav() BEFORE mutating scope. Records
// are suppressed during ApplyNav restoration and while a PendingNav is
// in flight — the parent records the origin side of cross-tab opens
// directly, so programmatic hops would just pollute ctrl+o history.
func recordDeparture(m Model, depart jumplist.NavSnapshot, cmd tea.Cmd) tea.Cmd {
	return jumplist.AppendRecord(m.applyingNav, m.pendingNav.hasTarget(), depart, cmd)
}

// StrictAncestorOf reports whether s sits strictly above other on the
// same drill path (subscription → namespace → entity → topic sub →
// queue type). Equal scope returns false — pane/cursor differences are
// legitimate walk stops. Implements jumplist.ScopeAncestor so
// ctrl+o/ctrl+i walk past h-equivalent hops.
func (s sbNavSnapshot) StrictAncestorOf(other jumplist.NavSnapshot) bool {
	o, ok := other.(sbNavSnapshot)
	if !ok || s.subscriptionID != o.subscriptionID {
		return false
	}
	if s.namespace.Name != "" && s.namespace.Name != o.namespace.Name {
		return false
	}
	if s.entityName != "" {
		if s.entityName != o.entityName {
			return false
		}
		// At entity depth the Active/DLQ choice is part of the
		// position — a different queue type is a sibling, not an
		// ancestor.
		if s.deadLetter != o.deadLetter {
			return false
		}
	}
	if s.subName != "" && s.subName != o.subName {
		return false
	}
	sameScope := s.namespace.Name == o.namespace.Name && s.entityName == o.entityName &&
		s.subName == o.subName && s.deadLetter == o.deadLetter
	return !sameScope
}
