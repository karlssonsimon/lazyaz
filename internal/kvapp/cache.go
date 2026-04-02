package kvapp

import (
	"azure-storage/internal/azure"
	"azure-storage/internal/azure/keyvault"
	"azure-storage/internal/cache"
)

// kvCache provides an in-memory, stale-while-revalidate cache for:
//
//	subscriptions → vaults → secrets → versions
type kvCache struct {
	subscriptions *cache.Loader[azure.Subscription]
	vaults        *cache.Loader[keyvault.Vault]
	secrets       *cache.Loader[keyvault.Secret]
	versions      *cache.Loader[keyvault.SecretVersion]
}

func newCache(db *cache.DB) kvCache {
	if db != nil {
		return kvCache{
			subscriptions: cache.NewLoader[azure.Subscription](cache.NewStore[azure.Subscription](db, "subscriptions")),
			vaults:        cache.NewLoader[keyvault.Vault](cache.NewStore[keyvault.Vault](db, "kv_vaults")),
			secrets:       cache.NewLoader[keyvault.Secret](cache.NewStore[keyvault.Secret](db, "kv_secrets")),
			versions:      cache.NewLoader[keyvault.SecretVersion](cache.NewStore[keyvault.SecretVersion](db, "kv_secret_versions")),
		}
	}
	return kvCache{
		subscriptions: cache.NewLoader(cache.NewMap[azure.Subscription]()),
		vaults:        cache.NewLoader(cache.NewMap[keyvault.Vault]()),
		secrets:       cache.NewLoader(cache.NewMap[keyvault.Secret]()),
		versions:      cache.NewLoader(cache.NewMap[keyvault.SecretVersion]()),
	}
}

// KVStores holds the shared cache stores for key vault resources.
type KVStores struct {
	Subscriptions cache.Store[azure.Subscription]
	Vaults        cache.Store[keyvault.Vault]
	Secrets       cache.Store[keyvault.Secret]
	Versions      cache.Store[keyvault.SecretVersion]
}

// NewCacheWithStores creates a kvCache where each Loader wraps the
// provided shared stores.
func NewCacheWithStores(s KVStores) kvCache {
	return kvCache{
		subscriptions: cache.NewLoader(s.Subscriptions),
		vaults:        cache.NewLoader(s.Vaults),
		secrets:       cache.NewLoader(s.Secrets),
		versions:      cache.NewLoader(s.Versions),
	}
}
