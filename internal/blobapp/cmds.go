package blobapp

import (
	"context"
	"fmt"
	"strings"
	"time"

	"azure-storage/internal/appshell"
	"azure-storage/internal/azure"
	"azure-storage/internal/azure/blob"
	"azure-storage/internal/cache"

	tea "github.com/charmbracelet/bubbletea"
)

// fetch picks Fetch or FetchFresh based on the fresh flag.
func loaderFetch[T any](l *cache.Loader[T], fresh bool, key string, fn func(context.Context, func([]T)) error, wrap func(cache.Page[T]) tea.Msg) tea.Cmd {
	if fresh {
		return l.FetchFresh(key, fn, wrap)
	}
	return l.Fetch(key, fn, wrap)
}

func fetchSubscriptionsCmd(svc *blob.Service, loader *cache.Loader[azure.Subscription], fresh bool) tea.Cmd {
	return loaderFetch(loader, fresh, "", func(ctx context.Context, send func([]azure.Subscription)) error {
		ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
		defer cancel()
		return svc.ListSubscriptions(ctx, send)
	}, func(p cache.Page[azure.Subscription]) tea.Msg {
		return appshell.SubscriptionsLoadedMsg{Subscriptions: p.Items, Done: p.Done, Err: p.Err, Next: p.Next}
	})
}

func fetchAccountsCmd(svc *blob.Service, loader *cache.Loader[blob.Account], subscriptionID string, fresh bool, gen int) tea.Cmd {
	return loaderFetch(loader, fresh, subscriptionID, func(ctx context.Context, send func([]blob.Account)) error {
		ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
		defer cancel()
		return svc.DiscoverAccountsForSubscription(ctx, subscriptionID, send)
	}, func(p cache.Page[blob.Account]) tea.Msg {
		return accountsLoadedMsg{gen: gen, cached: p.Cached, subscriptionID: subscriptionID, accounts: p.Items, done: p.Done, err: p.Err, next: p.Next}
	})
}

func fetchContainersCmd(svc *blob.Service, loader *cache.Loader[blob.ContainerInfo], account blob.Account, fresh bool, gen int) tea.Cmd {
	return loaderFetch(loader, fresh, cache.Key(account.SubscriptionID, account.Name), func(ctx context.Context, send func([]blob.ContainerInfo)) error {
		ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
		defer cancel()
		return svc.ListContainers(ctx, account, send)
	}, func(p cache.Page[blob.ContainerInfo]) tea.Msg {
		return containersLoadedMsg{gen: gen, cached: p.Cached, account: account, containers: p.Items, done: p.Done, err: p.Err, next: p.Next}
	})
}

func fetchHierarchyBlobsCmd(svc *blob.Service, loader *cache.Loader[blob.BlobEntry], account blob.Account, containerName, prefix string, limit int, fresh bool, gen int) tea.Cmd {
	key := blobsCacheKey(account.SubscriptionID, account.Name, containerName, prefix, false)
	return loaderFetch(loader, fresh, key, func(ctx context.Context, send func([]blob.BlobEntry)) error {
		ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
		defer cancel()
		return svc.ListBlobsLimited(ctx, account, containerName, prefix, limit, send)
	}, func(p cache.Page[blob.BlobEntry]) tea.Msg {
		return blobsLoadedMsg{gen: gen, cached: p.Cached, account: account, container: containerName, prefix: prefix, loadAll: false, query: "", blobs: p.Items, done: p.Done, err: p.Err, next: p.Next}
	})
}

func fetchAllBlobsCmd(svc *blob.Service, loader *cache.Loader[blob.BlobEntry], account blob.Account, containerName, prefix string, fresh bool, gen int) tea.Cmd {
	key := blobsCacheKey(account.SubscriptionID, account.Name, containerName, prefix, true)
	return loaderFetch(loader, fresh, key, func(ctx context.Context, send func([]blob.BlobEntry)) error {
		ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
		defer cancel()
		return svc.ListAllBlobs(ctx, account, containerName, send)
	}, func(p cache.Page[blob.BlobEntry]) tea.Msg {
		return blobsLoadedMsg{gen: gen, cached: p.Cached, account: account, container: containerName, prefix: prefix, loadAll: true, query: "", blobs: p.Items, done: p.Done, err: p.Err, next: p.Next}
	})
}

// fetchSearchBlobsCmd does NOT take a gen — search results bypass the
// FetchSession merge system and go through handleSearchBlobsLoaded,
// which manages its own state in m.search.
func fetchSearchBlobsCmd(svc *blob.Service, loader *cache.Loader[blob.BlobEntry], account blob.Account, containerName, currentPrefix, query string, limit int, fresh bool) tea.Cmd {
	effectivePrefix := blobSearchPrefix(currentPrefix, query)
	// Search results use a unique key that won't collide with hierarchy/all caches.
	key := cache.Key("search", account.SubscriptionID, account.Name, containerName, effectivePrefix)
	return loaderFetch(loader, fresh, key, func(ctx context.Context, send func([]blob.BlobEntry)) error {
		ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
		defer cancel()
		return svc.SearchBlobsByPrefix(ctx, account, containerName, effectivePrefix, limit, send)
	}, func(p cache.Page[blob.BlobEntry]) tea.Msg {
		return blobsLoadedMsg{account: account, container: containerName, prefix: currentPrefix, loadAll: false, query: strings.TrimSpace(query), blobs: p.Items, done: p.Done, err: p.Err, next: p.Next}
	})
}

func downloadBlobsCmd(svc *blob.Service, account blob.Account, containerName string, blobNames []string, destinationRoot string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()

		results, err := svc.DownloadBlobs(ctx, account, containerName, blobNames, destinationRoot)
		msg := blobsDownloadedMsg{
			destinationRoot: destinationRoot,
			total:           len(blobNames),
			err:             err,
		}
		if err != nil {
			return msg
		}

		for _, result := range results {
			if result.Err != nil {
				msg.failed++
				if len(msg.failures) < 3 {
					msg.failures = append(msg.failures, fmt.Sprintf("%s: %v", result.BlobName, result.Err))
				}
				continue
			}
			msg.downloaded++
		}

		if msg.failed > 3 {
			msg.failures = append(msg.failures, fmt.Sprintf("... and %d more", msg.failed-3))
		}

		return msg
	}
}
