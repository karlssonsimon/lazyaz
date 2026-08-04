package blobapp

import (
	"testing"

	"github.com/karlssonsimon/lazyaz/internal/azure/blob"

	tea "charm.land/bubbletea/v2"
)

// TestCopyTargetsFullScope pins the palette contents for a drilled-in
// scope: values must be the real untruncated strings — especially the
// prefix path, which the header breadcrumb renders truncated.
func TestCopyTargetsFullScope(t *testing.T) {
	longPrefix := "data/logs/2026/very/deep/nested/folder/structure/"

	m := NewModel(nil, testConfig, nil)
	m.hasAccount = true
	m.currentAccount = blob.Account{Name: "acct", BlobEndpoint: "https://acct.blob.core.windows.net/"}
	m.hasContainer = true
	m.containerName = "prod-logs"
	m.prefix = longPrefix
	m.blobs = []blob.BlobEntry{{Name: longPrefix + "app.json"}}
	m.refreshItems()
	m.marked.Add("a.txt")
	m.marked.Add("b.txt")

	got := map[string]string{}
	for _, target := range m.copyTargets() {
		got[target.Label] = target.Value
	}

	want := map[string]string{
		"Blob name":        longPrefix + "app.json",
		"Blob URL":         "https://acct.blob.core.windows.net/prod-logs/" + longPrefix + "app.json",
		"Prefix path":      longPrefix,
		"Container":        "prod-logs",
		"Account":          "acct",
		"Blob endpoint":    "https://acct.blob.core.windows.net/",
		"Marked names (2)": "a.txt\nb.txt",
	}
	for label, value := range want {
		if got[label] != value {
			t.Errorf("target %q = %q, want %q", label, got[label], value)
		}
	}
	if len(got) != len(want) {
		t.Errorf("copyTargets returned %d entries, want %d: %v", len(got), len(want), got)
	}
}

func TestCopyTargetsEmptyScope(t *testing.T) {
	m := NewModel(nil, testConfig, nil)
	if targets := m.copyTargets(); len(targets) != 0 {
		t.Fatalf("empty scope should offer no targets, got %v", targets)
	}
}

// TestCopyPaletteKeyOpensOverlay drives the Y binding through the
// normal-mode key handler.
func TestCopyPaletteKeyOpensOverlay(t *testing.T) {
	m := NewModel(nil, testConfig, nil)
	m.SubOverlay.Close()
	m.hasAccount = true
	m.currentAccount = blob.Account{Name: "acct"}

	updated, _ := m.handleNormalKey(tea.KeyPressMsg{Code: 'Y', Text: "Y"}, "Y")
	if !updated.copyOverlay.Active {
		t.Fatal("Y should open the copy palette")
	}
	if updated.inputMode() != ModeCopyPalette {
		t.Fatalf("inputMode = %v, want ModeCopyPalette", updated.inputMode())
	}
}
