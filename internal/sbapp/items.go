package sbapp

import (
	"fmt"
	"strings"

	"azure-storage/internal/azure/servicebus"

	"github.com/charmbracelet/bubbles/list"
)

type namespaceItem struct {
	namespace servicebus.Namespace
}

func (i namespaceItem) Title() string {
	return i.namespace.Name
}

func (i namespaceItem) Description() string {
	return ""
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
	return ""
}

func (i entityItem) FilterValue() string {
	return i.entity.Name
}

type topicSubItem struct {
	sub servicebus.TopicSubscription
}

func (i topicSubItem) Title() string { return i.sub.Name }
func (i topicSubItem) Description() string {
	return ""
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
	return ""
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

// Identity functions used by cache.FetchSession and
// ui.SetItemsPreserveKey. Messages deliberately opt out — they're
// ephemeral peek results and go through plain replace semantics.

func namespaceKey(ns servicebus.Namespace) string   { return ns.Name }
func entityKey(e servicebus.Entity) string          { return e.Name }
func topicSubKey(s servicebus.TopicSubscription) string { return s.Name }

func namespaceItemKey(it list.Item) string {
	if ni, ok := it.(namespaceItem); ok {
		return ni.namespace.Name
	}
	return ""
}

func entityItemKey(it list.Item) string {
	if ei, ok := it.(entityItem); ok {
		return ei.entity.Name
	}
	return ""
}

func topicSubItemKey(it list.Item) string {
	if ti, ok := it.(topicSubItem); ok {
		return ti.sub.Name
	}
	return ""
}
