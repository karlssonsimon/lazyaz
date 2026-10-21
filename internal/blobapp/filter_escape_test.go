package blobapp

import (
	"testing"

	"github.com/karlssonsimon/lazyaz/internal/azure/blob"
	"github.com/karlssonsimon/lazyaz/internal/ui"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
)

func newEscTestModel(t *testing.T) Model {
	t.Helper()
	m := NewModel(nil, testConfig, nil)
	m.SubOverlay.Close()
	m.hasAccount = true
	m.currentAccount = blob.Account{Name: "acct"}
	m.hasContainer = true
	m.containerName = "c"
	m.focus = blobsPane
	m.blobs = []blob.BlobEntry{{Name: "a.txt"}, {Name: "b.txt"}, {Name: "c.txt"}, {Name: "d.txt"}}
	m.refreshItems()
	return m
}

func pressEsc(m Model) Model {
	updated, _ := m.handleNormalKey(tea.KeyPressMsg{Code: tea.KeyEscape}, "esc")
	return updated
}

// TestEscAfterFilterKeepsCursorOnFilteredBlob: filter down to one
// file, apply the filter, press Esc. The filter clears but the cursor
// must stay on that file in the full list rather than on whatever row
// shares its filtered index.
func TestEscAfterFilterKeepsCursorOnFilteredBlob(t *testing.T) {
	m := newEscTestModel(t)
	m.blobsList.SetFilterText("c.txt")
	ui.ApplyFilterState(&m.blobsList)
	m.blobsList.Select(0)
	if got := blobItemKey(m.blobsList.SelectedItem()); got != "c.txt" {
		t.Fatalf("setup: filtered selection = %q, want c.txt", got)
	}

	m = pressEsc(m)

	if m.blobsList.FilterState() != list.Unfiltered {
		t.Fatalf("filter state = %v, want Unfiltered", m.blobsList.FilterState())
	}
	if got := blobItemKey(m.blobsList.SelectedItem()); got != "c.txt" {
		t.Fatalf("after Esc selection = %q, want c.txt", got)
	}
	if got := len(m.blobsList.VisibleItems()); got != 4 {
		t.Fatalf("after Esc %d rows visible, want 4", got)
	}
}

// TestEscWithNothingToClearJumpsToTop: a second Esc, with no filter
// and no marks left to peel, moves the cursor to the first row.
func TestEscWithNothingToClearJumpsToTop(t *testing.T) {
	m := newEscTestModel(t)
	m.blobsList.Select(2)

	m = pressEsc(m)

	if got := m.blobsList.Index(); got != 0 {
		t.Fatalf("after Esc index = %d, want 0", got)
	}
}

// TestEscPeelsFilterThenMarksThenTop pins the order of the Esc stack.
func TestEscPeelsFilterThenMarksThenTop(t *testing.T) {
	m := newEscTestModel(t)
	m.marked.Add("a.txt")
	m.blobsList.SetFilterText("d.txt")
	ui.ApplyFilterState(&m.blobsList)
	m.blobsList.Select(0)

	m = pressEsc(m)
	if got := blobItemKey(m.blobsList.SelectedItem()); got != "d.txt" || m.marked.Len() != 1 {
		t.Fatalf("first Esc: selection=%q marked=%d, want d.txt and 1 mark", got, m.marked.Len())
	}

	m = pressEsc(m)
	if got := blobItemKey(m.blobsList.SelectedItem()); got != "d.txt" || m.marked.Len() != 0 {
		t.Fatalf("second Esc: selection=%q marked=%d, want d.txt and 0 marks", got, m.marked.Len())
	}

	m = pressEsc(m)
	if got := m.blobsList.Index(); got != 0 {
		t.Fatalf("third Esc: index=%d, want 0", got)
	}
}

// TestEscAfterFilterKeepsCursorInContainersPane: the same holds for the
// other panes, which otherwise let the bubbles list reset the filter.
func TestEscAfterFilterKeepsCursorInContainersPane(t *testing.T) {
	m := newEscTestModel(t)
	m.focus = containersPane
	m.containersList.SetItems(containersToItems([]blob.ContainerInfo{{Name: "alpha"}, {Name: "beta"}, {Name: "gamma"}}))
	m.containersList.SetFilterText("gamma")
	ui.ApplyFilterState(&m.containersList)
	m.containersList.Select(0)

	m = pressEsc(m)

	if m.containersList.FilterState() != list.Unfiltered {
		t.Fatalf("filter state = %v, want Unfiltered", m.containersList.FilterState())
	}
	if got := containerItemKey(m.containersList.SelectedItem()); got != "gamma" {
		t.Fatalf("after Esc selection = %q, want gamma", got)
	}
}
