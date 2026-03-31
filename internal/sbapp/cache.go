package sbapp

import (
	"azure-storage/internal/azure"
	"azure-storage/internal/cache"
	"azure-storage/internal/servicebus"
)

// sbCache provides an in-memory, stale-while-revalidate cache for:
//
//	subscriptions → namespaces → entities → topic subscriptions
//
// Messages are not cached because they are ephemeral peek results.
type sbCache struct {
	subscriptions cache.Map[azure.Subscription]
	namespaces    cache.Map[servicebus.Namespace]
	entities      cache.Map[servicebus.Entity]
	topicSubs     cache.Map[servicebus.TopicSubscription]
}

func newCache() sbCache {
	return sbCache{
		subscriptions: cache.NewMap[azure.Subscription](),
		namespaces:    cache.NewMap[servicebus.Namespace](),
		entities:      cache.NewMap[servicebus.Entity](),
		topicSubs:     cache.NewMap[servicebus.TopicSubscription](),
	}
}
