package sbapp

import (
	"fmt"

	"azure-storage/internal/azure/servicebus"
	"azure-storage/internal/ui"
)

func (m *Model) inspectFocusedItem() {
	switch m.focus {
	case namespacesPane:
		item, ok := m.namespacesList.SelectedItem().(namespaceItem)
		if !ok {
			return
		}
		ns := item.namespace
		m.inspectTitle = "Namespace"
		m.inspectFields = []ui.InspectField{
			{Label: "Name", Value: ns.Name},
			{Label: "Subscription", Value: ns.SubscriptionID},
			{Label: "Resource Group", Value: ns.ResourceGroup},
			{Label: "FQDN", Value: ns.FQDN},
		}
	case entitiesPane:
		item, ok := m.entitiesList.SelectedItem().(entityItem)
		if !ok {
			return
		}
		e := item.entity
		kind := "Queue"
		if e.Kind == servicebus.EntityTopic {
			kind = "Topic"
		}
		m.inspectTitle = kind
		m.inspectFields = []ui.InspectField{
			{Label: "Name", Value: e.Name},
			{Label: "Kind", Value: kind},
			{Label: "Active Messages", Value: fmt.Sprintf("%d", e.ActiveMsgCount)},
			{Label: "Dead Letter", Value: fmt.Sprintf("%d", e.DeadLetterCount)},
		}
	case detailPane:
		// Could be topic sub or message.
		if item, ok := m.detailList.SelectedItem().(topicSubItem); ok {
			s := item.sub
			m.inspectTitle = "Topic Subscription"
			m.inspectFields = []ui.InspectField{
				{Label: "Name", Value: s.Name},
				{Label: "Active Messages", Value: fmt.Sprintf("%d", s.ActiveMsgCount)},
				{Label: "Dead Letter", Value: fmt.Sprintf("%d", s.DeadLetterCount)},
			}
		} else if item, ok := m.detailList.SelectedItem().(messageItem); ok {
			msg := item.message
			m.inspectTitle = "Message"
			m.inspectFields = []ui.InspectField{
				{Label: "Message ID", Value: ui.EmptyToDash(msg.MessageID)},
				{Label: "Enqueued At", Value: ui.FormatTime(msg.EnqueuedAt)},
				{Label: "Body Preview", Value: compactPreview(msg.BodyPreview, 80)},
			}
		}
	}
}
