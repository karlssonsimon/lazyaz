package sbapp

import (
	"fmt"
	"strings"

	"github.com/karlssonsimon/lazyaz/internal/azure/servicebus"

	"charm.land/bubbles/v2/list"
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
	if i.entity.Kind == servicebus.EntityTopic {
		return fmt.Sprintf("▶ %s", i.entity.Name)
	}
	return fmt.Sprintf("☰ %s", i.entity.Name)
}

func (i entityItem) Description() string {
	return ""
}

func (i entityItem) FilterValue() string {
	return i.entity.Name
}

type subscriptionItem struct {
	sub servicebus.TopicSubscription
}

func (i subscriptionItem) Title() string {
	return i.sub.Name
}

func (i subscriptionItem) Description() string {
	return ""
}

func (i subscriptionItem) FilterValue() string {
	return i.sub.Name
}

type queueTypeItem struct {
	label      string
	deadLetter bool
	count      int64
}

func (i queueTypeItem) Title() string {
	return fmt.Sprintf("%s (%d)", i.label, i.count)
}

func (i queueTypeItem) Description() string { return "" }
func (i queueTypeItem) FilterValue() string { return i.label }

type messageItem struct {
	message   servicebus.PeekedMessage
	duplicate bool
}

func (i messageItem) Title() string {
	id := i.message.MessageID
	if id == "" {
		id = "(no id)"
	}
	prefix := ""
	if i.duplicate {
		prefix = "[DUP] "
	}

	ts := "    -     "
	if !i.message.EnqueuedAt.IsZero() {
		ts = i.message.EnqueuedAt.Local().Format("2006-01-02 15:04")
	}

	return fmt.Sprintf("%s%-40s  %s", prefix, id, ts)
}

func (i messageItem) Description() string {
	return ""
}

func (i messageItem) FilterValue() string {
	return i.message.MessageID + " " + i.message.BodyPreview
}

func compactPreview(s string, max int) string {
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

// entitiesToItems builds the flat list of entities for the entities pane,
// applying the current sort and optional DLQ filter.
func entitiesToItems(entities []servicebus.Entity, field entitySortField, desc bool, dlqFilter bool) []list.Item {
	ordered := sortAndFilterEntities(entities, field, desc, dlqFilter)
	items := make([]list.Item, 0, len(ordered))
	for _, e := range ordered {
		items = append(items, entityItem{entity: e})
	}
	return items
}

func subscriptionsToItems(subs []servicebus.TopicSubscription) []list.Item {
	items := make([]list.Item, 0, len(subs))
	for _, s := range subs {
		items = append(items, subscriptionItem{sub: s})
	}
	return items
}

func messagesToItems(messages []servicebus.PeekedMessage, duplicates map[string]struct{}) []list.Item {
	items := make([]list.Item, 0, len(messages))
	for _, msg := range messages {
		_, isDup := duplicates[msg.MessageID]
		items = append(items, messageItem{message: msg, duplicate: isDup})
	}
	return items
}

// Identity functions used by cache.Broker's internal merge and
// ui.SetItemsPreserveKey.

func namespaceKey(ns servicebus.Namespace) string       { return ns.Name }
func entityKey(e servicebus.Entity) string              { return e.Name }
func topicSubKey(s servicebus.TopicSubscription) string { return s.Name }

func messageItemKey(it list.Item) string {
	if mi, ok := it.(messageItem); ok {
		return mi.message.MessageID
	}
	return ""
}

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

func subscriptionItemKey(it list.Item) string {
	if si, ok := it.(subscriptionItem); ok {
		return si.sub.Name
	}
	return ""
}
