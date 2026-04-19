package blobapp

import (
	"github.com/karlssonsimon/lazyaz/internal/azure"
	"github.com/karlssonsimon/lazyaz/internal/azure/blob"
	"github.com/karlssonsimon/lazyaz/internal/cache"
)

// blobCache provides a stale-while-revalidate cache for:
//
//	subscriptions → storage accounts → containers → blobs
//
// Each Broker is shared across tabs — multiple subscribers to the same
// key share a single in-flight fetch.
type blobCache struct {
	subscriptions *cache.Broker[azure.Subscription]
	accounts      *cache.Broker[blob.Account]      // key: subscriptionID
	containers    *cache.Broker[blob.ContainerInfo] // key: subscriptionID, accountName
	blobs         *cache.Broker[blob.BlobEntry]     // key: subscriptionID, accountName, container, prefix, loadAll
}

func newCache(db *cache.DB) blobCache {
	if db != nil {
		return blobCache{
			subscriptions: cache.NewBroker(cache.NewStore[azure.Subscription](db, "subscriptions"), azure.SubscriptionKey),
			accounts:      cache.NewBroker(cache.NewStore[blob.Account](db, "blob_accounts"), accountKey),
			containers:    cache.NewBroker(cache.NewStore[blob.ContainerInfo](db, "blob_containers"), containerKey),
			blobs:         cache.NewBroker(cache.NewStore[blob.BlobEntry](db, "blobs"), blobEntryKey),
		}
	}
	return blobCache{
		subscriptions: cache.NewBroker(cache.NewMap[azure.Subscription](), azure.SubscriptionKey),
		accounts:      cache.NewBroker(cache.NewMap[blob.Account](), accountKey),
		containers:    cache.NewBroker(cache.NewMap[blob.ContainerInfo](), containerKey),
		blobs:         cache.NewBroker(cache.NewMap[blob.BlobEntry](), blobEntryKey),
	}
}

// BlobStores holds the shared brokers for blob resources, plus the
// persistent cache handle for usage tracking. Usage may be nil when
// the parent is running in in-memory mode.
// The parent tabapp owns these and passes them when creating tabs.
type BlobStores struct {
	Subscriptions *cache.Broker[azure.Subscription]
	Accounts      *cache.Broker[blob.Account]
	Containers    *cache.Broker[blob.ContainerInfo]
	Blobs         *cache.Broker[blob.BlobEntry]
	Usage         *cache.DB
}

// NewCacheWithStores creates a blobCache using pre-built shared brokers.
func NewCacheWithStores(s BlobStores) blobCache {
	return blobCache{
		subscriptions: s.Subscriptions,
		accounts:      s.Accounts,
		containers:    s.Containers,
		blobs:         s.Blobs,
	}
}

func blobsCacheKey(subscriptionID, accountName, container, prefix string, loadAll bool) string {
	allStr := "0"
	if loadAll {
		allStr = "1"
	}
	return cache.Key(subscriptionID, accountName, container, prefix, allStr)
}
