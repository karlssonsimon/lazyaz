package blobapp

import (
	"testing"

	"github.com/karlssonsimon/lazyaz/internal/azure/blob"
)

func newYankTestModel(t *testing.T) Model {
	t.Helper()
	m := NewModel(nil, testConfig, nil)
	m.SubOverlay.Close()
	m.hasAccount = true
	m.currentAccount = blob.Account{Name: "acct"}
	m.hasContainer = true
	m.containerName = "c"
	m.blobs = []blob.BlobEntry{{Name: "a.txt"}, {Name: "b.txt"}, {Name: "c.txt"}, {Name: "d.txt"}}
	m.refreshItems()
	return m
}

// TestYankBlobNamesUsesMarks pins that yanking names with marks set
// copies every marked name, one per line, not just the cursor row.
func TestYankBlobNamesUsesMarks(t *testing.T) {
	m := newYankTestModel(t)
	m.marked.Add("d.txt")
	m.marked.Add("b.txt")
	m.blobsList.Select(0) // cursor on a.txt, which is not marked

	got, _ := m.yankBlobNames()
	if want := "b.txt\nd.txt"; got != want {
		t.Fatalf("yankBlobNames() = %q, want %q", got, want)
	}
}

// TestYankBlobNamesUsesVisualRange pins that an uncommitted visual range
// counts as the selection, exactly like the download path.
func TestYankBlobNamesUsesVisualRange(t *testing.T) {
	m := newYankTestModel(t)
	m.blobsList.Select(1)
	m.toggleVisualLineMode()
	m.blobsList.Select(2)

	got, updated := m.yankBlobNames()
	if want := "b.txt\nc.txt"; got != want {
		t.Fatalf("yankBlobNames() = %q, want %q", got, want)
	}
	if updated.visual.Active() {
		t.Fatal("yank should end visual mode like download does")
	}
}

// TestYankBlobNamesFallsBackToCursor pins the single-row behavior.
func TestYankBlobNamesFallsBackToCursor(t *testing.T) {
	m := newYankTestModel(t)
	m.blobsList.Select(2)

	got, _ := m.yankBlobNames()
	if want := "c.txt"; got != want {
		t.Fatalf("yankBlobNames() = %q, want %q", got, want)
	}
}

// TestActionMenuYankLabelCountsSelection pins that the menu entry says
// how many names it will copy once a selection exists.
func TestActionMenuYankLabelCountsSelection(t *testing.T) {
	m := newYankTestModel(t)
	m.focus = blobsPane
	m.marked.Add("a.txt")
	m.marked.Add("b.txt")

	var labels []string
	for _, a := range m.buildActions() {
		labels = append(labels, a.label)
	}
	found := false
	for _, l := range labels {
		if l == "Yank blob names (2)" {
			found = true
		}
		if l == "Yank blob name" {
			t.Fatalf("single-row yank entry still offered with 2 marked: %v", labels)
		}
	}
	if !found {
		t.Fatalf("no multi-name yank entry in %v", labels)
	}
}
