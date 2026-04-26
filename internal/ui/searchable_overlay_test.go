package ui

import "testing"

type searchableOverlayTestItem struct {
	label string
}

func TestSearchableOverlayFiltersAndSelects(t *testing.T) {
	var s SearchableOverlay[searchableOverlayTestItem]
	items := []searchableOverlayTestItem{
		{label: "download"},
		{label: "delete"},
		{label: "rename"},
	}

	s.Open(items, func(item searchableOverlayTestItem) string { return item.label })
	s.TypeText("del")

	visible := s.Visible()
	if len(visible) != 1 || visible[0].label != "delete" {
		t.Fatalf("Visible after filter: want only delete, got %+v", visible)
	}

	selected, ok := s.Selected()
	if !ok {
		t.Fatalf("Selected after filter: want delete, got none")
	}
	if selected.label != "delete" {
		t.Fatalf("Selected after filter: want delete, got %q", selected.label)
	}
}

func TestSearchableOverlayCancelClearsQueryBeforeClosing(t *testing.T) {
	var s SearchableOverlay[searchableOverlayTestItem]
	s.Open([]searchableOverlayTestItem{{label: "delete"}}, func(item searchableOverlayTestItem) string { return item.label })
	s.TypeText("del")

	if closed := s.Cancel(); closed {
		t.Fatalf("first Cancel with query: want closed=false")
	}
	if !s.Active {
		t.Fatalf("first Cancel with query should keep overlay active")
	}
	if s.Query != "" {
		t.Fatalf("first Cancel with query should clear query, got %q", s.Query)
	}

	if closed := s.Cancel(); !closed {
		t.Fatalf("second Cancel without query: want closed=true")
	}
	if s.Active {
		t.Fatalf("second Cancel without query should close overlay")
	}
}
