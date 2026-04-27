package dashapp

import (
	"fmt"
	"strings"

	"github.com/karlssonsimon/lazyaz/internal/azure"
	"github.com/karlssonsimon/lazyaz/internal/cache"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// usedBlobWidget surfaces the Blob storage resources the user has
// touched most often in the current subscription. Open-in-tab
// navigation is deferred — for now this is a read-only "where do I
// usually go" cheat-sheet.
type usedBlobWidget struct{}

const (
	blobResourceAccount   = "blob_account"
	blobResourceContainer = "blob_container"
)

var blobUsageTypes = []string{blobResourceAccount, blobResourceContainer}

func (usedBlobWidget) Title() string        { return "Most used Blob" }
func (usedBlobWidget) Position() (int, int) { return 1, 1 }

func (usedBlobWidget) RowCount(m *Model, view widgetViewState) int {
	return len(m.usedBlobEntries(view.filter))
}

func (w usedBlobWidget) Render(m *Model, width, innerHeight, offset, cursor int, view widgetViewState) string {
	if !m.HasSubscription {
		return "Pick a subscription with " + m.Keymap.SubscriptionPicker.Short() + "."
	}
	entries := m.usedBlobRows(view)
	if len(entries) == 0 {
		if view.filter != "" {
			return "No matches for filter: " + view.filter
		}
		return "Drill into a Blob account or container to populate this list."
	}

	cells := [][]string{{"Kind", "Resource", "Uses"}}
	for _, e := range entries {
		cells = append(cells, []string{usageKindLabel(e.ResourceType), e.Display, fmt.Sprintf("%d", e.Count)})
	}
	aligns := []lipgloss.Position{lipgloss.Left, lipgloss.Left, lipgloss.Right}
	return renderScrollableTable(cells, aligns, m.Styles, offset, innerHeightToVisibleData(innerHeight), cursor)
}

func (usedBlobWidget) SortFields() []SortField {
	return []SortField{
		{Label: "Uses"},
		{Label: "Resource"},
		{Label: "Kind"},
	}
}

func (w usedBlobWidget) Actions(m *Model, cursorRow int) []Action {
	var actions []Action
	entries := m.usedBlobRows(focusedWidgetView(m))
	if cursorRow >= 0 && cursorRow < len(entries) {
		if cmd := openUsageEntryInBlobCmd(m.CurrentSub, entries[cursorRow]); cmd != nil {
			actions = append(actions, Action{
				Label: "Open in Blob tab",
				Key:   "o",
				Cmd:   cmd,
			})
		}
	}
	actions = append(actions, sortAction())
	actions = append(actions, clearUsageAction(blobUsageTypes...))
	return actions
}

func (m Model) usedBlobRows(view widgetViewState) []cache.UsageEntry {
	entries := m.usedBlobEntries(view.filter)
	if view.hasSort {
		sortUsageEntries(entries, view.sortField, view.sortDesc)
	}
	return entries
}

// openUsageEntryInBlobCmd builds the right cross-tab nav msg for a
// blob usage row. Resource keys are "<sub>/<account>[/<container>]";
// returns nil when the key shape doesn't match the resource type.
func openUsageEntryInBlobCmd(sub azure.Subscription, e cache.UsageEntry) tea.Cmd {
	parts := strings.Split(e.ResourceKey, "/")
	switch e.ResourceType {
	case blobResourceAccount:
		if len(parts) < 2 {
			return nil
		}
		return openBlobAccountCmd(sub, parts[1])
	case blobResourceContainer:
		if len(parts) < 3 {
			return nil
		}
		return openBlobContainerCmd(sub, parts[1], parts[2])
	}
	return nil
}

// usedBlobEntries pulls usage rows for blob_account + blob_container,
// merges, sorts by count desc + recency, applies optional filter.
func (m Model) usedBlobEntries(filter string) []cache.UsageEntry {
	if m.db == nil || !m.HasSubscription {
		return nil
	}
	var all []cache.UsageEntry
	for _, t := range blobUsageTypes {
		all = append(all, m.db.TopUsage(m.CurrentSub.ID, t, 100)...)
	}
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
