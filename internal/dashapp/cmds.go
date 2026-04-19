package dashapp

import (
	"context"

	"github.com/karlssonsimon/lazyaz/internal/appshell"
	"github.com/karlssonsimon/lazyaz/internal/azure"
	"github.com/karlssonsimon/lazyaz/internal/azure/servicebus"
	"github.com/karlssonsimon/lazyaz/internal/cache"

	tea "charm.land/bubbletea/v2"
)

// namespacesLoadedMsg carries the result of a namespaces fetch for the
// active subscription. done=false indicates a streamed intermediate page.
type namespacesLoadedMsg struct {
	subscriptionID string
	namespaces     []servicebus.Namespace
	done           bool
	err            error
	next           tea.Cmd
}

// entitiesLoadedMsg carries entities for a single namespace, as part of
// the per-namespace fan-out.
type entitiesLoadedMsg struct {
	namespace servicebus.Namespace
	entities  []servicebus.Entity
	done      bool
	err       error
	next      tea.Cmd
}

// topicSubsLoadedMsg carries subscriptions for a single topic. Fetched
// lazily when a namespace has topics and the DLQ widget needs per-sub
// DLQ counts.
type topicSubsLoadedMsg struct {
	namespace servicebus.Namespace
	topicName string
	subs      []servicebus.TopicSubscription
	done      bool
	err       error
	next      tea.Cmd
}

func fetchSubscriptionsCmd(svc *servicebus.Service, broker *cache.Broker[azure.Subscription], seed []azure.Subscription) tea.Cmd {
	cmd, _ := broker.Subscribe("", seed, func(ctx context.Context, send func([]azure.Subscription)) error {
		return svc.ListSubscriptions(ctx, send)
	}, func(p cache.Page[azure.Subscription]) tea.Msg {
		return appshell.SubscriptionsLoadedMsg{Subscriptions: p.Items, Done: p.Done, Err: p.Err, Next: p.Next}
	})
	return cmd
}

func fetchNamespacesCmd(svc *servicebus.Service, broker *cache.Broker[servicebus.Namespace], subscriptionID string, seed []servicebus.Namespace) tea.Cmd {
	cmd, _ := broker.Subscribe(subscriptionID, seed, func(ctx context.Context, send func([]servicebus.Namespace)) error {
		return svc.ListNamespaces(ctx, subscriptionID, send)
	}, func(p cache.Page[servicebus.Namespace]) tea.Msg {
		return namespacesLoadedMsg{subscriptionID: subscriptionID, namespaces: p.Items, done: p.Done, err: p.Err, next: p.Next}
	})
	return cmd
}

func fetchEntitiesCmd(svc *servicebus.Service, broker *cache.Broker[servicebus.Entity], ns servicebus.Namespace) tea.Cmd {
	cacheKey := ns.Name
	cmd, _ := broker.Subscribe(cacheKey, nil, func(ctx context.Context, send func([]servicebus.Entity)) error {
		return svc.ListEntities(ctx, ns, send)
	}, func(p cache.Page[servicebus.Entity]) tea.Msg {
		return entitiesLoadedMsg{namespace: ns, entities: p.Items, done: p.Done, err: p.Err, next: p.Next}
	})
	return cmd
}

func fetchTopicSubsCmd(svc *servicebus.Service, broker *cache.Broker[servicebus.TopicSubscription], ns servicebus.Namespace, topicName string) tea.Cmd {
	cacheKey := ns.Name + "/" + topicName
	cmd, _ := broker.Subscribe(cacheKey, nil, func(ctx context.Context, send func([]servicebus.TopicSubscription)) error {
		return svc.ListTopicSubscriptions(ctx, ns, topicName, send)
	}, func(p cache.Page[servicebus.TopicSubscription]) tea.Msg {
		return topicSubsLoadedMsg{namespace: ns, topicName: topicName, subs: p.Items, done: p.Done, err: p.Err, next: p.Next}
	})
	return cmd
}
