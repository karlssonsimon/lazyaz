package kvapp

import (
	"github.com/karlssonsimon/lazyaz/internal/azure"
	"github.com/karlssonsimon/lazyaz/internal/azure/keyvault"
	"github.com/karlssonsimon/lazyaz/internal/cache"
)

// kvCache provides a stale-while-revalidate cache for:
//
//	subscriptions → vaults → secrets → versions
type kvCache struct {
	subscriptions *cache.Broker[azure.Subscription]
	vaults        *cache.Broker[keyvault.Vault]
	secrets       *cache.Broker[keyvault.Secret]
	versions      *cache.Broker[keyvault.SecretVersion]
}

func newCache(db *cache.DB) kvCache {
	if db != nil {
		return kvCache{
			subscriptions: cache.NewBroker(cache.NewStore[azure.Subscription](db, "subscriptions"), azure.SubscriptionKey),
			vaults:        cache.NewBroker(cache.NewStore[keyvault.Vault](db, "kv_vaults"), vaultKey),
			secrets:       cache.NewBroker(cache.NewStore[keyvault.Secret](db, "kv_secrets"), secretKey),
			versions:      cache.NewBroker(cache.NewStore[keyvault.SecretVersion](db, "kv_secret_versions"), versionKey),
		}
	}
	return kvCache{
		subscriptions: cache.NewBroker(cache.NewMap[azure.Subscription](), azure.SubscriptionKey),
		vaults:        cache.NewBroker(cache.NewMap[keyvault.Vault](), vaultKey),
		secrets:       cache.NewBroker(cache.NewMap[keyvault.Secret](), secretKey),
		versions:      cache.NewBroker(cache.NewMap[keyvault.SecretVersion](), versionKey),
	}
}

// KVStores holds the shared brokers for key vault resources.
type KVStores struct {
	Subscriptions *cache.Broker[azure.Subscription]
	Vaults        *cache.Broker[keyvault.Vault]
	Secrets       *cache.Broker[keyvault.Secret]
	Versions      *cache.Broker[keyvault.SecretVersion]
}

// NewCacheWithStores creates a kvCache using pre-built shared brokers.
func NewCacheWithStores(s KVStores) kvCache {
	return kvCache{
		subscriptions: s.Subscriptions,
		vaults:        s.Vaults,
		secrets:       s.Secrets,
		versions:      s.Versions,
	}
}
