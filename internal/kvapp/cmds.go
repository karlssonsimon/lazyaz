package kvapp

import (
	"context"
	"time"

	"github.com/karlssonsimon/lazyaz/internal/appshell"
	"github.com/karlssonsimon/lazyaz/internal/azure"
	"github.com/karlssonsimon/lazyaz/internal/azure/keyvault"
	"github.com/karlssonsimon/lazyaz/internal/cache"

	"github.com/atotto/clipboard"
	tea "charm.land/bubbletea/v2"
)

func fetchSubscriptionsCmd(svc *keyvault.Service, loader *cache.Loader[azure.Subscription], seed []azure.Subscription) tea.Cmd {
	return loader.Fetch("", seed, func(ctx context.Context, send func([]azure.Subscription)) error {
		ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
		defer cancel()
		return svc.ListSubscriptions(ctx, send)
	}, func(p cache.Page[azure.Subscription]) tea.Msg {
		return appshell.SubscriptionsLoadedMsg{Subscriptions: p.Items, Done: p.Done, Err: p.Err, Next: p.Next}
	})
}

func fetchVaultsCmd(svc *keyvault.Service, loader *cache.Loader[keyvault.Vault], subscriptionID string, seed []keyvault.Vault) tea.Cmd {
	return loader.Fetch(subscriptionID, seed, func(ctx context.Context, send func([]keyvault.Vault)) error {
		ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
		defer cancel()
		return svc.ListVaults(ctx, subscriptionID, send)
	}, func(p cache.Page[keyvault.Vault]) tea.Msg {
		return vaultsLoadedMsg{subscriptionID: subscriptionID, vaults: p.Items, done: p.Done, err: p.Err, next: p.Next}
	})
}

func fetchSecretsCmd(svc *keyvault.Service, loader *cache.Loader[keyvault.Secret], vault keyvault.Vault, seed []keyvault.Secret) tea.Cmd {
	return loader.Fetch(cache.Key(vault.SubscriptionID, vault.Name), seed, func(ctx context.Context, send func([]keyvault.Secret)) error {
		ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
		defer cancel()
		return svc.ListSecrets(ctx, vault, send)
	}, func(p cache.Page[keyvault.Secret]) tea.Msg {
		return secretsLoadedMsg{vault: vault, secrets: p.Items, done: p.Done, err: p.Err, next: p.Next}
	})
}

func fetchVersionsCmd(svc *keyvault.Service, loader *cache.Loader[keyvault.SecretVersion], vault keyvault.Vault, secretName string, seed []keyvault.SecretVersion) tea.Cmd {
	return loader.Fetch(cache.Key(vault.SubscriptionID, vault.Name, secretName), seed, func(ctx context.Context, send func([]keyvault.SecretVersion)) error {
		ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
		defer cancel()
		return svc.ListSecretVersions(ctx, vault, secretName, send)
	}, func(p cache.Page[keyvault.SecretVersion]) tea.Msg {
		return versionsLoadedMsg{vault: vault, secretName: secretName, versions: p.Items, done: p.Done, err: p.Err, next: p.Next}
	})
}

func yankSecretValueCmd(svc *keyvault.Service, vault keyvault.Vault, secretName string, version string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		value, err := svc.GetSecretValue(ctx, vault, secretName, version)
		if err != nil {
			return secretValueYankedMsg{secretName: secretName, version: version, err: err}
		}

		err = clipboard.WriteAll(value)
		return secretValueYankedMsg{secretName: secretName, version: version, err: err}
	}
}
