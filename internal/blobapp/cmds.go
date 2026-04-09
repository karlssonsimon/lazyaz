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

func fetchSubscriptionsCmd(svc *blob.Service, broker *cache.Broker[azure.Subscription], seed []azure.Subscription) tea.Cmd {
	cmd, _ := broker.Subscribe("", seed, func(ctx context.Context, send func([]azure.Subscription)) error {
		return svc.ListSubscriptions(ctx, send)
	}, func(p cache.Page[azure.Subscription]) tea.Msg {
		return appshell.SubscriptionsLoadedMsg{Subscriptions: p.Items, Done: p.Done, Err: p.Err, Next: p.Next}
	})
	return cmd
}

func fetchAccountsCmd(svc *blob.Service, broker *cache.Broker[blob.Account], subscriptionID string, seed []blob.Account) tea.Cmd {
	cmd, _ := broker.Subscribe(subscriptionID, seed, func(ctx context.Context, send func([]blob.Account)) error {
		return svc.DiscoverAccountsForSubscription(ctx, subscriptionID, send)
	}, func(p cache.Page[blob.Account]) tea.Msg {
		return accountsLoadedMsg{subscriptionID: subscriptionID, accounts: p.Items, done: p.Done, err: p.Err, next: p.Next}
	})
	return cmd
}

func fetchContainersCmd(svc *blob.Service, broker *cache.Broker[blob.ContainerInfo], account blob.Account, seed []blob.ContainerInfo) tea.Cmd {
	cmd, _ := broker.Subscribe(cache.Key(account.SubscriptionID, account.Name), seed, func(ctx context.Context, send func([]blob.ContainerInfo)) error {
		return svc.ListContainers(ctx, account, send)
	}, func(p cache.Page[blob.ContainerInfo]) tea.Msg {
		return containersLoadedMsg{account: account, containers: p.Items, done: p.Done, err: p.Err, next: p.Next}
	})
	return cmd
}

func fetchHierarchyBlobsCmd(svc *blob.Service, broker *cache.Broker[blob.BlobEntry], account blob.Account, containerName, prefix string, limit int, seed []blob.BlobEntry) tea.Cmd {
	key := blobsCacheKey(account.SubscriptionID, account.Name, containerName, prefix, false)
	cmd, _ := broker.Subscribe(key, seed, func(ctx context.Context, send func([]blob.BlobEntry)) error {
		return svc.ListBlobsLimited(ctx, account, containerName, prefix, limit, send)
	}, func(p cache.Page[blob.BlobEntry]) tea.Msg {
		return blobsLoadedMsg{account: account, container: containerName, prefix: prefix, loadAll: false, query: "", blobs: p.Items, done: p.Done, err: p.Err, next: p.Next}
	})
	return cmd
}

func fetchAllBlobsCmd(svc *blob.Service, broker *cache.Broker[blob.BlobEntry], account blob.Account, containerName, prefix string, seed []blob.BlobEntry) tea.Cmd {
	key := blobsCacheKey(account.SubscriptionID, account.Name, containerName, prefix, true)
	cmd, _ := broker.Subscribe(key, seed, func(ctx context.Context, send func([]blob.BlobEntry)) error {
		return svc.ListAllBlobs(ctx, account, containerName, send)
	}, func(p cache.Page[blob.BlobEntry]) tea.Msg {
		return blobsLoadedMsg{account: account, container: containerName, prefix: prefix, loadAll: true, query: "", blobs: p.Items, done: p.Done, err: p.Err, next: p.Next}
	})
	return cmd
}

func fetchAllBlobsWithPrefixCmd(svc *blob.Service, account blob.Account, containerName, currentPrefix, query string) tea.Cmd {
	effectivePrefix := blobSearchPrefix(currentPrefix, query)
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()

		var all []blob.BlobEntry
		err := svc.SearchBlobsByPrefix(ctx, account, containerName, effectivePrefix, 0, func(batch []blob.BlobEntry) {
			all = append(all, batch...)
		})
		return blobsLoadedMsg{
			account:   account,
			container: containerName,
			prefix:    currentPrefix,
			loadAll:   true,
			blobs:     all,
			done:      true,
			err:       err,
		}
	}
}

// fetchSearchBlobsCmd streams prefix-search results directly without
// caching. Prefix searches are ephemeral queries — caching them would
// pollute the store with one-off data that goes stale immediately.
func fetchSearchBlobsCmd(svc *blob.Service, account blob.Account, containerName, currentPrefix, query string, limit int) tea.Cmd {
	effectivePrefix := blobSearchPrefix(currentPrefix, query)
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()

		var all []blob.BlobEntry
		err := svc.SearchBlobsByPrefix(ctx, account, containerName, effectivePrefix, limit, func(batch []blob.BlobEntry) {
			all = append(all, batch...)
		})
		return blobsLoadedMsg{
			account:   account,
			container: containerName,
			prefix:    currentPrefix,
			query:     strings.TrimSpace(query),
			blobs:     all,
			done:      true,
			err:       err,
		}
	}
}

func downloadBlobToClipboardCmd(svc *blob.Service, account blob.Account, containerName, blobName string, size int64) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		data, err := svc.ReadBlobRange(ctx, account, containerName, blobName, 0, size)
		if err != nil {
			return blobContentClipboardMsg{blobName: blobName, err: err}
		}
		return blobContentClipboardMsg{blobName: blobName, content: string(data)}
	}
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
