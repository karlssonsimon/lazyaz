package ui

import (
	"testing"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
)

type navItem string

func (n navItem) FilterValue() string { return string(n) }
func (n navItem) Title() string       { return string(n) }
func (n navItem) Description() string { return "" }

func navItems(names ...string) []list.Item {
	items := make([]list.Item, len(names))
	for i, n := range names {
		items[i] = navItem(n)
	}
	return items
}

// Repro: user presses "/" (Filtering, empty query), then a background
// refresh replaces the items via SetItemsPreserveKey.
func TestSetItemsPreserveKeyDuringEmptyFiltering(t *testing.T) {
	l := list.New(navItems("alpha", "beta"), list.NewDefaultDelegate(), 40, 20)
	l.SetFilteringEnabled(true)

	m2, _ := l.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	l = m2
	if l.FilterState() != list.Filtering {
		t.Fatalf("expected Filtering after /, got %v", l.FilterState())
	}
	if got := len(l.VisibleItems()); got != 2 {
		t.Fatalf("empty filter should show all: got %d", got)
	}

	SetItemsPreserveKey(&l, navItems("alpha", "beta", "gamma"), func(it list.Item) string { return string(it.(navItem)) })

	if got := len(l.VisibleItems()); got != 3 {
		t.Fatalf("after refresh during empty Filtering: %d visible, want 3", got)
	}
}
