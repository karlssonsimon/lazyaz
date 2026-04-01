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
	subscriptions cache.Store[azure.Subscription]
	vaults        cache.Store[keyvault.Vault]
	secrets       cache.Store[keyvault.Secret]
	versions      cache.Store[keyvault.SecretVersion]
}

func newCache() kvCache {
	return kvCache{
		subscriptions: cache.NewMap[azure.Subscription](),
		vaults:        cache.NewMap[keyvault.Vault](),
		secrets:       cache.NewMap[keyvault.Secret](),
		versions:      cache.NewMap[keyvault.SecretVersion](),
	}
}
