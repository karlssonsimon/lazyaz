package blobapp

import (
	"fmt"
	"strings"

	"github.com/karlssonsimon/lazyaz/internal/azure/blob"
	"github.com/karlssonsimon/lazyaz/internal/ui"

	"charm.land/bubbles/v2/list"
)

type accountItem struct {
	account blob.Account
}

func (i accountItem) Title() string {
	return i.account.Name
}

func (i accountItem) Description() string {
	return ""
}

func (i accountItem) FilterValue() string {
	return i.account.Name + " " + i.account.SubscriptionID + " " + i.account.ResourceGroup
}

type containerItem struct {
	container blob.ContainerInfo
}

func (i containerItem) Title() string {
	return i.container.Name
}

func (i containerItem) Description() string {
	return ""
}

func (i containerItem) FilterValue() string {
	return i.container.Name
}

type blobItem struct {
	blob        blob.BlobEntry
	displayName string
	marked      bool
	visual      bool
}

func (i blobItem) Title() string {
	if i.blob.IsPrefix {
		return "[DIR] " + i.displayName
	}
	return i.displayName
}

func (i blobItem) Description() string {
	if i.blob.IsPrefix {
		return ""
	}
	return fmt.Sprintf("%s  %s  %s",
		humanSize(i.blob.Size),
		ui.EmptyToDash(i.blob.AccessTier),
		ui.FormatTime(i.blob.LastModified),
	)
}

func (i blobItem) FilterValue() string {
	return i.blob.Name
}

func accountsToItems(accounts []blob.Account) []list.Item {
	items := make([]list.Item, 0, len(accounts))
	for _, account := range accounts {
		items = append(items, accountItem{account: account})
	}
	return items
}

func containersToItems(containers []blob.ContainerInfo) []list.Item {
	items := make([]list.Item, 0, len(containers))
	for _, containerInfo := range containers {
		items = append(items, containerItem{container: containerInfo})
	}
	return items
}

func blobsToItems(entries []blob.BlobEntry, prefix string, marked map[string]blob.BlobEntry, visual map[string]struct{}) []list.Item {
	items := make([]list.Item, 0, len(entries))
	for _, entry := range entries {
		items = append(items, blobItem{
			blob:        entry,
			displayName: trimPrefixForDisplay(entry.Name, prefix),
			marked:      isBlobMarked(marked, entry.Name),
			visual:      isBlobVisualSelected(visual, entry.Name),
		})
	}
	return items
}

func isBlobMarked(marked map[string]blob.BlobEntry, blobName string) bool {
	if len(marked) == 0 {
		return false
	}
	_, ok := marked[blobName]
	return ok
}

func isBlobVisualSelected(visual map[string]struct{}, blobName string) bool {
	if len(visual) == 0 {
		return false
	}
	_, ok := visual[blobName]
	return ok
}

func trimPrefixForDisplay(name, prefix string) string {
	if prefix == "" {
		return name
	}
	trimmed := strings.TrimPrefix(name, prefix)
	if trimmed == "" {
		return name
	}
	return trimmed
}

func parentPrefix(prefix string) string {
	prefix = strings.TrimSuffix(prefix, "/")
	idx := strings.LastIndex(prefix, "/")
	if idx < 0 {
		return ""
	}
	return prefix[:idx+1]
}

// Identity functions used by cache.FetchSession and
// ui.SetItemsPreserveKey. Blob prefixes (synthetic "folder" entries in
// hierarchy mode) use the same Name field as real blobs, so a single
// keyer handles both cases.

func accountKey(a blob.Account) string       { return a.Name }
func containerKey(c blob.ContainerInfo) string { return c.Name }
func blobEntryKey(b blob.BlobEntry) string   { return b.Name }

func accountItemKey(it list.Item) string {
	if ai, ok := it.(accountItem); ok {
		return ai.account.Name
	}
	return ""
}

func containerItemKey(it list.Item) string {
	if ci, ok := it.(containerItem); ok {
		return ci.container.Name
	}
	return ""
}

func blobItemKey(it list.Item) string {
	if bi, ok := it.(blobItem); ok {
		return bi.blob.Name
	}
	return ""
}

func blobSearchPrefix(currentPrefix, query string) string {
	needle := strings.TrimSpace(strings.ReplaceAll(query, "\\", "/"))
	if needle == "" {
		return strings.TrimSpace(currentPrefix)
	}
	if strings.HasPrefix(needle, "/") {
		return strings.TrimPrefix(needle, "/")
	}
	base := strings.TrimSpace(currentPrefix)
	if base == "" || strings.HasPrefix(needle, base) {
		return needle
	}
	return base + needle
}
