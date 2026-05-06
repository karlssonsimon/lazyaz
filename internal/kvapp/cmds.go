package kvapp

import (
	"context"
	"time"

	"github.com/karlssonsimon/lazyaz/internal/appshell"
	"github.com/karlssonsimon/lazyaz/internal/azure"
	"github.com/karlssonsimon/lazyaz/internal/azure/keyvault"
	"github.com/karlssonsimon/lazyaz/internal/cache"

	tea "charm.land/bubbletea/v2"
	"github.com/atotto/clipboard"
)

func fetchSubscriptionsCmd(svc *keyvault.Service, broker *cache.Broker[azure.Subscription], tenantID string, seed []azure.Subscription) tea.Cmd {
	cmd, _ := broker.Subscribe(tenantID, seed, func(ctx context.Context, send func([]azure.Subscription)) error {
		return svc.ListSubscriptions(ctx, send)
	}, func(p cache.Page[azure.Subscription]) tea.Msg {
		return appshell.SubscriptionsLoadedMsg{Subscriptions: p.Items, Done: p.Done, Err: p.Err, Next: p.Next}
	})
	return cmd
}

func fetchVaultsCmd(svc *keyvault.Service, broker *cache.Broker[keyvault.Vault], subscriptionID string, seed []keyvault.Vault) tea.Cmd {
	cmd, _ := broker.Subscribe(subscriptionID, seed, func(ctx context.Context, send func([]keyvault.Vault)) error {
		return svc.ListVaults(ctx, subscriptionID, send)
	}, func(p cache.Page[keyvault.Vault]) tea.Msg {
		return vaultsLoadedMsg{subscriptionID: subscriptionID, vaults: p.Items, done: p.Done, err: p.Err, next: p.Next}
	})
	return cmd
}

func fetchSecretsCmd(svc *keyvault.Service, broker *cache.Broker[keyvault.Secret], vault keyvault.Vault, seed []keyvault.Secret) tea.Cmd {
	cmd, _ := broker.Subscribe(cache.Key(vault.SubscriptionID, vault.Name), seed, func(ctx context.Context, send func([]keyvault.Secret)) error {
		return svc.ListSecrets(ctx, vault, send)
	}, func(p cache.Page[keyvault.Secret]) tea.Msg {
		return secretsLoadedMsg{vault: vault, secrets: p.Items, done: p.Done, err: p.Err, next: p.Next}
	})
	return cmd
}

func fetchVersionsCmd(svc *keyvault.Service, broker *cache.Broker[keyvault.SecretVersion], vault keyvault.Vault, secretName string, seed []keyvault.SecretVersion) tea.Cmd {
	cmd, _ := broker.Subscribe(cache.Key(vault.SubscriptionID, vault.Name, secretName), seed, func(ctx context.Context, send func([]keyvault.SecretVersion)) error {
		return svc.ListSecretVersions(ctx, vault, secretName, send)
	}, func(p cache.Page[keyvault.SecretVersion]) tea.Msg {
		return versionsLoadedMsg{vault: vault, secretName: secretName, versions: p.Items, done: p.Done, err: p.Err, next: p.Next}
	})
	return cmd
}

// revealSecretValueCmd fetches a secret value for on-screen display only
// (no clipboard write — that's yankSecretValueCmd's job). Same Azure call
// as the yank path, different result message so update can route it to
// the reveal map without touching the clipboard.
func revealSecretValueCmd(svc *keyvault.Service, vault keyvault.Vault, secretName, version string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		value, err := svc.GetSecretValue(ctx, vault, secretName, version)
		return secretRevealedMsg{secretName: secretName, version: version, value: value, err: err}
	}
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
