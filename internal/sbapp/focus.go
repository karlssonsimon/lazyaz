package sbapp

import (
	"github.com/karlssonsimon/lazyaz/internal/ui"

	"charm.land/bubbles/v2/list"
)

func (m *Model) setFocus(pane int) {
	m.focus = pane
	m.resize()
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
	m.blurAllFilters()
	panes := m.navigablePanes()
	for i, p := range panes {
		if p == m.focus {
			m.focus = panes[(i+1)%len(panes)]
			m.resize()
			return
		}
	}
	m.focus = panes[0]
	m.resize()
}

func (m *Model) previousFocus() {
	m.blurAllFilters()
	panes := m.navigablePanes()
	for i, p := range panes {
		if p == m.focus {
			m.focus = panes[(i-1+len(panes))%len(panes)]
			m.resize()
			return
		}
	}
	m.focus = panes[len(panes)-1]
	m.resize()
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
	return m.focusedListSettingFilter() || m.SubOverlay.Active || m.entitySortOverlay.active || m.targetPicker.active
}
