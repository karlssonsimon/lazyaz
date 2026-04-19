// Package dashapp is the dashboard tab — a read-only overview of Azure
// resources with a fixed set of widgets. Unlike the resource-specific
// explorers (blobapp, sbapp, kvapp), there are no list panes or drill-down
// flows: widgets render aggregate data and refresh on demand.
package dashapp

import (
	"github.com/karlssonsimon/lazyaz/internal/appshell"
	"github.com/karlssonsimon/lazyaz/internal/azure"
	"github.com/karlssonsimon/lazyaz/internal/azure/servicebus"
	"github.com/karlssonsimon/lazyaz/internal/cache"
	"github.com/karlssonsimon/lazyaz/internal/keymap"
	"github.com/karlssonsimon/lazyaz/internal/ui"
)

// DashStores bundles the caches the dashboard reads. Populated by the
// parent tabapp from the shared broker set so widgets piggyback on the
// same data the sbapp explorer uses. DB is also used for usage stats
// the "Most used" widgets render.
type DashStores struct {
	Subscriptions *cache.Broker[azure.Subscription]
	Namespaces    *cache.Broker[servicebus.Namespace]
	Entities      *cache.Broker[servicebus.Entity]
	TopicSubs     *cache.Broker[servicebus.TopicSubscription]
	DB            *cache.DB
}

// Model is the dashboard tab's Bubble Tea model.
type Model struct {
	appshell.Model

	service *servicebus.Service
	stores  DashStores
	db      *cache.DB

	// namespaces in the current subscription.
	namespaces []servicebus.Namespace

	// entitiesByNS[namespaceName] = entities list. Populated as fan-out
	// fetches complete. Missing key means "not yet fetched".
	entitiesByNS map[string][]servicebus.Entity

	// topicSubsByKey[namespaceName+"/"+topicName] = subscriptions. Same
	// convention as entitiesByNS.
	topicSubsByKey map[string][]servicebus.TopicSubscription

	// pendingFetches tracks in-flight per-namespace entity fetches so
	// the UI can show an aggregate "loading" indicator.
	pendingFetches int

	// refreshInFlight counts the fetches fired by refreshAll that
	// haven't yet delivered a done (or error) page. A refresh is
	// blocked while this is non-zero so spamming the refresh key
	// doesn't pile up redundant broker subscribers.
	refreshInFlight int

	// widgets is the dashboard's registered widget set, in display
	// order. Index doubles as focus index and offset slot.
	widgets []Widget

	// focusedIdx is the index into widgets that currently has focus
	// for scroll + spatial nav.
	focusedIdx int

	// offsets is parallel to widgets — offsets[i] is the scroll offset
	// for widgets[i]. Clamped at render time so data changes can't
	// leave a widget scrolled past its end.
	offsets []int

	// cursors is parallel to widgets — cursors[i] is the row the
	// cursor is on within widgets[i]'s data list. Drives highlight
	// rendering and is the row index actions operate on.
	cursors []int

	// viewStates is parallel to widgets — per-widget sort/filter
	// state. Ephemeral (not persisted across tab close).
	viewStates []widgetViewState

	// gPrefixActive is true between the first 'g' and the second 'g'
	// of a `gg` jump-to-top chord. Cleared by the second g (which
	// triggers the jump) or by any other key press.
	gPrefixActive bool

	// rowHeights are the inner widget pane heights, indexed by grid
	// row, recomputed each WindowSizeMsg from the real bar heights.
	// Scroll math reads these so the clamp matches what the renderer
	// is actually doing.
	rowHeights []int

	// actionMenu is the overlay opened by `a` listing the focused
	// widget's actions for the cursor row.
	actionMenu actionMenuState

	// sortOverlay is the dedicated sort picker opened by the
	// "Sort by..." action (or the `s` direct keybind).
	sortOverlay sortOverlayState

	// filterInputActive is true while the user is typing into the
	// focused widget's filter. While true, all keypresses are
	// consumed by the filter input (no shortcuts fire) and
	// IsTextInputActive returns true so the parent stays out of
	// the way.
	filterInputActive bool
}

// NewModelWithCache constructs a dashboard model wired to shared caches.
func NewModelWithCache(svc *servicebus.Service, cfg ui.Config, stores DashStores, km keymap.Keymap) Model {
	widgets := dashboardWidgets()
	m := Model{
		Model:          appshell.New(cfg, km),
		service:        svc,
		stores:         stores,
		entitiesByNS:   make(map[string][]servicebus.Entity),
		topicSubsByKey: make(map[string][]servicebus.TopicSubscription),
		widgets:        widgets,
		offsets:        make([]int, len(widgets)),
		cursors:        make([]int, len(widgets)),
		viewStates:     make([]widgetViewState, len(widgets)),
		db:             stores.DB,
	}
	m.HydrateSubscriptionsFromCache(stores.Subscriptions)
	if !m.HasSubscription {
		m.SubOverlay.Open()
		m.SetLoading(-1)
		m.Status = "Loading Azure subscriptions..."
	}
	return m
}

// SetSubscription overrides the embedded appshell.Model method to reset
// per-subscription state and re-swap the credential for tenant-scoped
// access (mirrors sbapp's pattern). Hydrates every cached layer the
// dashboard renders so an already-warmed cache (e.g. from sbapp on the
// same subscription) lights up instantly instead of flashing placeholders.
func (m *Model) SetSubscription(sub azure.Subscription) {
	m.Model.SetSubscription(sub)
	if sub.TenantID != "" {
		if cred, err := azure.NewCredentialForTenant(sub.TenantID); err == nil {
			m.service.SetCredential(cred)
		}
	}
	m.namespaces = nil
	m.entitiesByNS = make(map[string][]servicebus.Entity)
	m.topicSubsByKey = make(map[string][]servicebus.TopicSubscription)
	m.pendingFetches = 0
	m.hydrateFromCache(sub)
}

// hydrateFromCache fills namespaces, entitiesByNS, and topicSubsByKey
// from whatever the shared brokers already hold. A subsequent fetch
// still runs to confirm freshness — this just gives the user something
// useful to look at on tab open.
func (m *Model) hydrateFromCache(sub azure.Subscription) {
	cachedNS, ok := m.stores.Namespaces.Get(sub.ID)
	if !ok {
		return
	}
	m.namespaces = cachedNS
	for _, ns := range cachedNS {
		ents, ok := m.stores.Entities.Get(ns.Name)
		if !ok {
			continue
		}
		m.entitiesByNS[ns.Name] = ents
		for _, e := range ents {
			if e.Kind != servicebus.EntityTopic {
				continue
			}
			key := ns.Name + "/" + e.Name
			if subs, ok := m.stores.TopicSubs.Get(key); ok {
				m.topicSubsByKey[key] = subs
			}
		}
	}
}

// ApplyScheme repaints styles; no list delegates to update.
func (m *Model) ApplyScheme(scheme ui.Scheme) {
	m.SetScheme(scheme)
}

// IsTextInputActive reports whether the tab is accepting free-form text.
// True while a filter input is open so the parent app doesn't snatch
// keys for global shortcuts (single-letter quit, tab nav, etc.).
func (m Model) IsTextInputActive() bool {
	return m.filterInputActive
}

// HelpSections returns the dashboard's keybindings for the parent
// help overlay. Mirrors the structure used by blobapp/sbapp/kvapp.
func (m Model) HelpSections() []ui.HelpSection {
	km := m.Keymap
	return []ui.HelpSection{
		{
			Title: "Widget navigation",
			Items: []string{
				keymap.HelpEntry(km.WidgetUp, "focus widget above"),
				keymap.HelpEntry(km.WidgetDown, "focus widget below"),
				keymap.HelpEntry(km.WidgetLeft, "focus widget left"),
				keymap.HelpEntry(km.WidgetRight, "focus widget right"),
			},
		},
		{
			Title: "Scroll focused widget",
			Items: []string{
				keymap.HelpEntry(km.WidgetScrollUp, "scroll up one row"),
				keymap.HelpEntry(km.WidgetScrollDown, "scroll down one row"),
				keymap.HelpEntry(km.HalfPageUp, "scroll half page up"),
				keymap.HelpEntry(km.HalfPageDown, "scroll half page down"),
				keymap.HelpEntry(km.WidgetScrollTop, "jump to top (gg)"),
				keymap.HelpEntry(km.WidgetScrollBottom, "jump to bottom"),
			},
		},
		{
			Title: "Widget actions",
			Items: []string{
				keymap.HelpEntry(km.ActionMenu, "open action menu (cursor row)"),
				"o  open in Service Bus tab",
				"s  sort picker (each direction is its own option)",
				keymap.HelpEntry(km.FilterInput, "filter rows (esc clears, enter accepts)"),
				"x  clear usage stats (on usage widgets)",
			},
		},
		{
			Title: "App",
			Items: []string{
				keymap.HelpEntry(km.SubscriptionPicker, "change subscription"),
				keymap.HelpEntry(km.RefreshScope, "refresh data"),
				keymap.HelpEntry(km.ReloadSubscriptions, "reload subscriptions"),
				keymap.HelpEntry(km.ToggleHelp, "toggle help"),
				keymap.HelpEntry(km.Quit, "quit"),
			},
		},
	}
}
