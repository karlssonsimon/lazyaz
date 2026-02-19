package blobapp

import (
	"context"
	"fmt"
	"strings"
	"time"

	"azure-storage/internal/azure"

	tea "github.com/charmbracelet/bubbletea"
)

func loadSubscriptionsCmd(svc *azure.Service) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		subscriptions, err := svc.ListSubscriptions(ctx)
		return subscriptionsLoadedMsg{subscriptions: subscriptions, err: err}
	}
}

func loadAccountsForSubscriptionCmd(svc *azure.Service, subscriptionID string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		accounts, err := svc.DiscoverAccountsForSubscription(ctx, subscriptionID)
		return accountsLoadedMsg{subscriptionID: subscriptionID, accounts: accounts, err: err}
	}
}

func loadContainersCmd(svc *azure.Service, account azure.Account) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()

		containers, err := svc.ListContainers(ctx, account)
		return containersLoadedMsg{account: account, containers: containers, err: err}
	}
}

func loadHierarchyBlobsCmd(svc *azure.Service, account azure.Account, containerName, prefix string, limit int) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		blobs, err := svc.ListBlobsLimited(ctx, account, containerName, prefix, limit)
		return blobsLoadedMsg{account: account, container: containerName, prefix: prefix, loadAll: false, query: "", blobs: blobs, err: err}
	}
}

func loadAllBlobsCmd(svc *azure.Service, account azure.Account, containerName, prefix string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()

		blobs, err := svc.ListAllBlobs(ctx, account, containerName)
		return blobsLoadedMsg{account: account, container: containerName, prefix: prefix, loadAll: true, query: "", blobs: blobs, err: err}
	}
}

func searchBlobsByPrefixCmd(svc *azure.Service, account azure.Account, containerName, currentPrefix, query string, limit int) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()

		effectivePrefix := blobSearchPrefix(currentPrefix, query)
		blobs, err := svc.SearchBlobsByPrefix(ctx, account, containerName, effectivePrefix, limit)
		return blobsLoadedMsg{account: account, container: containerName, prefix: currentPrefix, loadAll: false, query: strings.TrimSpace(query), blobs: blobs, err: err}
	}
}

func downloadBlobsCmd(svc *azure.Service, account azure.Account, containerName string, blobNames []string, destinationRoot string) tea.Cmd {
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
