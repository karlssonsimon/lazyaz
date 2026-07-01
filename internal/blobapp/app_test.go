package blobapp

import (
	"fmt"
	"strings"
	"testing"

	"github.com/karlssonsimon/lazyaz/internal/azure"
	"github.com/karlssonsimon/lazyaz/internal/azure/blob"
	"github.com/karlssonsimon/lazyaz/internal/cache"
	"github.com/karlssonsimon/lazyaz/internal/ui"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

var testConfig = ui.Config{
	ThemeName: "fallback",
	Schemes:   []ui.Scheme{ui.FallbackScheme()},
}

// TestDeleteFolderActionAvailableOnBothAccountTypes regresses a bug
// where "Delete folder..." was hidden on flat-namespace (non-HNS)
// accounts, leaving virtual folders un-deletable through the UI. The
// service-level DeleteDirectory now lists+batch-deletes for flat
// accounts, so the action should surface in both cases.
func TestDeleteFolderActionAvailableOnBothAccountTypes(t *testing.T) {
	cases := []struct {
		name       string
		hnsEnabled bool
		wantRename bool
		wantCreate bool
	}{
		{name: "HNS account", hnsEnabled: true, wantRename: true, wantCreate: true},
		{name: "flat-namespace account", hnsEnabled: false, wantRename: false, wantCreate: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := NewModel(nil, testConfig, nil)
			m.SubOverlay.Close()
			m.hasAccount = true
			m.hasContainer = true
			m.focus = blobsPane
			m.currentAccount.IsHnsEnabled = tc.hnsEnabled
			m.blobs = []blob.BlobEntry{{Name: "reports/", IsPrefix: true}}
			m.blobsList.SetItems(blobsToItems(m.blobs, m.prefix, 12))

			labels := make(map[string]bool)
			for _, a := range m.buildActions() {
				labels[a.label] = true
			}
			if !labels["Delete folder..."] {
				t.Fatal("Delete folder... should be available on virtual folder cursor regardless of HNS")
			}
			if labels["Rename folder..."] != tc.wantRename {
				t.Fatalf("Rename folder... presence = %v, want %v (HNS-only)", labels["Rename folder..."], tc.wantRename)
			}
			if labels["Create folder..."] != tc.wantCreate {
				t.Fatalf("Create folder... presence = %v, want %v (HNS-only)", labels["Create folder..."], tc.wantCreate)
			}
		})
	}
}

// TestDeleteFolderActionHiddenOnFileCursor confirms the folder-delete
// action only surfaces when the cursor is on a virtual folder, not a
// regular blob — preventing accidental whole-folder semantics on a
// single-file cursor.
func TestDeleteFolderActionHiddenOnFileCursor(t *testing.T) {
	m := NewModel(nil, testConfig, nil)
	m.SubOverlay.Close()
	m.hasAccount = true
	m.hasContainer = true
	m.focus = blobsPane
	m.currentAccount.IsHnsEnabled = false
	m.blobs = []blob.BlobEntry{{Name: "report.csv", IsPrefix: false}}
	m.blobsList.SetItems(blobsToItems(m.blobs, m.prefix, 12))

	for _, a := range m.buildActions() {
		if a.label == "Delete folder..." {
			t.Fatal("Delete folder... should NOT appear when cursor is on a blob, not a folder")
		}
	}
}

// TestIsTextInputActiveTrueForFuzzyFilterOverlays guards against a class
// of regressions where the parent tabapp eats keys (q→quit, 1–9→tab-jump)
// while an overlay is open that fuzzy-filters typed characters. The sort
// overlay's options are number-prefixed, so this surfaced as 1/2/3
// jumping tabs instead of selecting an option.
func TestIsTextInputActiveTrueForFuzzyFilterOverlays(t *testing.T) {
	m := NewModel(nil, testConfig, nil)
	m.SubOverlay.Close()
	if m.IsTextInputActive() {
		t.Fatal("normal mode should not be text input")
	}
	m.actionMenu.open([]action{{label: "Upload"}})
	if !m.IsTextInputActive() {
		t.Fatal("action menu open: want text input active")
	}
	m.actionMenu.close()
	m.sortOverlay.open(blobSortNone, false)
	if !m.IsTextInputActive() {
		t.Fatal("sort overlay open: want text input active")
	}
}

func TestBlobHelpDescribesMillerColumns(t *testing.T) {
	m := NewModel(nil, ui.Config{ThemeName: "fallback", Schemes: []ui.Scheme{ui.FallbackScheme()}}, nil)
	sections := m.HelpSections()
	joined := fmt.Sprint(sections)
	if !strings.Contains(joined, "column") || !strings.Contains(joined, "filter focused column") {
		t.Fatalf("help does not describe Miller column navigation: %v", sections)
	}
	if helpHasBlankGoUpBack(sections) || !strings.Contains(joined, "backspace  go up/back") {
		t.Fatalf("help must bind go up/back to backspace without blank entries: %v", sections)
	}
}

func helpHasBlankGoUpBack(sections []ui.HelpSection) bool {
	for _, section := range sections {
		for _, item := range section.Items {
			if strings.HasPrefix(item, "  go up/back") {
				return true
			}
		}
	}
	return false
}

func TestParentPrefix(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		output string
	}{
		{name: "root", input: "", output: ""},
		{name: "single folder", input: "foo/", output: ""},
		{name: "nested", input: "foo/bar/", output: "foo/"},
		{name: "nested without trailing slash", input: "foo/bar", output: "foo/"},
		{name: "deep", input: "foo/bar/baz/", output: "foo/bar/"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := parentPrefix(tc.input); got != tc.output {
				t.Fatalf("expected %q, got %q", tc.output, got)
			}
		})
	}
}

func TestTrimPrefixForDisplay(t *testing.T) {
	tests := []struct {
		name   string
		value  string
		prefix string
		want   string
	}{
		{name: "no prefix", value: "folder/file.txt", prefix: "", want: "folder/file.txt"},
		{name: "with prefix", value: "folder/file.txt", prefix: "folder/", want: "file.txt"},
		{name: "same as prefix", value: "folder/", prefix: "folder/", want: "folder/"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := trimPrefixForDisplay(tc.value, tc.prefix); got != tc.want {
				t.Fatalf("expected %q, got %q", tc.want, got)
			}
		})
	}
}

func TestSetSubscriptionAllowsNilServiceWithTenant(t *testing.T) {
	m := NewModel(nil, testConfig, nil)
	if m.service == nil {
		t.Fatalf("NewModel(nil) left service nil")
	}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("SetSubscription panicked with nil service: %v", r)
		}
	}()
	m.SetSubscription(azure.Subscription{ID: "sub", TenantID: "tenant"})
}

func TestBlobSearchPrefix(t *testing.T) {
	tests := []struct {
		name          string
		currentPrefix string
		query         string
		want          string
	}{
		{name: "plain query at root", currentPrefix: "", query: "foo", want: "foo"},
		{name: "query scoped to current prefix", currentPrefix: "logs/", query: "2026", want: "logs/2026"},
		{name: "query already includes prefix", currentPrefix: "logs/", query: "logs/2026", want: "logs/2026"},
		{name: "leading slash means absolute", currentPrefix: "logs/", query: "/archive/", want: "archive/"},
		{name: "windows slash normalized", currentPrefix: "logs/", query: "2026\\02", want: "logs/2026/02"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := blobSearchPrefix(tc.currentPrefix, tc.query); got != tc.want {
				t.Fatalf("expected %q, got %q", tc.want, got)
			}
		})
	}
}

func TestComputePreviewWindow(t *testing.T) {
	tests := []struct {
		name         string
		totalSize    int64
		cursor       int64
		visibleLines int
	}{
		{name: "small blob", totalSize: 1024, cursor: 0, visibleLines: 20},
		{name: "middle of large blob", totalSize: 10 * 1024 * 1024, cursor: 5 * 1024 * 1024, visibleLines: 30},
		{name: "near end", totalSize: 10 * 1024 * 1024, cursor: 10*1024*1024 - 10, visibleLines: 25},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			start, count := computePreviewWindow(tc.totalSize, tc.cursor, tc.visibleLines)
			if start < 0 {
				t.Fatalf("expected non-negative start, got %d", start)
			}
			if count < 0 {
				t.Fatalf("expected non-negative count, got %d", count)
			}
			if start+count > tc.totalSize {
				t.Fatalf("window exceeds blob bounds: start=%d count=%d total=%d", start, count, tc.totalSize)
			}
		})
	}
}

func TestTypingQWhileSearchActiveDoesNotQuit(t *testing.T) {
	m := NewModel(nil, testConfig, nil)
	m.focus = blobsPane
	m.hasContainer = true
	m.filter.inputOpen = true

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	if _, ok := updated.(Model); !ok {
		t.Fatalf("expected updated model type %T, got %T", Model{}, updated)
	}

	if isQuitCmd(cmd) {
		t.Fatal("expected typing q during active search not to quit")
	}
}

func TestHelpToggleOpensAndCloses(t *testing.T) {
	m := NewModel(nil, testConfig, nil)
	m.SubOverlay.Close() // close auto-opened picker so keys reach help handler

	updated, _ := m.Update(tea.KeyPressMsg{Code: '?', Text: "?"})
	model := updated.(Model)
	if !model.HelpOverlay.Active {
		t.Fatal("expected ? to open help overlay")
	}

	updated, _ = model.Update(tea.KeyPressMsg{Code: '?', Text: "?"})
	model = updated.(Model)
	if model.HelpOverlay.Active {
		t.Fatal("expected ? to close help overlay")
	}
}

func TestViewRenders(t *testing.T) {
	m := NewModel(nil, testConfig, nil)
	m.Width = 120
	m.Height = 40
	m.resize()

	view := m.View()
	if view.Content == "" {
		t.Fatal("expected non-empty view")
	}
}

func TestBlobViewUsesCompactHeaderAndStatus(t *testing.T) {
	m := NewModel(nil, ui.Config{ThemeName: "fallback", Schemes: []ui.Scheme{ui.FallbackScheme()}}, nil)
	m.Width = 100
	m.Height = 30
	m.SubOverlay.Close() // exercise the underlying chrome, not the picker
	m.resize()
	out := m.View().Content
	if strings.Contains(out, "Storage Accounts") {
		t.Fatalf("old mixed-case pane title rendered: %q", out)
	}
	// "Blob" no longer appears in the breadcrumb — the tab bar labels
	// the explorer; the breadcrumb starts at the subscription. Brand stays.
	if !strings.Contains(out, "lazyaz") {
		t.Fatalf("compact app header missing brand: %q", out)
	}
	if !strings.Contains(out, "ACCOUNTS") {
		t.Fatalf("column title badge missing: %q", out)
	}
}

func TestPreviewViewportHeightMatchesFlatDetailsColumnBody(t *testing.T) {
	m := NewModel(nil, testConfig, nil)
	m.Width = 120
	m.Height = 30
	m.hasAccount = true
	m.hasContainer = true
	m.focus = blobsPane
	m.preview.open = true

	m.resize()

	want := ui.MillerListBodyHeight(m.paneHeight, false)
	if got := m.preview.viewport.Height(); got != want {
		t.Fatalf("preview viewport height = %d, want flat details body height %d", got, want)
	}
}

func isQuitCmd(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

// TestPreviewPaneOverflowDoesNotEatStatusBar verifies that even with a
// pathologically wide and long preview (a long blob name that wraps the
// title, very wide content lines that would force lipgloss to wrap, and
// far more content rows than fit in the viewport), the preview pane
// stays inside its frame and the status bar stays visible. Regression
// test for the v1→v2 lipgloss MaxHeight-clips-the-border bug.
func TestPreviewPaneOverflowDoesNotEatStatusBar(t *testing.T) {
	m := NewModel(nil, testConfig, nil)
	m.SubOverlay.Close()
	m.Width = 200
	m.Height = 60
	m.hasAccount = true
	m.currentAccount.Name = "test-account"
	m.hasContainer = true
	m.containerName = "test-container"
	m.focus = blobsPane
	m.preview.open = true
	m.preview.blobName = "verylongblobnamethatwillforcetitletowrapacrossseverallinesinthepreviewtitle.xml"
	m.preview.blobSize = 1024
	m.preview.contentType = "text/plain"
	wide := strings.Repeat("X", 500)
	m.preview.viewport.SetContent(strings.Repeat(wide+"\n", 200))
	m.preview.rendered = strings.Repeat(wide+"\n", 200)
	m.resize()

	view := m.View()
	if got := strings.Count(view.Content, "\n") + 1; got != m.Height {
		t.Errorf("rendered view: %d lines, want %d", got, m.Height)
	}
	stripped := ansi.Strip(view.Content)
	if !strings.Contains(stripped, "test-account") {
		t.Errorf("header account value missing from view")
	}
	if !strings.Contains(stripped, "DETAILS") {
		t.Errorf("preview details column missing from view")
	}
}

// TestVisualSelectionRespectsActiveFilter mirrors kvapp's test of the
// same name: with a list filter applied so only "alpha-*" rows are
// visible, a visual range from alpha-1 to alpha-3 must cover exactly
// the three alpha rows — never the beta rows hidden between them.
func TestVisualSelectionRespectsActiveFilter(t *testing.T) {
	m := NewModel(nil, testConfig, nil)
	m.focus = blobsPane
	m.blobs = []blob.BlobEntry{
		{Name: "alpha-1"},
		{Name: "beta-1"},
		{Name: "alpha-2"},
		{Name: "beta-2"},
		{Name: "alpha-3"},
	}
	m.refreshItems()
	m.blobsList.SetFilterText("alpha")

	if visible := m.blobsList.VisibleItems(); len(visible) != 3 {
		t.Fatalf("filter should expose 3 alpha rows, got %d", len(visible))
	}

	m.blobsList.Select(0) // alpha-1
	m.visualLineMode = true
	m.visualAnchor = "alpha-1"
	m.blobsList.Select(2) // alpha-3

	if got := m.visualRangeCount(); got != 3 {
		t.Fatalf("visualRangeCount = %d, want 3", got)
	}

	m.commitVisualSelection()
	want := []string{"alpha-1", "alpha-2", "alpha-3"}
	if len(m.markedBlobs) != len(want) {
		t.Fatalf("markedBlobs = %v, want %v", m.markedBlobs, want)
	}
	for _, name := range want {
		if _, ok := m.markedBlobs[name]; !ok {
			t.Fatalf("markedBlobs missing %q, got %v", name, m.markedBlobs)
		}
	}
}

// TestVisualAnchorSurvivesItemRebuild pins the anchor-index cache: a
// SetItems rebuild shifts every visible index, so the cached anchor
// position must be re-resolved from the anchor name, not reused.
func TestVisualAnchorSurvivesItemRebuild(t *testing.T) {
	m := NewModel(nil, testConfig, nil)
	m.focus = blobsPane
	m.blobs = []blob.BlobEntry{
		{Name: "b"},
		{Name: "c"},
		{Name: "d"},
	}
	m.refreshItems()

	m.blobsList.Select(1) // c
	m.visualLineMode = true
	m.visualAnchor = "c"
	m.blobsList.Select(2) // d

	if lo, hi, ok := m.visualRange(); !ok || lo != 1 || hi != 2 {
		t.Fatalf("visualRange = (%d, %d, %v), want (1, 2, true)", lo, hi, ok)
	}

	// Prepend an entry: c moves from index 1 to 2, and the cursor key
	// "d" is preserved by the rebuild at index 3.
	m.blobs = []blob.BlobEntry{
		{Name: "a"},
		{Name: "b"},
		{Name: "c"},
		{Name: "d"},
	}
	m.refreshItems()

	if lo, hi, ok := m.visualRange(); !ok || lo != 2 || hi != 3 {
		t.Fatalf("visualRange after rebuild = (%d, %d, %v), want (2, 3, true)", lo, hi, ok)
	}
}

// TestLoadMoreAppendsAndTracksMarker covers the "Load N more" flow: a
// continuation page appends to the current view (deduped), the marker
// state follows the response, and the pane title carries a trailing "+"
// only while more entries remain.
func TestLoadMoreAppendsAndTracksMarker(t *testing.T) {
	m := NewModel(nil, testConfig, nil)
	account := blob.Account{Name: "acct", SubscriptionID: "sub-1"}
	m.hasAccount = true
	m.currentAccount = account
	m.hasContainer = true
	m.containerName = "data"
	m.blobs = []blob.BlobEntry{{Name: "a.txt"}, {Name: "b.txt"}}
	m.blobNextMarker = "marker-1"

	updated, _ := m.handleMoreBlobsLoaded(moreBlobsLoadedMsg{
		account:    account,
		container:  "data",
		newBlobs:   []blob.BlobEntry{{Name: "b.txt"}, {Name: "c.txt"}, {Name: "d.txt"}},
		nextMarker: "marker-2",
	})
	if len(updated.blobs) != 4 {
		t.Fatalf("blobs = %d entries, want 4 (b.txt deduped)", len(updated.blobs))
	}
	if updated.blobNextMarker != "marker-2" {
		t.Fatalf("blobNextMarker = %q, want marker-2", updated.blobNextMarker)
	}
	if updated.blobsList.Title != "Blobs (4+)" {
		t.Fatalf("title = %q, want Blobs (4+)", updated.blobsList.Title)
	}
	if cached, ok := updated.cache.blobs.Get(blobsCacheKey("sub-1", "acct", "data", "", false)); !ok || len(cached) != 4 {
		t.Fatalf("broker store should hold the extended list, got %d ok=%v", len(cached), ok)
	}

	// Final page: marker drained, title drops the "+".
	updated, _ = updated.handleMoreBlobsLoaded(moreBlobsLoadedMsg{
		account:   account,
		container: "data",
		newBlobs:  []blob.BlobEntry{{Name: "e.txt"}},
	})
	if updated.blobNextMarker != "" {
		t.Fatalf("blobNextMarker = %q, want empty after final page", updated.blobNextMarker)
	}
	if updated.blobsList.Title != "Blobs (5)" {
		t.Fatalf("title = %q, want Blobs (5)", updated.blobsList.Title)
	}
}

// TestLoadMoreDroppedAfterNavigation: a continuation page landing after
// the user left the scope must not pollute the new view.
func TestLoadMoreDroppedAfterNavigation(t *testing.T) {
	m := NewModel(nil, testConfig, nil)
	account := blob.Account{Name: "acct", SubscriptionID: "sub-1"}
	m.hasAccount = true
	m.currentAccount = account
	m.hasContainer = true
	m.containerName = "other" // user moved on
	m.blobs = []blob.BlobEntry{{Name: "x.txt"}}

	updated, _ := m.handleMoreBlobsLoaded(moreBlobsLoadedMsg{
		account:    account,
		container:  "data",
		newBlobs:   []blob.BlobEntry{{Name: "stale.txt"}},
		nextMarker: "marker-2",
	})
	if len(updated.blobs) != 1 || updated.blobNextMarker != "" {
		t.Fatalf("stale continuation applied: %d blobs, marker %q", len(updated.blobs), updated.blobNextMarker)
	}
}

// TestLoadMoreActionVisibility: the action menu offers "Load N more"
// only in hierarchy mode with a continuation marker present.
func TestLoadMoreActionVisibility(t *testing.T) {
	m := NewModel(nil, testConfig, nil)
	m.hasAccount = true
	m.currentAccount = blob.Account{Name: "acct", SubscriptionID: "sub-1"}
	m.hasContainer = true
	m.containerName = "data"
	m.focus = blobsPane

	hasLoadMore := func(m Model) bool {
		for _, a := range m.buildActions() {
			if a.id == actionLoadMore {
				return true
			}
		}
		return false
	}

	if hasLoadMore(m) {
		t.Fatal("Load more should be hidden without a marker")
	}
	m.blobNextMarker = "marker-1"
	if !hasLoadMore(m) {
		t.Fatal("Load more should appear when a marker exists")
	}
	m.blobLoadAll = true
	if hasLoadMore(m) {
		t.Fatal("Load more should be hidden in load-all mode")
	}
}

// TestCurrentNavCapturesExactPosition: snapshots carry the folder
// prefix and the selected row, not just account/container — without
// them ctrl+o lands in the right container but at the root on an
// arbitrary row.
func TestCurrentNavCapturesExactPosition(t *testing.T) {
	m := NewModel(nil, testConfig, nil)
	m.CurrentSub = azure.Subscription{ID: "sub-1"}
	m.HasSubscription = true
	m.hasAccount = true
	m.currentAccount = blob.Account{Name: "acct", SubscriptionID: "sub-1"}
	m.hasContainer = true
	m.containerName = "data"
	m.prefix = "logs/2026/"
	m.focus = blobsPane
	m.blobs = []blob.BlobEntry{
		{Name: "logs/2026/jan.txt"},
		{Name: "logs/2026/feb.txt"},
	}
	m.refreshItems()
	m.blobsList.Select(1)

	snap, ok := m.CurrentNav().(blobNavSnapshot)
	if !ok {
		t.Fatal("CurrentNav should return a blobNavSnapshot")
	}
	if snap.prefix != "logs/2026/" {
		t.Fatalf("prefix = %q, want logs/2026/", snap.prefix)
	}
	if snap.itemKey != "logs/2026/feb.txt" {
		t.Fatalf("itemKey = %q, want the selected row", snap.itemKey)
	}
	if snap.subscriptionID != "sub-1" {
		t.Fatalf("subscriptionID = %q, want sub-1", snap.subscriptionID)
	}
}

// TestApplyNavRestoresExactPosition: with warm caches, restoring a
// snapshot puts the user back on the same account/container/prefix and
// the same blob row.
func TestApplyNavRestoresExactPosition(t *testing.T) {
	m := NewModel(nil, testConfig, nil)
	m.CurrentSub = azure.Subscription{ID: "sub-1"}
	m.HasSubscription = true
	account := blob.Account{Name: "acct", SubscriptionID: "sub-1"}
	m.cache.accounts.Set("sub-1", []blob.Account{account})
	m.cache.containers.Set(cache.Key("sub-1", "acct"), []blob.ContainerInfo{{Name: "data"}})
	m.cache.blobs.Set(blobsCacheKey("sub-1", "acct", "data", "logs/", false), []blob.BlobEntry{
		{Name: "logs/a.txt"},
		{Name: "logs/b.txt"},
	})

	m.ApplyNav(blobNavSnapshot{
		subscriptionID: "sub-1",
		accountName:    "acct",
		containerName:  "data",
		prefix:         "logs/",
		itemKey:        "logs/b.txt",
		focusedPane:    blobsPane,
	})

	if m.currentAccount.Name != "acct" || m.containerName != "data" || m.prefix != "logs/" {
		t.Fatalf("scope not restored: account=%q container=%q prefix=%q", m.currentAccount.Name, m.containerName, m.prefix)
	}
	it, ok := m.blobsList.SelectedItem().(blobItem)
	if !ok || it.blob.Name != "logs/b.txt" {
		t.Fatalf("cursor not restored to logs/b.txt, got %+v", m.blobsList.SelectedItem())
	}
	if m.pendingNav.hasTarget() {
		t.Fatal("pendingNav should be drained after a cache-warm restore")
	}
}

// TestStrictAncestorOf pins the drill-path comparison ctrl+o/ctrl+i
// use to skip h-equivalent stops.
func TestStrictAncestorOf(t *testing.T) {
	deep := blobNavSnapshot{subscriptionID: "s", accountName: "a", containerName: "c", prefix: "x/y/"}
	cases := []struct {
		name string
		s    blobNavSnapshot
		want bool
	}{
		{"accounts list is ancestor", blobNavSnapshot{subscriptionID: "s"}, true},
		{"containers list is ancestor", blobNavSnapshot{subscriptionID: "s", accountName: "a"}, true},
		{"parent folder is ancestor", blobNavSnapshot{subscriptionID: "s", accountName: "a", containerName: "c", prefix: "x/"}, true},
		{"equal scope is not", deep, false},
		{"sibling container is not", blobNavSnapshot{subscriptionID: "s", accountName: "a", containerName: "other"}, false},
		{"sibling folder is not", blobNavSnapshot{subscriptionID: "s", accountName: "a", containerName: "c", prefix: "z/"}, false},
		{"other subscription is not", blobNavSnapshot{subscriptionID: "t", accountName: "a"}, false},
	}
	for _, tc := range cases {
		if got := tc.s.StrictAncestorOf(deep); got != tc.want {
			t.Errorf("%s: StrictAncestorOf = %v, want %v", tc.name, got, tc.want)
		}
	}
}
