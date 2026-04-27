package dashapp

import (
	"sort"
	"strings"

	"github.com/karlssonsimon/lazyaz/internal/azure"
	"github.com/karlssonsimon/lazyaz/internal/azure/servicebus"
	"github.com/karlssonsimon/lazyaz/internal/cache"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// usedSBWidget surfaces the Service Bus resources the user has touched
// most often in the current subscription. Reads from the persistent
// usage_stats table, scoped to m.CurrentSub.ID, mixing namespaces /
// queues / topics / topic-subscriptions.
type usedSBWidget struct{}

const (
	sbResourceNamespace = "sb_namespace"
	sbResourceQueue     = "sb_queue"
	sbResourceTopic     = "sb_topic"
	sbResourceTopicSub  = "sb_topic_sub"
)

// sbUsageTypes is the resource_type set this widget queries.
var sbUsageTypes = []string{sbResourceNamespace, sbResourceQueue, sbResourceTopic, sbResourceTopicSub}

func (usedSBWidget) Title() string        { return "Most used · Service Bus" }
func (usedSBWidget) Position() (int, int) { return 0, 1 }

func (w usedSBWidget) Context(m *Model, view widgetViewState) string {
	return "last 7d"
}

func (usedSBWidget) RowCount(m *Model, view widgetViewState) int {
	return len(m.usedSBEntries(view.filter))
}

func (w usedSBWidget) Render(m *Model, width, innerHeight, offset, cursor int, view widgetViewState) string {
	if !m.HasSubscription {
		return "Pick a subscription with " + m.Keymap.SubscriptionPicker.Short() + "."
	}
	entries := m.usedSBRows(view)
	if len(entries) == 0 {
		if view.filter != "" {
			return "No matches for filter: " + view.filter
		}
		return "Drill into a Service Bus namespace, queue, or topic to populate this list."
	}

	cells := [][]string{{"Kind", "Resource", "Uses"}}
	maxUse := int64(0)
	for _, e := range entries {
		if e.Count > maxUse {
			maxUse = e.Count
		}
	}
	for _, e := range entries {
		cells = append(cells, []string{
			m.Styles.Muted.Render(usageKindLabel(e.ResourceType)),
			e.Display,
			usageCountCell(e.Count, maxUse, m.Styles),
		})
	}
	aligns := []lipgloss.Position{lipgloss.Left, lipgloss.Left, lipgloss.Right}
	return renderScrollableTable(cells, aligns, m.Styles, offset, innerHeightToVisibleData(innerHeight), cursor)
}

func (usedSBWidget) SortFields() []SortField {
	return []SortField{
		{Label: "Uses"},
		{Label: "Resource"},
		{Label: "Kind"},
	}
}

func (w usedSBWidget) Actions(m *Model, cursorRow int) []Action {
	var actions []Action
	entries := m.usedSBRows(focusedWidgetView(m))
	if cursorRow >= 0 && cursorRow < len(entries) {
		e := entries[cursorRow]
		if cmd := openUsageEntryInSBCmd(m.CurrentSub, e); cmd != nil {
			actions = append(actions, Action{
				Label: "Open in Service Bus tab",
				Key:   "o",
				Cmd:   cmd,
			})
		}
	}
	actions = append(actions, sortAction())
	actions = append(actions, clearUsageAction(sbUsageTypes...))
	return actions
}

func (m Model) usedSBRows(view widgetViewState) []cache.UsageEntry {
	entries := m.usedSBEntries(view.filter)
	if view.hasSort {
		sortUsageEntries(entries, view.sortField, view.sortDesc)
	}
	return entries
}

// usedSBEntries pulls usage rows for every SB resource type, merges
// them client-side (one query per type), filters by substring, and
// returns the result sorted as the underlying TopUsage delivered each
// type (count desc, recency tiebreak). Sort overlays apply on top.
func (m Model) usedSBEntries(filter string) []cache.UsageEntry {
	if m.db == nil || !m.HasSubscription {
		return nil
	}
	var all []cache.UsageEntry
	for _, t := range sbUsageTypes {
		all = append(all, m.db.TopUsage(m.CurrentSub.ID, t, 100)...)
	}
	// Merged list: re-sort by count desc + recency desc so types
	// don't cluster.
	sortUsageEntries(all, 0, true)
	if filter == "" {
		return all
	}
	out := all[:0]
	for _, e := range all {
		if matchesFilter(e.Display, filter) || matchesFilter(usageKindLabel(e.ResourceType), filter) {
			out = append(out, e)
		}
	}
	return out
}

// usageKindLabel maps the internal resource_type to a human-readable
// "Kind" column value.
func usageKindLabel(t string) string {
	switch t {
	case sbResourceNamespace:
		return "namespace"
	case sbResourceQueue:
		return "queue"
	case sbResourceTopic:
		return "topic"
	case sbResourceTopicSub:
		return "topic-sub"
	case "blob_account":
		return "account"
	case "blob_container":
		return "container"
	}
	return t
}

// openUsageEntryInSBCmd builds the right cross-tab nav msg for the
// usage entry's kind. Resource keys carry "<sub>/<ns>/[entity[/sub]]"
// — split and dispatch. Returns nil if the key shape doesn't match
// the resource type (corrupt row, version mismatch, etc.).
func openUsageEntryInSBCmd(sub azure.Subscription, e cache.UsageEntry) tea.Cmd {
	parts := strings.Split(e.ResourceKey, "/")
	switch e.ResourceType {
	case sbResourceNamespace:
		if len(parts) < 2 {
			return nil
		}
		return openSBNamespaceCmd(sub, servicebus.Namespace{Name: parts[1]})
	case sbResourceQueue, sbResourceTopic:
		if len(parts) < 3 {
			return nil
		}
		return openSBEntityCmd(sub, servicebus.Namespace{Name: parts[1]}, parts[2], "", false)
	case sbResourceTopicSub:
		if len(parts) < 4 {
			return nil
		}
		// Topic-sub usage opens straight into DLQ — matches the DLQ
		// alerts widget's "fix the problem" flow, since topic-subs are
		// usually surfaced because something dead-lettered.
		return openSBEntityCmd(sub, servicebus.Namespace{Name: parts[1]}, parts[2], parts[3], true)
	}
	return nil
}

// sortUsageEntries sorts in place. Field 0 = uses (count), 1 = display,
// 2 = kind. Direction is baked into the comparator so tiebreakers
// don't flip with desc.
func sortUsageEntries(entries []cache.UsageEntry, field int, desc bool) {
	cmpStr := func(a, b string) bool {
		la, lb := strings.ToLower(a), strings.ToLower(b)
		if desc {
			return la > lb
		}
		return la < lb
	}
	cmpInt := func(a, b int64) bool {
		if desc {
			return a > b
		}
		return a < b
	}
	cmp := func(i, j int) bool {
		a, b := entries[i], entries[j]
		switch field {
		case 1:
			if a.Display != b.Display {
				return cmpStr(a.Display, b.Display)
			}
		case 2:
			if a.ResourceType != b.ResourceType {
				return cmpStr(a.ResourceType, b.ResourceType)
			}
		default:
			if a.Count != b.Count {
				return cmpInt(a.Count, b.Count)
			}
		}
		// Tiebreaker: recency desc, never flips with direction.
		return a.LastUsedAt > b.LastUsedAt
	}
	sort.SliceStable(entries, cmp)
}
