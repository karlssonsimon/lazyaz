package kvapp

import (
	"github.com/karlssonsimon/lazyaz/internal/azure"
	"github.com/karlssonsimon/lazyaz/internal/azure/keyvault"
	"github.com/karlssonsimon/lazyaz/internal/cache"
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
			subscriptions: cache.NewLoader(cache.NewStore[azure.Subscription](db, "subscriptions"), azure.SubscriptionKey),
			vaults:        cache.NewLoader(cache.NewStore[keyvault.Vault](db, "kv_vaults"), vaultKey),
			secrets:       cache.NewLoader(cache.NewStore[keyvault.Secret](db, "kv_secrets"), secretKey),
			versions:      cache.NewLoader(cache.NewStore[keyvault.SecretVersion](db, "kv_secret_versions"), versionKey),
		}
	}
	return kvCache{
		subscriptions: cache.NewLoader(cache.NewMap[azure.Subscription](), azure.SubscriptionKey),
		vaults:        cache.NewLoader(cache.NewMap[keyvault.Vault](), vaultKey),
		secrets:       cache.NewLoader(cache.NewMap[keyvault.Secret](), secretKey),
		versions:      cache.NewLoader(cache.NewMap[keyvault.SecretVersion](), versionKey),
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
		subscriptions: cache.NewLoader(s.Subscriptions, azure.SubscriptionKey),
		vaults:        cache.NewLoader(s.Vaults, vaultKey),
		secrets:       cache.NewLoader(s.Secrets, secretKey),
		versions:      cache.NewLoader(s.Versions, versionKey),
	}
}
