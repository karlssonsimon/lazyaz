package ui

import (
	"testing"

	"charm.land/bubbles/v2/list"
)

type versionItem string

func (v versionItem) FilterValue() string { return string(v) }
func (v versionItem) Title() string       { return string(v) }
func (v versionItem) Description() string { return "" }

// Every path that can reshape the visible item set must yield a new
// version — vim.Visual trusts this number instead of re-scanning per
// keypress, so a path that forgets to bump shows a stale selection.
func TestVisibleVersionBumpsOnMutation(t *testing.T) {
	items := []list.Item{versionItem("alpha"), versionItem("beta")}
	l := NewList(items, list.NewDefaultDelegate(), 40, 10)

	mutations := []struct {
		name string
		do   func()
	}{
		{"SetItems", func() { l.SetItems(items) }},
		{"SetFilterText", func() { l.SetFilterText("al") }},
		{"ResetFilter", func() { l.ResetFilter() }},
		{"SetItemsPreserveKey", func() {
			SetItemsPreserveKey(&l, items, func(it list.Item) string { return string(it.(versionItem)) })
		}},
		{"SyncFilter while filtering", func() {
			l.SetFilterState(list.Filtering)
			l.FilterInput.SetValue("a")
			SyncFilter(&l)
		}},
	}

	for _, m := range mutations {
		before := l.VisibleVersion()
		m.do()
		if l.VisibleVersion() == before {
			t.Errorf("%s did not bump the visible version", m.name)
		}
	}
}

// Reads must not bump: a stable version is what makes the cache work.
func TestVisibleVersionStableAcrossReads(t *testing.T) {
	l := NewList([]list.Item{versionItem("alpha")}, list.NewDefaultDelegate(), 40, 10)

	before := l.VisibleVersion()
	_ = l.VisibleItems()
	_ = l.Index()
	l.Select(0)
	l.CursorDown()
	_ = l.RenderWindow()
	if got := l.VisibleVersion(); got != before {
		t.Errorf("version moved from %d to %d without a mutation", before, got)
	}
}
