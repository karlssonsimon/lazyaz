package blobapp

import (
	"github.com/karlssonsimon/lazyaz/internal/azure"
	"github.com/karlssonsimon/lazyaz/internal/azure/blob"
	"github.com/karlssonsimon/lazyaz/internal/cache"
)

// blobCache provides an in-memory, stale-while-revalidate cache for:
//
//	subscriptions → storage accounts → containers → blobs
type blobCache struct {
	subscriptions *cache.Loader[azure.Subscription]
	accounts      *cache.Loader[blob.Account]      // key: subscriptionID
	containers    *cache.Loader[blob.ContainerInfo] // key: subscriptionID, accountName
	blobs         *cache.Loader[blob.BlobEntry]     // key: subscriptionID, accountName, container, prefix, loadAll
}

func newCache(db *cache.DB) blobCache {
	if db != nil {
		return blobCache{
			subscriptions: cache.NewLoader(cache.NewStore[azure.Subscription](db, "subscriptions"), azure.SubscriptionKey),
			accounts:      cache.NewLoader(cache.NewStore[blob.Account](db, "blob_accounts"), accountKey),
			containers:    cache.NewLoader(cache.NewStore[blob.ContainerInfo](db, "blob_containers"), containerKey),
			blobs:         cache.NewLoader(cache.NewStore[blob.BlobEntry](db, "blobs"), blobEntryKey),
		}
	}
	return blobCache{
		subscriptions: cache.NewLoader(cache.NewMap[azure.Subscription](), azure.SubscriptionKey),
		accounts:      cache.NewLoader(cache.NewMap[blob.Account](), accountKey),
		containers:    cache.NewLoader(cache.NewMap[blob.ContainerInfo](), containerKey),
		blobs:         cache.NewLoader(cache.NewMap[blob.BlobEntry](), blobEntryKey),
	}
}

// BlobStores holds the shared cache stores for blob resources.
// The parent tabapp owns these and passes them when creating tabs.
type BlobStores struct {
	Subscriptions cache.Store[azure.Subscription]
	Accounts      cache.Store[blob.Account]
	Containers    cache.Store[blob.ContainerInfo]
	Blobs         cache.Store[blob.BlobEntry]
}

// NewCacheWithStores creates a blobCache where each Loader wraps the
// provided shared stores. Each tab gets its own Loaders (independent
// fetch lifecycle) but shares the underlying data.
func NewCacheWithStores(s BlobStores) blobCache {
	return blobCache{
		subscriptions: cache.NewLoader(s.Subscriptions, azure.SubscriptionKey),
		accounts:      cache.NewLoader(s.Accounts, accountKey),
		containers:    cache.NewLoader(s.Containers, containerKey),
		blobs:         cache.NewLoader(s.Blobs, blobEntryKey),
	}
}

func blobsCacheKey(subscriptionID, accountName, container, prefix string, loadAll bool) string {
	allStr := "0"
	if loadAll {
		allStr = "1"
	}
	return cache.Key(subscriptionID, accountName, container, prefix, allStr)
}
