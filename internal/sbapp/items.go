package sbapp

import (
	"fmt"
	"strings"

	"azure-storage/internal/azure/servicebus"
	"azure-storage/internal/ui"

	"github.com/charmbracelet/bubbles/list"
)

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
	return id
}

func (i messageItem) Description() string {
	enqueued := ui.FormatTime(i.message.EnqueuedAt)
	preview := compactPreview(i.message.BodyPreview, 50)
	if preview == "" {
		return enqueued
	}
	return fmt.Sprintf("%s | %s", enqueued, preview)
}

func (i messageItem) FilterValue() string {
	return i.message.MessageID + " " + i.message.BodyPreview
}

func compactPreview(s string, max int) string {
	// Collapse whitespace (newlines, tabs, runs of spaces) into single spaces.
	var b strings.Builder
	inSpace := false
	for _, r := range s {
		if r == '\n' || r == '\r' || r == '\t' || r == ' ' {
			if !inSpace && b.Len() > 0 {
				b.WriteByte(' ')
			}
			inSpace = true
			continue
		}
		inSpace = false
		b.WriteRune(r)
	}
	out := strings.TrimSpace(b.String())
	if len(out) > max {
		return out[:max] + "..."
	}
	return out
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

func entitiesToFilteredItems(entities []servicebus.Entity, dlqOnly bool) []list.Item {
	items := make([]list.Item, 0, len(entities))
	for _, e := range entities {
		if dlqOnly && e.DeadLetterCount == 0 {
			continue
		}
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
