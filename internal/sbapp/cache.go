package sbapp

import (
	"github.com/karlssonsimon/lazyaz/internal/azure"
	"github.com/karlssonsimon/lazyaz/internal/azure/servicebus"
	"github.com/karlssonsimon/lazyaz/internal/cache"
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
			subscriptions: cache.NewLoader(cache.NewStore[azure.Subscription](db, "subscriptions"), azure.SubscriptionKey),
			namespaces:    cache.NewLoader(cache.NewStore[servicebus.Namespace](db, "sb_namespaces"), namespaceKey),
			entities:      cache.NewLoader(cache.NewStore[servicebus.Entity](db, "sb_entities"), entityKey),
			topicSubs:     cache.NewLoader(cache.NewStore[servicebus.TopicSubscription](db, "sb_topic_subs"), topicSubKey),
		}
	}
	return sbCache{
		subscriptions: cache.NewLoader(cache.NewMap[azure.Subscription](), azure.SubscriptionKey),
		namespaces:    cache.NewLoader(cache.NewMap[servicebus.Namespace](), namespaceKey),
		entities:      cache.NewLoader(cache.NewMap[servicebus.Entity](), entityKey),
		topicSubs:     cache.NewLoader(cache.NewMap[servicebus.TopicSubscription](), topicSubKey),
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
		subscriptions: cache.NewLoader(s.Subscriptions, azure.SubscriptionKey),
		namespaces:    cache.NewLoader(s.Namespaces, namespaceKey),
		entities:      cache.NewLoader(s.Entities, entityKey),
		topicSubs:     cache.NewLoader(s.TopicSubs, topicSubKey),
	}
}
