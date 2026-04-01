package sbapp

import (
	"azure-storage/internal/azure"
	"azure-storage/internal/azure/servicebus"
	"azure-storage/internal/cache"
)

// sbCache provides an in-memory, stale-while-revalidate cache for:
//
//	subscriptions → namespaces → entities → topic subscriptions
//
// Messages are not cached because they are ephemeral peek results.
type sbCache struct {
	subscriptions cache.Store[azure.Subscription]
	namespaces    cache.Store[servicebus.Namespace]
	entities      cache.Store[servicebus.Entity]
	topicSubs     cache.Store[servicebus.TopicSubscription]
}

func newCache() sbCache {
	return sbCache{
		subscriptions: cache.NewMap[azure.Subscription](),
		namespaces:    cache.NewMap[servicebus.Namespace](),
		entities:      cache.NewMap[servicebus.Entity](),
		topicSubs:     cache.NewMap[servicebus.TopicSubscription](),
	}
}
