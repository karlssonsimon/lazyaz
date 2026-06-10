package appshell

import (
	"context"
	"fmt"
	"time"

	"github.com/karlssonsimon/lazyaz/internal/azure"
	"github.com/karlssonsimon/lazyaz/internal/cache"

	tea "charm.land/bubbletea/v2"
)

// SubscriptionLister is the slice of each app's service that
// FetchSubscriptionsCmd needs: a streaming subscription listing.
type SubscriptionLister interface {
	ListSubscriptions(ctx context.Context, send func([]azure.Subscription)) error
}

// FetchSubscriptionsCmd subscribes to the broker's subscription stream
// for tenantID and emits SubscriptionsLoadedMsg pages.
func FetchSubscriptionsCmd(svc SubscriptionLister, broker *cache.Broker[azure.Subscription], tenantID string, seed []azure.Subscription) tea.Cmd {
	cmd, _ := broker.Subscribe(tenantID, seed, func(ctx context.Context, send func([]azure.Subscription)) error {
		return svc.ListSubscriptions(ctx, send)
	}, func(p cache.Page[azure.Subscription]) tea.Msg {
		return SubscriptionsLoadedMsg{Subscriptions: p.Items, Done: p.Done, Err: p.Err, Next: p.Next}
	})
	return cmd
}

// SubscriptionsLoadedMsg is the shared result of FetchSubscriptionsCmd.
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
	// Keep the broker scope key aligned with the active subscription's
	// tenant. Subscription broker queries (HydrateSubscriptionsFromCache,
	// per-app subscription Set/Get sites) all key on m.Tenant — without
	// this, switching to a sub in a different tenant would silently read
	// from the wrong slot or fall back to an empty key.
	m.Tenant = sub.TenantID
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

// HandleSubscriptionsLoaded applies a streaming subscriptions page: it
// updates Subscriptions, keeps an open picker overlay's filtered view in
// sync, and on the final page caches the list in the broker and resolves
// the loading spinner.
//
// When the final page lands and a preferred subscription matches, it is
// returned with selectPreferred=true instead — the caller must run its own
// selectSubscription, then ClearLoading and resolve the spinner with the
// returned status on the post-selection model (selection is app-specific,
// so the shared code can't finish the spinner here).
func (m *Model) HandleSubscriptionsLoaded(
	msg SubscriptionsLoadedMsg,
	broker *cache.Broker[azure.Subscription],
) (matched azure.Subscription, status string, selectPreferred bool, cmd tea.Cmd) {
	if msg.Err != nil {
		m.ClearLoading()
		m.ResolveSpinner(m.LoadingSpinnerID, LevelError, fmt.Sprintf("Failed to load subscriptions: %s", msg.Err.Error()))
		return azure.Subscription{}, "", false, nil
	}

	m.Subscriptions = msg.Subscriptions
	// Keep the overlay's filtered view in sync with streaming results
	// so new subscriptions matching the user's query appear immediately.
	if m.SubOverlay.Active {
		m.SubOverlay.Refilter(m.Subscriptions)
	}

	if msg.Done {
		broker.Set(m.Tenant, msg.Subscriptions)
		status = fmt.Sprintf("Loaded %d subscriptions in %s", len(msg.Subscriptions), time.Since(m.LoadingStartedAt).Round(time.Millisecond))
		if !m.HasSubscription {
			if sub, ok := m.TryApplyPreferredSubscription(); ok {
				// The constructor opened the picker overlay; selectSubscription
				// drives navigation but doesn't dismiss it (the interactive
				// path is dismissed inside the overlay's HandleKey). Close
				// it here so the data loading behind it actually shows.
				m.SubOverlay.Close()
				return sub, status, true, nil
			}
			m.SubOverlay.Open()
		}
		m.ClearLoading()
		m.ResolveSpinner(m.LoadingSpinnerID, LevelSuccess, status)
		return azure.Subscription{}, "", false, nil
	}

	return azure.Subscription{}, "", false, msg.Next
}

// HydrateSubscriptionsFromCache populates Subscriptions from the given broker
// without hitting Azure. Safe to call from an app constructor.
func (m *Model) HydrateSubscriptionsFromCache(broker *cache.Broker[azure.Subscription]) {
	if broker == nil {
		return
	}
	if cached, ok := broker.Get(m.Tenant); ok {
		m.Subscriptions = cached
	}
}
