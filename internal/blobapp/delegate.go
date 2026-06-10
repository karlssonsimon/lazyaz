package blobapp

import (
	"charm.land/bubbles/v2/list"
)

// blobMarkKey extracts the mark/visual lookup key for ui.MarkDelegate
// from a blobs-list item.
func blobMarkKey(item list.Item) (string, bool) {
	b, ok := item.(blobItem)
	if !ok {
		return "", false
	}
	return b.blob.Name, true
}
