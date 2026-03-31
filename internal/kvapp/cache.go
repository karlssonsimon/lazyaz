package kvapp

import (
	"azure-storage/internal/azure"
	"azure-storage/internal/cache"
	"azure-storage/internal/keyvault"
)

// kvCache provides an in-memory, stale-while-revalidate cache for:
//
//	subscriptions → vaults → secrets → versions
type kvCache struct {
	subscriptions cache.Map[azure.Subscription]
	vaults        cache.Map[keyvault.Vault]
	secrets       cache.Map[keyvault.Secret]
	versions      cache.Map[keyvault.SecretVersion]
}

func newCache() kvCache {
	return kvCache{
		subscriptions: cache.NewMap[azure.Subscription](),
		vaults:        cache.NewMap[keyvault.Vault](),
		secrets:       cache.NewMap[keyvault.Secret](),
		versions:      cache.NewMap[keyvault.SecretVersion](),
	}
}
