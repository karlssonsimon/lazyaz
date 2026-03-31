package blobapp

import (
	"azure-storage/internal/azure"
	"azure-storage/internal/cache"
)

// blobCache provides an in-memory, stale-while-revalidate cache for:
//
//	subscriptions → storage accounts → containers → blobs
type blobCache struct {
	subscriptions cache.Map[azure.Subscription]
	accounts      cache.Map[azure.Account]      // key: subscriptionID
	containers    cache.Map[azure.ContainerInfo] // key: subscriptionID, accountName
	blobs         cache.Map[azure.BlobEntry]     // key: subscriptionID, accountName, container, prefix, loadAll
}

func newCache() blobCache {
	return blobCache{
		subscriptions: cache.NewMap[azure.Subscription](),
		accounts:      cache.NewMap[azure.Account](),
		containers:    cache.NewMap[azure.ContainerInfo](),
		blobs:         cache.NewMap[azure.BlobEntry](),
	}
}

func blobsCacheKey(subscriptionID, accountName, container, prefix string, loadAll bool) string {
	allStr := "0"
	if loadAll {
		allStr = "1"
	}
	return cache.Key(subscriptionID, accountName, container, prefix, allStr)
}
