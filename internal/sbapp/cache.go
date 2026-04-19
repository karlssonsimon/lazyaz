package sbapp

import (
	"github.com/karlssonsimon/lazyaz/internal/azure"
	"github.com/karlssonsimon/lazyaz/internal/azure/servicebus"
	"github.com/karlssonsimon/lazyaz/internal/cache"
)

// sbCache provides a stale-while-revalidate cache for:
//
//	subscriptions → namespaces → entities → topic subscriptions
//
// Messages are not cached because they are ephemeral peek results.
type sbCache struct {
	subscriptions *cache.Broker[azure.Subscription]
	namespaces    *cache.Broker[servicebus.Namespace]
	entities      *cache.Broker[servicebus.Entity]
	topicSubs     *cache.Broker[servicebus.TopicSubscription]
}

func newCache(db *cache.DB) sbCache {
	if db != nil {
		return sbCache{
			subscriptions: cache.NewBroker(cache.NewStore[azure.Subscription](db, "subscriptions"), azure.SubscriptionKey),
			namespaces:    cache.NewBroker(cache.NewStore[servicebus.Namespace](db, "sb_namespaces"), namespaceKey),
			entities:      cache.NewBroker(cache.NewStore[servicebus.Entity](db, "sb_entities"), entityKey),
			topicSubs:     cache.NewBroker(cache.NewStore[servicebus.TopicSubscription](db, "sb_topic_subs"), topicSubKey),
		}
	}
	return sbCache{
		subscriptions: cache.NewBroker(cache.NewMap[azure.Subscription](), azure.SubscriptionKey),
		namespaces:    cache.NewBroker(cache.NewMap[servicebus.Namespace](), namespaceKey),
		entities:      cache.NewBroker(cache.NewMap[servicebus.Entity](), entityKey),
		topicSubs:     cache.NewBroker(cache.NewMap[servicebus.TopicSubscription](), topicSubKey),
	}
}

// SBStores holds the shared brokers for service bus resources, plus
// the persistent cache handle for usage tracking. Usage may be nil
// when the parent is running in in-memory mode.
type SBStores struct {
	Subscriptions *cache.Broker[azure.Subscription]
	Namespaces    *cache.Broker[servicebus.Namespace]
	Entities      *cache.Broker[servicebus.Entity]
	TopicSubs     *cache.Broker[servicebus.TopicSubscription]
	Usage         *cache.DB
}

// NewCacheWithStores creates an sbCache using pre-built shared brokers.
func NewCacheWithStores(s SBStores) sbCache {
	return sbCache{
		subscriptions: s.Subscriptions,
		namespaces:    s.Namespaces,
		entities:      s.Entities,
		topicSubs:     s.TopicSubs,
	}
}
