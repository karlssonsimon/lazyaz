package sbapp

import (
	"github.com/karlssonsimon/lazyaz/internal/cache"
	"github.com/karlssonsimon/lazyaz/internal/ui"

	"charm.land/bubbles/v2/list"
)

// snapshotCurrentPane saves the focused pane's cursor and filter into
// the appropriate history map so they survive navigation.
func (m *Model) snapshotCurrentPane() {
	switch m.focus {
	case namespacesPane:
		if m.HasSubscription {
			m.namespacesHistory[m.CurrentSub.ID] = ui.SnapshotListState(&m.namespacesList, namespaceItemKey)
		}
	case entitiesPane:
		if m.hasNamespace {
			m.entitiesHistory[cache.Key(m.CurrentSub.ID, m.currentNS.Name)] = ui.SnapshotListState(&m.entitiesList, entityItemKey)
		}
	case subscriptionsPane:
		if m.isTopicSelected() {
			m.subscriptionsHistory[cache.Key(m.CurrentSub.ID, m.currentNS.Name, m.currentEntity.Name)] = ui.SnapshotListState(&m.subscriptionsList, subscriptionItemKey)
		}
	}
}

// restoreCurrentPane re-applies saved cursor and filter for the focused
// pane from the history map, ensuring filters survive transitions.
func (m *Model) restoreCurrentPane() {
	switch m.focus {
	case namespacesPane:
		if m.HasSubscription {
			ui.RestoreListState(&m.namespacesList, m.namespacesHistory[m.CurrentSub.ID], namespaceItemKey)
		}
	case entitiesPane:
		if m.hasNamespace {
			ui.RestoreListState(&m.entitiesList, m.entitiesHistory[cache.Key(m.CurrentSub.ID, m.currentNS.Name)], entityItemKey)
		}
	case subscriptionsPane:
		if m.isTopicSelected() {
			ui.RestoreListState(&m.subscriptionsList, m.subscriptionsHistory[cache.Key(m.CurrentSub.ID, m.currentNS.Name, m.currentEntity.Name)], subscriptionItemKey)
		}
	}
}

// exitPane cleans up the outgoing pane before a transition.
// Snapshots the pane's state, blurs filter inputs, and exits visual
// mode if active.
func (m *Model) exitPane() {
	m.snapshotCurrentPane()
	m.blurAllFilters()
	if m.focus == messagesPane && m.visualLineMode {
		m.visualLineMode = false
		m.visualAnchor = ""
	}
}

// transitionTo performs exitPane cleanup on the current pane, then sets
// focus to the target pane and restores its saved state. This is the
// single codepath for all focus changes, guaranteeing that filters and
// cursor positions survive navigation.
func (m *Model) transitionTo(pane int) {
	m.exitPane()
	m.focus = pane
	m.resize()
	m.restoreCurrentPane()
}

// navigablePanes returns the ordered list of pane indices the user can
// cycle through.
func (m Model) navigablePanes() []int {
	panes := []int{namespacesPane, entitiesPane}
	if m.isTopicSelected() {
		panes = append(panes, subscriptionsPane)
	}
	if m.hasPeekTarget {
		panes = append(panes, queueTypePane)
	}
	if m.hasPeekTarget && (m.focus == messagesPane || m.focus == messagePreviewPane || len(m.peekedMessages) > 0) {
		panes = append(panes, messagesPane)
	}
	if m.viewingMessage {
		panes = append(panes, messagePreviewPane)
	}
	return panes
}

func (m *Model) nextFocus() {
	panes := m.navigablePanes()
	next := panes[0]
	for i, p := range panes {
		if p == m.focus {
			next = panes[(i+1)%len(panes)]
			break
		}
	}
	m.transitionTo(next)
}

func (m *Model) previousFocus() {
	panes := m.navigablePanes()
	prev := panes[len(panes)-1]
	for i, p := range panes {
		if p == m.focus {
			prev = panes[(i-1+len(panes))%len(panes)]
			break
		}
	}
	m.transitionTo(prev)
}

func (m *Model) blurAllFilters() {
	m.namespacesList.FilterInput.Blur()
	m.entitiesList.FilterInput.Blur()
	m.subscriptionsList.FilterInput.Blur()
	m.messageList.FilterInput.Blur()
}

func (m *Model) commitFocusedFilter() {
	m.blurAllFilters()
	switch m.focus {
	case namespacesPane:
		ui.ApplyFilterState(&m.namespacesList)
	case entitiesPane:
		ui.ApplyFilterState(&m.entitiesList)
	case subscriptionsPane:
		ui.ApplyFilterState(&m.subscriptionsList)
	case messagesPane:
		ui.ApplyFilterState(&m.messageList)
	}
}

func (m *Model) scrollFocusedHalfPage(direction int) {
	if direction == 0 {
		return
	}
	var target *list.Model
	switch m.focus {
	case namespacesPane:
		target = &m.namespacesList
	case entitiesPane:
		target = &m.entitiesList
	case subscriptionsPane:
		target = &m.subscriptionsList
	case queueTypePane:
		target = &m.queueTypeList
	case messagesPane:
		target = &m.messageList
	default:
		return
	}
	steps := ui.HalfPageStep(*target)
	for i := 0; i < steps; i++ {
		if direction > 0 {
			target.CursorDown()
		} else {
			target.CursorUp()
		}
	}
}

func (m Model) focusedListSettingFilter() bool {
	switch m.focus {
	case namespacesPane:
		return m.namespacesList.SettingFilter()
	case entitiesPane:
		return m.entitiesList.SettingFilter()
	case subscriptionsPane:
		return m.subscriptionsList.SettingFilter()
	case messagesPane:
		return m.messageList.SettingFilter()
	default:
		return false
	}
}

// IsTextInputActive reports whether the model is currently accepting
// free-form text input.
func (m Model) IsTextInputActive() bool {
	switch m.inputMode() {
	case ModeNormal, ModeVisualLine:
		return false
	default:
		return true
	}
}
