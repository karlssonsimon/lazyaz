package sbapp

import (
	"fmt"
	"strings"

	"github.com/karlssonsimon/lazyaz/internal/azure/servicebus"

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

// entityItem represents a top-level entity in the entities pane —
// either a queue (which has messages directly) or a topic (which has
// child subscriptions). Topics can be expanded in place via expanded:
// when true the children are rendered as topicSubChildItem rows
// immediately after the topic in the same list.
type entityItem struct {
	entity   servicebus.Entity
	expanded bool // only meaningful for topics
}

func (i entityItem) Title() string {
	if i.entity.Kind == servicebus.EntityTopic {
		marker := "▶"
		if i.expanded {
			marker = "▼"
		}
		return fmt.Sprintf("%s %s", marker, i.entity.Name)
	}
	return fmt.Sprintf("☰ %s", i.entity.Name)
}

func (i entityItem) Description() string {
	return ""
}

func (i entityItem) FilterValue() string {
	return i.entity.Name
}

// topicSubChildItem is a topic subscription rendered as an indented
// child row under its parent topic in the entities pane. Selecting
// one peeks the subscription's messages — the same flow as selecting
// a queue, just one level deeper in the tree.
type topicSubChildItem struct {
	parentTopic string
	sub         servicebus.TopicSubscription
}

func (i topicSubChildItem) Title() string {
	return fmt.Sprintf("    ↳ %s", i.sub.Name)
}

func (i topicSubChildItem) Description() string {
	return ""
}

func (i topicSubChildItem) FilterValue() string {
	return i.sub.Name
}

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

// entitiesTreeToItems flattens the entities + per-topic-subs into the
// list items the entities pane renders. Topics with expandedTopics[name]
// true are followed immediately by their topicSubChildItem children.
// Queues are always leaves.
//
// filter selects which entity kinds to include (all, queues only, or
// topics only). When dlqSort is true, entities with at least one DLQ
// message are pulled to the top of the (filtered) list in descending
// DLQ-count order; the rest follow in their original order.
func entitiesTreeToItems(
	entities []servicebus.Entity,
	subsByTopic map[string][]servicebus.TopicSubscription,
	expandedTopics map[string]bool,
	filter entityFilterMode,
	dlqSort bool,
) []list.Item {
	// Apply type filter first.
	filtered := make([]servicebus.Entity, 0, len(entities))
	for _, e := range entities {
		if !filter.matches(e) {
			continue
		}
		filtered = append(filtered, e)
	}

	ordered := filtered
	if dlqSort {
		ordered = sortEntitiesByDLQDesc(filtered)
	}

	items := make([]list.Item, 0, len(ordered))
	for _, e := range ordered {
		expanded := e.Kind == servicebus.EntityTopic && expandedTopics[e.Name]
		items = append(items, entityItem{entity: e, expanded: expanded})
		if expanded {
			for _, s := range subsByTopic[e.Name] {
				items = append(items, topicSubChildItem{parentTopic: e.Name, sub: s})
			}
		}
	}
	return items
}

// matches reports whether an entity passes the current filter mode.
func (f entityFilterMode) matches(e servicebus.Entity) bool {
	switch f {
	case entityFilterQueues:
		return e.Kind == servicebus.EntityQueue
	case entityFilterTopics:
		return e.Kind == servicebus.EntityTopic
	default:
		return true
	}
}

// sortEntitiesByDLQDesc returns a copy of entities with DLQ-bearing
// entries pulled to the front, sorted by DeadLetterCount desc. Entries
// with zero DLQ retain their original relative order. Stable so the
// fallback ordering is predictable.
func sortEntitiesByDLQDesc(entities []servicebus.Entity) []servicebus.Entity {
	out := make([]servicebus.Entity, len(entities))
	copy(out, entities)
	// Use a simple insertion-stable approach: bubble entries with DLQ
	// to the top, sorted by count desc among themselves.
	// For typical entity counts (<1000) this is fine.
	withDLQ := make([]servicebus.Entity, 0, len(out))
	withoutDLQ := make([]servicebus.Entity, 0, len(out))
	for _, e := range out {
		if e.DeadLetterCount > 0 {
			withDLQ = append(withDLQ, e)
		} else {
			withoutDLQ = append(withoutDLQ, e)
		}
	}
	// Stable sort withDLQ by DLQ count desc (insertion sort, stable).
	for i := 1; i < len(withDLQ); i++ {
		for j := i; j > 0 && withDLQ[j-1].DeadLetterCount < withDLQ[j].DeadLetterCount; j-- {
			withDLQ[j-1], withDLQ[j] = withDLQ[j], withDLQ[j-1]
		}
	}
	return append(withDLQ, withoutDLQ...)
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
// ui.SetItemsPreserveKey. Messages have a key too — used by re-peek
// after requeue/delete to keep the cursor on the same message instead
// of jumping back to the top.

func namespaceKey(ns servicebus.Namespace) string       { return ns.Name }
func entityKey(e servicebus.Entity) string              { return e.Name }
func topicSubKey(s servicebus.TopicSubscription) string { return s.Name }

func messageItemKey(it list.Item) string {
	if mi, ok := it.(messageItem); ok {
		return mi.message.MessageID
	}
	return ""
}

// entitiesTreeItemKey is the identity used by ui.SetItemsPreserveKey
// for the tree-shaped entities list. Topic-sub child rows include the
// parent topic name in the key so that the cursor doesn't jump from
// "topic-A → sub-1" to "topic-B → sub-1" if both have a sub by that
// name and the relative position lines up.
func entitiesTreeItemKey(it list.Item) string {
	switch v := it.(type) {
	case entityItem:
		return "e:" + v.entity.Name
	case topicSubChildItem:
		return "s:" + v.parentTopic + "/" + v.sub.Name
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
