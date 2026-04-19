package dashapp

import (
	"testing"

	"github.com/karlssonsimon/lazyaz/internal/azure/servicebus"
)

func TestSortNamespaceStatsByName(t *testing.T) {
	stats := []nsStats{
		{namespace: servicebus.Namespace{Name: "zeta"}, queues: 1},
		{namespace: servicebus.Namespace{Name: "alpha"}, queues: 5},
		{namespace: servicebus.Namespace{Name: "beta"}, queues: 3},
	}
	sortNamespaceStats(stats, nsSortByName, false)
	want := []string{"alpha", "beta", "zeta"}
	for i, name := range want {
		if stats[i].namespace.Name != name {
			t.Errorf("row %d = %s, want %s", i, stats[i].namespace.Name, name)
		}
	}
}

func TestSortNamespaceStatsByDLQDesc(t *testing.T) {
	stats := []nsStats{
		{namespace: servicebus.Namespace{Name: "a"}, dlq: 5},
		{namespace: servicebus.Namespace{Name: "b"}, dlq: 100},
		{namespace: servicebus.Namespace{Name: "c"}, dlq: 0},
	}
	sortNamespaceStats(stats, nsSortByDLQ, true)
	wantOrder := []int64{100, 5, 0}
	for i, want := range wantOrder {
		if stats[i].dlq != want {
			t.Errorf("row %d dlq = %d, want %d", i, stats[i].dlq, want)
		}
	}
}

func TestSortNamespaceStatsTieBreaksByName(t *testing.T) {
	stats := []nsStats{
		{namespace: servicebus.Namespace{Name: "zeta"}, dlq: 5},
		{namespace: servicebus.Namespace{Name: "alpha"}, dlq: 5},
	}
	sortNamespaceStats(stats, nsSortByDLQ, false)
	// Equal DLQ → name ascending tiebreaker
	if stats[0].namespace.Name != "alpha" {
		t.Errorf("tiebreaker failed: row 0 = %s, want alpha", stats[0].namespace.Name)
	}
}

func TestSortDLQAlertsByCountDesc(t *testing.T) {
	alerts := []dlqAlert{
		{namespace: servicebus.Namespace{Name: "ns"}, entityName: "small", count: 1},
		{namespace: servicebus.Namespace{Name: "ns"}, entityName: "big", count: 50},
		{namespace: servicebus.Namespace{Name: "ns"}, entityName: "mid", count: 10},
	}
	sortDLQAlerts(alerts, dlqSortByCount, true)
	wantNames := []string{"big", "mid", "small"}
	for i, name := range wantNames {
		if alerts[i].entityName != name {
			t.Errorf("row %d = %s, want %s", i, alerts[i].entityName, name)
		}
	}
}

func TestSortDLQAlertsByNamespaceAsc(t *testing.T) {
	alerts := []dlqAlert{
		{namespace: servicebus.Namespace{Name: "zulu"}, count: 1},
		{namespace: servicebus.Namespace{Name: "alpha"}, count: 100},
		{namespace: servicebus.Namespace{Name: "mike"}, count: 50},
	}
	sortDLQAlerts(alerts, dlqSortByNamespace, false)
	wantOrder := []string{"alpha", "mike", "zulu"}
	for i, name := range wantOrder {
		if alerts[i].namespace.Name != name {
			t.Errorf("row %d = %s, want %s", i, alerts[i].namespace.Name, name)
		}
	}
}

func TestSortDLQAlertsTopicSubsByEntity(t *testing.T) {
	alerts := []dlqAlert{
		{namespace: servicebus.Namespace{Name: "ns"}, entityName: "topic", subName: "z-sub", count: 1},
		{namespace: servicebus.Namespace{Name: "ns"}, entityName: "topic", subName: "a-sub", count: 1},
	}
	sortDLQAlerts(alerts, dlqSortByEntity, false)
	if alerts[0].subName != "a-sub" {
		t.Errorf("first row sub = %s, want a-sub", alerts[0].subName)
	}
}

func TestMatchesFilterCaseInsensitive(t *testing.T) {
	cases := []struct {
		hay, needle string
		want        bool
	}{
		{"sb-prod", "PROD", true},
		{"sb-prod", "prod", true},
		{"sb-prod", "stage", false},
		{"sb-prod", "", true},
	}
	for _, tc := range cases {
		if got := matchesFilter(tc.hay, tc.needle); got != tc.want {
			t.Errorf("matchesFilter(%q, %q) = %v, want %v", tc.hay, tc.needle, got, tc.want)
		}
	}
}

func TestDLQAlertMatchesFilterByEntity(t *testing.T) {
	a := dlqAlert{
		namespace:  servicebus.Namespace{Name: "sb-prod"},
		entityName: "orders",
		subName:    "fulfillment",
	}
	if !a.matchesFilter("FULFILL") {
		t.Errorf("expected entity-side match")
	}
	if !a.matchesFilter("prod") {
		t.Errorf("expected namespace-side match")
	}
	if a.matchesFilter("xyz") {
		t.Errorf("expected no match")
	}
}

func TestSortActionIsSingleEntry(t *testing.T) {
	a := sortAction()
	if a.Key != "s" {
		t.Errorf("sortAction key = %q, want %q", a.Key, "s")
	}
	if a.Cmd == nil {
		t.Errorf("sortAction Cmd is nil")
	}
	msg := a.Cmd()
	if _, ok := msg.(openSortOverlayMsg); !ok {
		t.Errorf("sortAction emits %T, want openSortOverlayMsg", msg)
	}
}
