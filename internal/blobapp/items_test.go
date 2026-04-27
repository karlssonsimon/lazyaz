package blobapp

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/karlssonsimon/lazyaz/internal/azure/blob"
	"github.com/karlssonsimon/lazyaz/internal/ui"
)

func TestBlobItemTitleFitsContentWidth(t *testing.T) {
	it := blobItem{
		blob: blob.BlobEntry{
			Name:         "reports/very/long/file-name-that-must-truncate.csv",
			Size:         123456,
			LastModified: time.Date(2026, 4, 26, 12, 30, 0, 0, time.UTC),
		},
		displayName:  "very/long/file-name-that-must-truncate.csv",
		contentWidth: 42,
	}
	title := it.Title()
	if got := lipgloss.Width(title); got > 42 {
		t.Fatalf("title width = %d, want <= 42: %q", got, title)
	}
	if !strings.Contains(title, "2026-04-26") {
		t.Fatalf("modified date missing: %q", title)
	}
	if !strings.Contains(title, "123.46 KB") {
		t.Fatalf("size missing: %q", title)
	}
}

func TestBlobPrefixTitleFitsContentWidth(t *testing.T) {
	it := blobItem{
		blob:         blob.BlobEntry{Name: "folder-name-that-is-too-long/", IsPrefix: true},
		displayName:  "folder-name-that-is-too-long/",
		contentWidth: 18,
	}
	if got := lipgloss.Width(it.Title()); got > 18 {
		t.Fatalf("prefix title width = %d, want <= 18: %q", got, it.Title())
	}
}

func TestBlobItemTitleWithWideRunesPreservesMetadata(t *testing.T) {
	it := blobItem{
		blob: blob.BlobEntry{
			Name:         "界界界界界界界界界界界界界界.csv",
			Size:         123456,
			LastModified: time.Date(2026, 4, 26, 12, 30, 0, 0, time.UTC),
		},
		displayName:  "界界界界界界界界界界界界界界.csv",
		contentWidth: 44,
	}
	title := it.Title()
	if got := lipgloss.Width(title); got > 44 {
		t.Fatalf("title width = %d, want <= 44: %q", got, title)
	}
	if !strings.Contains(title, "2026-04-26") {
		t.Fatalf("modified date missing: %q", title)
	}
	if !strings.Contains(title, "123.46 KB") {
		t.Fatalf("size missing: %q", title)
	}
}

func TestBlobItemTitlePreservesLargeSizeMetadataInNarrowRow(t *testing.T) {
	it := blobItem{
		blob: blob.BlobEntry{
			Name:         "large-report.csv",
			Size:         123456789000000,
			LastModified: time.Date(2026, 4, 26, 12, 30, 0, 0, time.UTC),
		},
		displayName:  "large-report.csv",
		contentWidth: 36,
	}
	title := it.Title()
	if got := lipgloss.Width(title); got > 36 {
		t.Fatalf("title width = %d, want <= 36: %q", got, title)
	}
	if !strings.Contains(title, "2026-04-26") {
		t.Fatalf("modified date missing: %q", title)
	}
	if !strings.Contains(title, "123.46 TB") {
		t.Fatalf("size missing: %q", title)
	}
}

func TestResizeRefreshesBlobItemContentWidth(t *testing.T) {
	m := NewModel(nil, testConfig, nil)
	m.Width = 160
	m.Height = 40
	m.hasAccount = true
	m.hasContainer = true
	m.focus = blobsPane
	m.blobs = []blob.BlobEntry{{Name: "reports/file.csv"}}
	m.blobsList.SetItems(blobsToItems(m.blobs, m.prefix, 12))

	m.resize()

	items := m.blobsList.Items()
	if len(items) != 1 {
		t.Fatalf("items len = %d, want 1", len(items))
	}
	got := items[0].(blobItem).contentWidth
	// Layout reserves all three slots (parent / focused / child), so
	// blobsPane is no longer the rightmost visible column even with
	// the preview closed — its right rule separates blobs from the
	// reserved-but-empty preview slot.
	want := ui.MillerContentWidth(ui.MillerColumnFrame{Width: m.paneWidths[blobsPane], RightRule: true})
	if got != want {
		t.Fatalf("blob item contentWidth = %d, want refreshed width %d", got, want)
	}
}

func TestResizeRefreshesParentBlobItemContentWidth(t *testing.T) {
	m := NewModel(nil, testConfig, nil)
	m.Width = 160
	m.Height = 40
	m.hasAccount = true
	m.hasContainer = true
	m.focus = blobsPane
	m.prefix = "reports/"
	m.blobs = []blob.BlobEntry{{Name: "reports/file.csv"}}
	m.parentBlobsList.SetItems(blobsToItems(m.blobs, "", 12))

	m.resize()

	items := m.parentBlobsList.Items()
	if len(items) != 1 {
		t.Fatalf("items len = %d, want 1", len(items))
	}
	got := items[0].(blobItem).contentWidth
	want := ui.MillerContentWidth(ui.MillerColumnFrame{Width: m.paneWidths[containersPane], RightRule: true})
	if got != want {
		t.Fatalf("parent blob item contentWidth = %d, want refreshed width %d", got, want)
	}
}

func TestBlobItemTitleVeryNarrowWidths(t *testing.T) {
	for width := 1; width <= 6; width++ {
		t.Run(fmt.Sprintf("width_%d", width), func(t *testing.T) {
			it := blobItem{
				blob:         blob.BlobEntry{Name: "界界界界界.csv", Size: 123456, LastModified: time.Date(2026, 4, 26, 12, 30, 0, 0, time.UTC)},
				displayName:  "界界界界界.csv",
				contentWidth: width,
			}
			if got := lipgloss.Width(it.Title()); got > width {
				t.Fatalf("title width = %d, want <= %d: %q", got, width, it.Title())
			}
		})
	}
}
