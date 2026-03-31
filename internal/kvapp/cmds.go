package kvapp

import (
	"context"
	"time"

	"azure-storage/internal/keyvault"

	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"
)

func loadSubscriptionsCmd(svc *keyvault.Service) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		subs, err := svc.ListSubscriptions(ctx)
		return subscriptionsLoadedMsg{subscriptions: subs, err: err}
	}
}

func loadVaultsCmd(svc *keyvault.Service, subscriptionID string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		vaults, err := svc.ListVaults(ctx, subscriptionID)
		return vaultsLoadedMsg{subscriptionID: subscriptionID, vaults: vaults, err: err}
	}
}

func loadSecretsCmd(svc *keyvault.Service, vault keyvault.Vault) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		secrets, err := svc.ListSecrets(ctx, vault)
		return secretsLoadedMsg{vault: vault, secrets: secrets, err: err}
	}
}

func loadVersionsCmd(svc *keyvault.Service, vault keyvault.Vault, secretName string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		versions, err := svc.ListSecretVersions(ctx, vault, secretName)
		return versionsLoadedMsg{vault: vault, secretName: secretName, versions: versions, err: err}
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
