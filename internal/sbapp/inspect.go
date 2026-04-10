package sbapp

import (
	"fmt"

	"github.com/karlssonsimon/lazyaz/internal/azure/servicebus"
	"github.com/karlssonsimon/lazyaz/internal/ui"
)

func (m Model) inspectFor(pane int) (string, []ui.InspectField) {
	switch pane {
	case namespacesPane:
		item, ok := m.namespacesList.SelectedItem().(namespaceItem)
		if !ok {
			return "Namespace", nil
		}
		ns := item.namespace
		return "Namespace", []ui.InspectField{
			{Label: "Name", Value: ns.Name},
			{Label: "Subscription", Value: ns.SubscriptionID},
			{Label: "Resource Group", Value: ns.ResourceGroup},
			{Label: "FQDN", Value: ns.FQDN},
		}
	case entitiesPane:
		item, ok := m.entitiesList.SelectedItem().(entityItem)
		if !ok {
			return "Entity", nil
		}
		e := item.entity
		kind := "Queue"
		if e.Kind == servicebus.EntityTopic {
			kind = "Topic"
		}
		return kind, []ui.InspectField{
			{Label: "Name", Value: e.Name},
			{Label: "Kind", Value: kind},
			{Label: "Active Messages", Value: fmt.Sprintf("%d", e.ActiveMsgCount)},
			{Label: "Dead Letter", Value: fmt.Sprintf("%d", e.DeadLetterCount)},
		}
	case subscriptionsPane:
		item, ok := m.subscriptionsList.SelectedItem().(subscriptionItem)
		if !ok {
			return "Subscription", nil
		}
		s := item.sub
		return "Topic Subscription", []ui.InspectField{
			{Label: "Name", Value: s.Name},
			{Label: "Parent Topic", Value: m.currentEntity.Name},
			{Label: "Active Messages", Value: fmt.Sprintf("%d", s.ActiveMsgCount)},
			{Label: "Dead Letter", Value: fmt.Sprintf("%d", s.DeadLetterCount)},
		}
	case messagesPane:
		if item, ok := m.messageList.SelectedItem().(messageItem); ok {
			msg := item.message
			return "Message", []ui.InspectField{
				{Label: "Message ID", Value: ui.EmptyToDash(msg.MessageID)},
				{Label: "Enqueued At", Value: ui.FormatTime(msg.EnqueuedAt)},
				{Label: "Body Preview", Value: compactPreview(msg.BodyPreview, 80)},
			}
		}
		return "Message", nil
	}
	return "", nil
}

func (m Model) inspectFooterHeight(pane int) int {
	if !m.inspectPanes[pane] {
		return 0
	}
	_, fields := m.inspectFor(pane)
	return ui.InspectStripHeight(fields)
}

func (m Model) inspectFooter(pane, contentWidth int) string {
	if !m.inspectPanes[pane] {
		return ""
	}
	title, fields := m.inspectFor(pane)
	return ui.RenderInspectStrip(title, fields, m.Styles, contentWidth)
}

func (m *Model) toggleInspect() {
	if m.inspectPanes == nil {
		m.inspectPanes = make(map[int]bool)
	}
	m.inspectPanes[m.focus] = !m.inspectPanes[m.focus]
	m.resize()
}
