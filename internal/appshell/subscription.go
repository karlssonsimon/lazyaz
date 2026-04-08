package appshell

import (
	"github.com/karlssonsimon/lazyaz/internal/azure"
	"github.com/karlssonsimon/lazyaz/internal/cache"

	tea "charm.land/bubbletea/v2"
)

// SubscriptionsLoadedMsg is the shared result of fetchSubscriptionsCmd.
// It fires once (done=true) at the end, or repeatedly during streaming loads
// where `next` chains the follow-up command.
type SubscriptionsLoadedMsg struct {
	Subscriptions []azure.Subscription
	Done          bool
	Err           error
	Next          tea.Cmd
}

// CurrentSubscription returns the active subscription and whether one is set.
func (m Model) CurrentSubscription() (azure.Subscription, bool) {
	return m.CurrentSub, m.HasSubscription
}

// SetSubscription sets the active subscription without triggering navigation.
// Callers that need to navigate should call the app's own selectSubscription.
//
// This also dismisses the subscription picker overlay and clears any
// loading state the constructor may have set up. Apps' constructors open
// the picker when no subscription is present yet — when a parent (like
// the tabapp) provides one explicitly, that picker is no longer needed.
func (m *Model) SetSubscription(sub azure.Subscription) {
	m.CurrentSub = sub
	m.HasSubscription = true
	m.SubOverlay.Close()
	m.ClearLoading()
	m.Status = ""
}

// SetPreferredSubscription records a subscription ID that the app should
// auto-select once subscriptions are loaded. Used by the tabapp to honor
// per-tab subscription configuration.
func (m *Model) SetPreferredSubscription(id string) {
	m.PreferredSub = id
}

// TryApplyPreferredSubscription looks up the preferred subscription ID
// in the currently loaded Subscriptions list. If a match exists, the
// preferred ID is cleared (so it doesn't fire twice) and the matched
// subscription is returned with ok=true. The caller is responsible for
// actually applying it via SetSubscription / selectSubscription.
func (m *Model) TryApplyPreferredSubscription() (azure.Subscription, bool) {
	if m.PreferredSub == "" {
		return azure.Subscription{}, false
	}
	for _, s := range m.Subscriptions {
		if s.ID == m.PreferredSub {
			m.PreferredSub = ""
			return s, true
		}
	}
	return azure.Subscription{}, false
}

// HydrateSubscriptionsFromCache populates Subscriptions from the given loader
// without hitting Azure. Safe to call from an app constructor.
func (m *Model) HydrateSubscriptionsFromCache(loader *cache.Loader[azure.Subscription]) {
	if loader == nil {
		return
	}
	if cached, ok := loader.Get(""); ok {
		m.Subscriptions = cached
	}
}
