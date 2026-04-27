package dashapp

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/karlssonsimon/lazyaz/internal/azure"
	"github.com/karlssonsimon/lazyaz/internal/azure/servicebus"
	"github.com/karlssonsimon/lazyaz/internal/cache"
	"github.com/karlssonsimon/lazyaz/internal/keymap"
	"github.com/karlssonsimon/lazyaz/internal/ui"
)

func TestDashboardHelpDescribesMillerColumns(t *testing.T) {
	m := NewModelWithCache(nil, ui.Config{ThemeName: "fallback", Schemes: []ui.Scheme{ui.FallbackScheme()}}, DashStores{
		Subscriptions: cache.NewBroker(cache.NewMap[azure.Subscription](), func(s azure.Subscription) string { return s.ID }),
		Namespaces:    cache.NewBroker(cache.NewMap[servicebus.Namespace](), servicebus.NamespaceKey),
		Entities:      cache.NewBroker(cache.NewMap[servicebus.Entity](), servicebus.EntityKey),
		TopicSubs:     cache.NewBroker(cache.NewMap[servicebus.TopicSubscription](), servicebus.TopicSubscriptionKey),
	}, keymap.Default())
	sections := m.HelpSections()
	joined := fmt.Sprint(sections)
	if !strings.Contains(joined, "widget") || !strings.Contains(joined, "filter focused widget") {
		t.Fatalf("help does not describe dashboard navigation: %v", sections)
	}
}

func TestDashboardViewUsesCommandCenterChrome(t *testing.T) {
	m := NewModelWithCache(nil, ui.Config{ThemeName: "fallback", Schemes: []ui.Scheme{ui.FallbackScheme()}}, DashStores{
		Subscriptions: cache.NewBroker(cache.NewMap[azure.Subscription](), func(s azure.Subscription) string { return s.ID }),
		Namespaces:    cache.NewBroker(cache.NewMap[servicebus.Namespace](), servicebus.NamespaceKey),
		Entities:      cache.NewBroker(cache.NewMap[servicebus.Entity](), servicebus.EntityKey),
		TopicSubs:     cache.NewBroker(cache.NewMap[servicebus.TopicSubscription](), servicebus.TopicSubscriptionKey),
	}, keymap.Default())
	m.Width = 100
	m.Height = 30
	m.SubOverlay.Close()
	m.recomputeWidgetHeights()
	out := m.View().Content
	// "Dashboard" no longer appears in the breadcrumb when no sub is
	// set; the tab bar labels the explorer. Brand stays.
	if !strings.Contains(out, "lazyaz") {
		t.Fatalf("compact Dashboard header missing brand: %q", out)
	}
	if strings.Contains(out, "╭") || strings.Contains(out, "╰") {
		t.Fatalf("dashboard still renders boxed widget borders: %q", out)
	}
	if !strings.Contains(out, "j/k rows") || strings.Contains(out, "j/k move") {
		t.Fatalf("dashboard footer should describe row movement explicitly: %q", out)
	}
}

func TestSetSubscriptionAllowsNilServiceWithTenant(t *testing.T) {
	m := NewModelWithCache(nil, ui.Config{ThemeName: "fallback", Schemes: []ui.Scheme{ui.FallbackScheme()}}, DashStores{
		Subscriptions: cache.NewBroker(cache.NewMap[azure.Subscription](), func(s azure.Subscription) string { return s.ID }),
		Namespaces:    cache.NewBroker(cache.NewMap[servicebus.Namespace](), servicebus.NamespaceKey),
		Entities:      cache.NewBroker(cache.NewMap[servicebus.Entity](), servicebus.EntityKey),
		TopicSubs:     cache.NewBroker(cache.NewMap[servicebus.TopicSubscription](), servicebus.TopicSubscriptionKey),
	}, keymap.Default())
	if m.service == nil {
		t.Fatalf("NewModelWithCache(nil) left service nil")
	}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("SetSubscription panicked with nil service: %v", r)
		}
	}()
	m.SetSubscription(azure.Subscription{ID: "sub", TenantID: "tenant"})
}

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

func TestNamespaceCountsActionUsesSortedRenderedRow(t *testing.T) {
	m := dashboardActionTestModel(t)
	m.focusedIdx = 0
	m.namespaces = []servicebus.Namespace{{Name: "zeta"}, {Name: "alpha"}}
	m.viewStates[0] = widgetViewState{hasSort: true, sortField: nsSortByName}

	actions := namespaceCountsWidget{}.Actions(&m, 0)
	msg, ok := actionMsg(actions, "o").(OpenSBNamespaceMsg)
	if !ok {
		t.Fatalf("open action emitted %T, want OpenSBNamespaceMsg", actionMsg(actions, "o"))
	}
	if msg.Namespace.Name != "alpha" {
		t.Fatalf("opened namespace = %q, want highlighted sorted row alpha", msg.Namespace.Name)
	}
}

func TestDLQAlertsActionUsesFilteredSortedRenderedRow(t *testing.T) {
	m := dashboardActionTestModel(t)
	m.focusedIdx = 1
	m.namespaces = []servicebus.Namespace{{Name: "beta"}, {Name: "alpha"}}
	m.entitiesByNS = map[string][]servicebus.Entity{
		"beta":  {{Name: "orders", Kind: servicebus.EntityQueue, DeadLetterCount: 10}},
		"alpha": {{Name: "orders", Kind: servicebus.EntityQueue, DeadLetterCount: 1}},
	}
	m.viewStates[1] = widgetViewState{filter: "orders", hasSort: true, sortField: dlqSortByNamespace}

	actions := dlqAlertsWidget{}.Actions(&m, 0)
	msg, ok := actionMsg(actions, "o").(OpenSBEntityMsg)
	if !ok {
		t.Fatalf("open action emitted %T, want OpenSBEntityMsg", actionMsg(actions, "o"))
	}
	if msg.Namespace.Name != "alpha" {
		t.Fatalf("opened namespace = %q, want highlighted sorted row alpha", msg.Namespace.Name)
	}
}

func TestServiceBusUsageActionUsesSortedRenderedRow(t *testing.T) {
	m := dashboardActionTestModel(t)
	m.focusedIdx = 2
	m.db.RecordUsage(sbResourceQueue, "sub/ns/zeta", "sub", "zeta")
	m.db.RecordUsage(sbResourceQueue, "sub/ns/alpha", "sub", "alpha")
	m.viewStates[2] = widgetViewState{hasSort: true, sortField: 1}

	actions := usedSBWidget{}.Actions(&m, 0)
	msg, ok := actionMsg(actions, "o").(OpenSBEntityMsg)
	if !ok {
		t.Fatalf("open action emitted %T, want OpenSBEntityMsg", actionMsg(actions, "o"))
	}
	if msg.EntityName != "alpha" {
		t.Fatalf("opened entity = %q, want highlighted sorted row alpha", msg.EntityName)
	}
}

func TestBlobUsageActionUsesFilteredSortedRenderedRow(t *testing.T) {
	m := dashboardActionTestModel(t)
	m.focusedIdx = 3
	m.db.RecordUsage(blobResourceAccount, "sub/zeta", "sub", "zeta")
	m.db.RecordUsage(blobResourceAccount, "sub/alpha", "sub", "alpha")
	m.viewStates[3] = widgetViewState{filter: "a", hasSort: true, sortField: 1}

	actions := usedBlobWidget{}.Actions(&m, 0)
	msg, ok := actionMsg(actions, "o").(OpenBlobAccountMsg)
	if !ok {
		t.Fatalf("open action emitted %T, want OpenBlobAccountMsg", actionMsg(actions, "o"))
	}
	if msg.AccountName != "alpha" {
		t.Fatalf("opened account = %q, want highlighted sorted row alpha", msg.AccountName)
	}
}

func dashboardActionTestModel(t *testing.T) Model {
	t.Helper()
	db, err := cache.OpenDB(filepath.Join(t.TempDir(), "cache.db"))
	if err != nil {
		t.Fatalf("open cache db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	m := NewModelWithCache(nil, ui.Config{ThemeName: "fallback", Schemes: []ui.Scheme{ui.FallbackScheme()}}, DashStores{
		Subscriptions: cache.NewBroker(cache.NewMap[azure.Subscription](), func(s azure.Subscription) string { return s.ID }),
		Namespaces:    cache.NewBroker(cache.NewMap[servicebus.Namespace](), servicebus.NamespaceKey),
		Entities:      cache.NewBroker(cache.NewMap[servicebus.Entity](), servicebus.EntityKey),
		TopicSubs:     cache.NewBroker(cache.NewMap[servicebus.TopicSubscription](), servicebus.TopicSubscriptionKey),
		DB:            db,
	}, keymap.Default())
	m.SetSubscription(azure.Subscription{ID: "sub", TenantID: "tenant"})
	m.SubOverlay.Close()
	return m
}

func actionMsg(actions []Action, key string) tea.Msg {
	for _, a := range actions {
		if a.Key == key && a.Cmd != nil {
			return a.Cmd()
		}
	}
	return nil
}
