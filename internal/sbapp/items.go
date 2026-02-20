package sbapp

import (
	"fmt"
	"strings"

	"azure-storage/internal/azure"
	"azure-storage/internal/servicebus"
	"azure-storage/internal/ui"

	"github.com/charmbracelet/bubbles/list"
)

type subscriptionItem struct {
	subscription azure.Subscription
}

func (i subscriptionItem) Title() string {
	if strings.TrimSpace(i.subscription.Name) != "" {
		return i.subscription.Name
	}
	return i.subscription.ID
}

func (i subscriptionItem) Description() string {
	id := i.subscription.ID
	if len(id) > 12 {
		id = id[:12]
	}
	state := strings.TrimSpace(i.subscription.State)
	if state == "" {
		return fmt.Sprintf("id %s", id)
	}
	return fmt.Sprintf("%s | id %s", state, id)
}

func (i subscriptionItem) FilterValue() string {
	return i.subscription.Name + " " + i.subscription.ID + " " + i.subscription.State
}

type namespaceItem struct {
	namespace servicebus.Namespace
}

func (i namespaceItem) Title() string {
	return i.namespace.Name
}

func (i namespaceItem) Description() string {
	shortSub := i.namespace.SubscriptionID
	if len(shortSub) > 8 {
		shortSub = shortSub[:8]
	}
	if i.namespace.ResourceGroup == "" {
		return fmt.Sprintf("sub %s", shortSub)
	}
	return fmt.Sprintf("sub %s | rg %s", shortSub, i.namespace.ResourceGroup)
}

func (i namespaceItem) FilterValue() string {
	return i.namespace.Name + " " + i.namespace.SubscriptionID + " " + i.namespace.ResourceGroup
}

type entityItem struct {
	entity servicebus.Entity
}

func (i entityItem) Title() string {
	tag := "[Q]"
	if i.entity.Kind == servicebus.EntityTopic {
		tag = "[T]"
	}
	return fmt.Sprintf("%s %s", tag, i.entity.Name)
}

func (i entityItem) Description() string {
	kind := "queue"
	if i.entity.Kind == servicebus.EntityTopic {
		kind = "topic"
	}
	return fmt.Sprintf("%s · active: %d · dlq: %d", kind, i.entity.ActiveMsgCount, i.entity.DeadLetterCount)
}

func (i entityItem) FilterValue() string {
	return i.entity.Name
}

type topicSubItem struct {
	sub servicebus.TopicSubscription
}

func (i topicSubItem) Title() string { return i.sub.Name }
func (i topicSubItem) Description() string {
	return fmt.Sprintf("active: %d · dlq: %d", i.sub.ActiveMsgCount, i.sub.DeadLetterCount)
}
func (i topicSubItem) FilterValue() string { return i.sub.Name }

type messageItem struct {
	message   servicebus.PeekedMessage
	marked    bool
	duplicate bool
}

func (i messageItem) Title() string {
	id := i.message.MessageID
	if id == "" {
		id = "(no id)"
	}
	if i.duplicate {
		return "[DUP] " + id
	}
	if i.marked {
		return "* " + id
	}
	return "  " + id
}

func (i messageItem) Description() string {
	enqueued := ui.FormatTime(i.message.EnqueuedAt)
	preview := i.message.BodyPreview
	if len(preview) > 80 {
		preview = preview[:80] + "..."
	}
	if preview == "" {
		return enqueued
	}
	return fmt.Sprintf("%s | %s", enqueued, preview)
}

func (i messageItem) FilterValue() string {
	return i.message.MessageID + " " + i.message.BodyPreview
}

func subscriptionsToItems(subs []azure.Subscription) []list.Item {
	items := make([]list.Item, 0, len(subs))
	for _, s := range subs {
		items = append(items, subscriptionItem{subscription: s})
	}
	return items
}

func namespacesToItems(namespaces []servicebus.Namespace) []list.Item {
	items := make([]list.Item, 0, len(namespaces))
	for _, ns := range namespaces {
		items = append(items, namespaceItem{namespace: ns})
	}
	return items
}

func entitiesToItems(entities []servicebus.Entity) []list.Item {
	items := make([]list.Item, 0, len(entities))
	for _, e := range entities {
		items = append(items, entityItem{entity: e})
	}
	return items
}

func topicSubsToItems(subs []servicebus.TopicSubscription) []list.Item {
	items := make([]list.Item, 0, len(subs))
	for _, s := range subs {
		items = append(items, topicSubItem{sub: s})
	}
	return items
}

func messagesToItems(messages []servicebus.PeekedMessage, marked, duplicates map[string]struct{}) []list.Item {
	items := make([]list.Item, 0, len(messages))
	for _, msg := range messages {
		_, isMarked := marked[msg.MessageID]
		_, isDup := duplicates[msg.MessageID]
		items = append(items, messageItem{message: msg, marked: isMarked, duplicate: isDup})
	}
	return items
}
