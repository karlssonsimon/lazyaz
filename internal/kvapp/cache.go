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

func newCache() kvCache {
	return kvCache{
		subscriptions: cache.NewLoader(cache.NewMap[azure.Subscription]()),
		vaults:        cache.NewLoader(cache.NewMap[keyvault.Vault]()),
		secrets:       cache.NewLoader(cache.NewMap[keyvault.Secret]()),
		versions:      cache.NewLoader(cache.NewMap[keyvault.SecretVersion]()),
	}
}
