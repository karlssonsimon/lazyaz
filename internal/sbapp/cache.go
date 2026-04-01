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
	subscriptions *cache.Loader[azure.Subscription]
	namespaces    *cache.Loader[servicebus.Namespace]
	entities      *cache.Loader[servicebus.Entity]
	topicSubs     *cache.Loader[servicebus.TopicSubscription]
}

func newCache() sbCache {
	return sbCache{
		subscriptions: cache.NewLoader(cache.NewMap[azure.Subscription]()),
		namespaces:    cache.NewLoader(cache.NewMap[servicebus.Namespace]()),
		entities:      cache.NewLoader(cache.NewMap[servicebus.Entity]()),
		topicSubs:     cache.NewLoader(cache.NewMap[servicebus.TopicSubscription]()),
	}
}
