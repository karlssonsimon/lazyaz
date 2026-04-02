package blobapp

import (
	"fmt"
	"strings"

	"azure-storage/internal/azure/blob"
	"azure-storage/internal/ui"

	"github.com/charmbracelet/bubbles/list"
)

type accountItem struct {
	account blob.Account
}

func (i accountItem) Title() string {
	return i.account.Name
}

func (i accountItem) Description() string {
	shortSub := i.account.SubscriptionID
	if len(shortSub) > 8 {
		shortSub = shortSub[:8]
	}
	if i.account.ResourceGroup == "" {
		return fmt.Sprintf("sub %s", shortSub)
	}
	return fmt.Sprintf("sub %s | rg %s", shortSub, i.account.ResourceGroup)
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
	if i.container.LastModified.IsZero() {
		return "-"
	}
	return ui.FormatTime(i.container.LastModified)
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
	prefix := "   "
	if i.visual {
		prefix = ">  "
	}
	if i.marked {
		if i.visual {
			prefix = ">* "
		} else {
			prefix = "*  "
		}
	}

	if i.blob.IsPrefix {
		if i.visual {
			return "> [DIR] " + i.displayName
		}
		return "  [DIR] " + i.displayName
	}

	return prefix + i.displayName
}

func (i blobItem) Description() string {
	if i.blob.IsPrefix {
		return ""
	}
	return fmt.Sprintf("%s | %s | %s", humanSize(i.blob.Size), ui.FormatTime(i.blob.LastModified), ui.EmptyToDash(i.blob.AccessTier))
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
