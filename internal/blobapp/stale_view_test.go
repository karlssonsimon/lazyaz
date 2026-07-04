package blobapp

import (
	"testing"

	"github.com/karlssonsimon/lazyaz/internal/azure/blob"
)

// Descending into a folder that isn't cached must clear the blobs pane
// instead of showing the parent folder's rows until the fetch lands.
func TestFolderDescendClearsStaleRowsOnCacheMiss(t *testing.T) {
	m := NewModel(nil, testConfig, nil)
	m.SubOverlay.Close()
	m.DismissSpinner(m.LoadingSpinnerID)
	m.ClearLoading()
	m.Width, m.Height = 100, 40
	m.hasAccount = true
	m.currentAccount = blob.Account{Name: "acct", SubscriptionID: "sub-1"}
	m.hasContainer = true
	m.containerName = "data"
	m.focus = blobsPane
	m.blobs = []blob.BlobEntry{
		{Name: "logs/", IsPrefix: true},
		{Name: "a.txt"},
		{Name: "b.txt"},
	}
	m.resize()
	m.refreshItems()
	m.blobsList.Title = "Blobs (3)"
	m.blobsList.Select(0) // cursor on the logs/ folder

	updated, _ := m.handleEnter()

	if updated.prefix != "logs/" {
		t.Fatalf("prefix = %q, want logs/", updated.prefix)
	}
	if got := len(updated.blobsList.Items()); got != 0 {
		t.Fatalf("blobs pane shows %d stale rows after descending into uncached folder, want 0", got)
	}
	if updated.blobsList.Title != "Blobs" {
		t.Fatalf("title = %q, want bare Blobs while loading", updated.blobsList.Title)
	}
}
