package sbapp

import (
	"fmt"

	"azure-storage/internal/azure/servicebus"
	"azure-storage/internal/ui"
)

// inspectFor returns the inspect title and field list for the given pane,
// based on its currently selected item. Returns ("", nil) if the pane has
// no inspectable selection.
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
	case detailPane:
		// Could be topic sub or message — check the underlying selection.
		if item, ok := m.detailList.SelectedItem().(topicSubItem); ok {
			s := item.sub
			return "Topic Subscription", []ui.InspectField{
				{Label: "Name", Value: s.Name},
				{Label: "Active Messages", Value: fmt.Sprintf("%d", s.ActiveMsgCount)},
				{Label: "Dead Letter", Value: fmt.Sprintf("%d", s.DeadLetterCount)},
			}
		}
		if item, ok := m.detailList.SelectedItem().(messageItem); ok {
			msg := item.message
			return "Message", []ui.InspectField{
				{Label: "Message ID", Value: ui.EmptyToDash(msg.MessageID)},
				{Label: "Enqueued At", Value: ui.FormatTime(msg.EnqueuedAt)},
				{Label: "Body Preview", Value: compactPreview(msg.BodyPreview, 80)},
			}
		}
		return "Detail", nil
	}
	return "", nil
}

// inspectFooterHeight returns the rendered row count of the inspect strip
// for the given pane (when toggled on), or 0 when off. Used by resize() to
// shrink the list height to make room for the strip.
func (m Model) inspectFooterHeight(pane int) int {
	if !m.inspectPanes[pane] {
		return 0
	}
	_, fields := m.inspectFor(pane)
	return ui.InspectStripHeight(fields)
}

// inspectFooter returns the rendered inspect strip for the pane (or "" when
// the toggle is off). Called from View() to populate ListPane.Footer.
func (m Model) inspectFooter(pane, contentWidth int) string {
	if !m.inspectPanes[pane] {
		return ""
	}
	title, fields := m.inspectFor(pane)
	return ui.RenderInspectStrip(title, fields, m.Styles, contentWidth)
}

// toggleInspect flips the inspect strip on/off for the focused pane.
func (m *Model) toggleInspect() {
	if m.inspectPanes == nil {
		m.inspectPanes = make(map[int]bool)
	}
	m.inspectPanes[m.focus] = !m.inspectPanes[m.focus]
	m.resize()
}
