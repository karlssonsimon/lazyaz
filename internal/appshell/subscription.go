package appshell

import (
	"azure-storage/internal/azure"
	"azure-storage/internal/cache"

	tea "github.com/charmbracelet/bubbletea"
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
