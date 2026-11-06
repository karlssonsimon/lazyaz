package acapp

import (
	"github.com/karlssonsimon/lazyaz/internal/azure"
	"github.com/karlssonsimon/lazyaz/internal/azure/appconfig"
	"github.com/karlssonsimon/lazyaz/internal/cache"
)

// acCache is the stale-while-revalidate cache for
//
//	subscriptions → stores → settings
type acCache struct {
	subscriptions *cache.Broker[azure.Subscription]
	stores        *cache.Broker[appconfig.Store]
	settings      *cache.Broker[appconfig.Setting]
}

func newCache(db *cache.DB) acCache {
	if db != nil {
		return acCache{
			subscriptions: cache.NewBroker(cache.NewStore[azure.Subscription](db, "subscriptions"), azure.SubscriptionKey),
			stores:        cache.NewBroker(cache.NewStore[appconfig.Store](db, "ac_stores"), appconfig.StoreKey),
			settings:      cache.NewBroker(cache.NewStore[appconfig.Setting](db, "ac_settings"), appconfig.SettingKey),
		}
	}
	return acCache{
		subscriptions: cache.NewBroker(cache.NewMap[azure.Subscription](), azure.SubscriptionKey),
		stores:        cache.NewBroker(cache.NewMap[appconfig.Store](), appconfig.StoreKey),
		settings:      cache.NewBroker(cache.NewMap[appconfig.Setting](), appconfig.SettingKey),
	}
}

// ACStores holds the shared brokers for App Configuration resources.
type ACStores struct {
	Subscriptions *cache.Broker[azure.Subscription]
	Stores        *cache.Broker[appconfig.Store]
	Settings      *cache.Broker[appconfig.Setting]
}

// NewCacheWithStores creates an acCache from shared brokers; nil ones
// fall back to private in-memory maps.
func NewCacheWithStores(s ACStores) acCache {
	c := acCache{subscriptions: s.Subscriptions, stores: s.Stores, settings: s.Settings}
	if c.subscriptions == nil {
		c.subscriptions = cache.NewBroker(cache.NewMap[azure.Subscription](), azure.SubscriptionKey)
	}
	if c.stores == nil {
		c.stores = cache.NewBroker(cache.NewMap[appconfig.Store](), appconfig.StoreKey)
	}
	if c.settings == nil {
		c.settings = cache.NewBroker(cache.NewMap[appconfig.Setting](), appconfig.SettingKey)
	}
	return c
}
