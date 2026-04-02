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

func newCache(db *cache.DB) sbCache {
	if db != nil {
		return sbCache{
			subscriptions: cache.NewLoader[azure.Subscription](cache.NewStore[azure.Subscription](db, "subscriptions")),
			namespaces:    cache.NewLoader[servicebus.Namespace](cache.NewStore[servicebus.Namespace](db, "sb_namespaces")),
			entities:      cache.NewLoader[servicebus.Entity](cache.NewStore[servicebus.Entity](db, "sb_entities")),
			topicSubs:     cache.NewLoader[servicebus.TopicSubscription](cache.NewStore[servicebus.TopicSubscription](db, "sb_topic_subs")),
		}
	}
	return sbCache{
		subscriptions: cache.NewLoader(cache.NewMap[azure.Subscription]()),
		namespaces:    cache.NewLoader(cache.NewMap[servicebus.Namespace]()),
		entities:      cache.NewLoader(cache.NewMap[servicebus.Entity]()),
		topicSubs:     cache.NewLoader(cache.NewMap[servicebus.TopicSubscription]()),
	}
}

// SBStores holds the shared cache stores for service bus resources.
type SBStores struct {
	Subscriptions cache.Store[azure.Subscription]
	Namespaces    cache.Store[servicebus.Namespace]
	Entities      cache.Store[servicebus.Entity]
	TopicSubs     cache.Store[servicebus.TopicSubscription]
}

// NewCacheWithStores creates an sbCache where each Loader wraps the
// provided shared stores.
func NewCacheWithStores(s SBStores) sbCache {
	return sbCache{
		subscriptions: cache.NewLoader(s.Subscriptions),
		namespaces:    cache.NewLoader(s.Namespaces),
		entities:      cache.NewLoader(s.Entities),
		topicSubs:     cache.NewLoader(s.TopicSubs),
	}
}
