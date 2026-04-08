package blobapp

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/karlssonsimon/lazyaz/internal/appshell"
	"github.com/karlssonsimon/lazyaz/internal/azure"
	"github.com/karlssonsimon/lazyaz/internal/azure/blob"
	"github.com/karlssonsimon/lazyaz/internal/cache"

	tea "charm.land/bubbletea/v2"
)

func fetchSubscriptionsCmd(svc *blob.Service, loader *cache.Loader[azure.Subscription], seed []azure.Subscription) tea.Cmd {
	return loader.Fetch("", seed, func(ctx context.Context, send func([]azure.Subscription)) error {
		ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
		defer cancel()
		return svc.ListSubscriptions(ctx, send)
	}, func(p cache.Page[azure.Subscription]) tea.Msg {
		return appshell.SubscriptionsLoadedMsg{Subscriptions: p.Items, Done: p.Done, Err: p.Err, Next: p.Next}
	})
}

func fetchAccountsCmd(svc *blob.Service, loader *cache.Loader[blob.Account], subscriptionID string, seed []blob.Account) tea.Cmd {
	return loader.Fetch(subscriptionID, seed, func(ctx context.Context, send func([]blob.Account)) error {
		ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
		defer cancel()
		return svc.DiscoverAccountsForSubscription(ctx, subscriptionID, send)
	}, func(p cache.Page[blob.Account]) tea.Msg {
		return accountsLoadedMsg{subscriptionID: subscriptionID, accounts: p.Items, done: p.Done, err: p.Err, next: p.Next}
	})
}

func fetchContainersCmd(svc *blob.Service, loader *cache.Loader[blob.ContainerInfo], account blob.Account, seed []blob.ContainerInfo) tea.Cmd {
	return loader.Fetch(cache.Key(account.SubscriptionID, account.Name), seed, func(ctx context.Context, send func([]blob.ContainerInfo)) error {
		ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
		defer cancel()
		return svc.ListContainers(ctx, account, send)
	}, func(p cache.Page[blob.ContainerInfo]) tea.Msg {
		return containersLoadedMsg{account: account, containers: p.Items, done: p.Done, err: p.Err, next: p.Next}
	})
}

func fetchHierarchyBlobsCmd(svc *blob.Service, loader *cache.Loader[blob.BlobEntry], account blob.Account, containerName, prefix string, limit int, seed []blob.BlobEntry) tea.Cmd {
	key := blobsCacheKey(account.SubscriptionID, account.Name, containerName, prefix, false)
	return loader.Fetch(key, seed, func(ctx context.Context, send func([]blob.BlobEntry)) error {
		ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
		defer cancel()
		return svc.ListBlobsLimited(ctx, account, containerName, prefix, limit, send)
	}, func(p cache.Page[blob.BlobEntry]) tea.Msg {
		return blobsLoadedMsg{account: account, container: containerName, prefix: prefix, loadAll: false, query: "", blobs: p.Items, done: p.Done, err: p.Err, next: p.Next}
	})
}

func fetchAllBlobsCmd(svc *blob.Service, loader *cache.Loader[blob.BlobEntry], account blob.Account, containerName, prefix string, seed []blob.BlobEntry) tea.Cmd {
	key := blobsCacheKey(account.SubscriptionID, account.Name, containerName, prefix, true)
	return loader.Fetch(key, seed, func(ctx context.Context, send func([]blob.BlobEntry)) error {
		ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
		defer cancel()
		return svc.ListAllBlobs(ctx, account, containerName, send)
	}, func(p cache.Page[blob.BlobEntry]) tea.Msg {
		return blobsLoadedMsg{account: account, container: containerName, prefix: prefix, loadAll: true, query: "", blobs: p.Items, done: p.Done, err: p.Err, next: p.Next}
	})
}

// fetchSearchBlobsCmd uses the loader's merge for consistency, but
// search results are scoped under a unique cache key so they don't
// pollute the hierarchy/load-all caches.
func fetchSearchBlobsCmd(svc *blob.Service, loader *cache.Loader[blob.BlobEntry], account blob.Account, containerName, currentPrefix, query string, limit int) tea.Cmd {
	effectivePrefix := blobSearchPrefix(currentPrefix, query)
	key := cache.Key("search", account.SubscriptionID, account.Name, containerName, effectivePrefix)
	return loader.Fetch(key, nil, func(ctx context.Context, send func([]blob.BlobEntry)) error {
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
