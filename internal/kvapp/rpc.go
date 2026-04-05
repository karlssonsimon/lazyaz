package kvapp

import (
	"fmt"

	"azure-storage/internal/azure"
	"azure-storage/internal/azure/keyvault"
	kvcore "azure-storage/internal/kvapp/core"
	kvrpc "azure-storage/internal/kvapp/rpc"
	"azure-storage/internal/ui"

	tea "github.com/charmbracelet/bubbletea"
)

type rpcSessionCreatedMsg struct {
	snapshot kvcore.Snapshot
	err      error
}

type rpcStateLoadedMsg struct {
	snapshot kvcore.Snapshot
	err      error
}

type rpcActionLoadedMsg struct {
	result kvrpc.ActionInvokeResult
	err    error
}

func (m Model) rpcEnabled() bool { return m.client != nil }

func rpcCreateSessionCmd(client *kvrpc.Client) tea.Cmd {
	return func() tea.Msg {
		snapshot, err := client.CreateSession()
		return rpcSessionCreatedMsg{snapshot: snapshot, err: err}
	}
}

func rpcActionCmd(client *kvrpc.Client, req kvcore.ActionRequest) tea.Cmd {
	return func() tea.Msg {
		result, err := client.InvokeAction(req)
		return rpcActionLoadedMsg{result: result, err: err}
	}
}

func (m *Model) applySnapshot(snapshot kvcore.Snapshot) {
	m.loading = snapshot.Loading
	m.status = snapshot.Status
	m.lastErr = snapshot.LastErr
	m.hasSubscription = snapshot.HasSubscription
	if snapshot.CurrentSubscription != nil {
		m.currentSub = azure.Subscription{ID: snapshot.CurrentSubscription.ID, Name: snapshot.CurrentSubscription.Name, State: snapshot.CurrentSubscription.State}
	} else {
		m.currentSub = azure.Subscription{}
	}
	m.hasVault = snapshot.HasVault
	if snapshot.CurrentVault != nil {
		m.currentVault = keyvault.Vault{Name: snapshot.CurrentVault.Name, SubscriptionID: snapshot.CurrentVault.SubscriptionID, VaultURI: snapshot.CurrentVault.VaultURI}
	} else {
		m.currentVault = keyvault.Vault{}
	}
	m.hasSecret = snapshot.HasSecret
	if snapshot.CurrentSecret != nil {
		m.currentSecret = keyvault.Secret{Name: snapshot.CurrentSecret.Name}
	} else {
		m.currentSecret = keyvault.Secret{}
	}
	m.focus = int(kvcore.PaneFromName(snapshot.Focus))

	m.subscriptions = nil
	for _, sub := range snapshot.Subscriptions {
		m.subscriptions = append(m.subscriptions, azure.Subscription{ID: sub.ID, Name: sub.Name, State: sub.State})
	}
	m.vaults = nil
	for _, vault := range snapshot.Vaults {
		m.vaults = append(m.vaults, keyvault.Vault{Name: vault.Name, SubscriptionID: vault.SubscriptionID, VaultURI: vault.VaultURI})
	}
	m.secrets = nil
	for _, secret := range snapshot.Secrets {
		m.secrets = append(m.secrets, keyvault.Secret{Name: secret.Name})
	}
	m.versions = nil
	for _, version := range snapshot.Versions {
		m.versions = append(m.versions, keyvault.SecretVersion{Version: version.Version})
	}
	ui.SetItemsPreserveIndex(&m.vaultsList, vaultsToItems(m.vaults))
	ui.SetItemsPreserveIndex(&m.secretsList, secretsToItems(m.secrets))
	ui.SetItemsPreserveIndex(&m.versionsList, versionsToItems(m.versions))
	m.resize()
}

func (m Model) rpcRefresh() tea.Cmd {
	return rpcActionCmd(m.client, kvcore.ActionRequest{Action: kvcore.ActionRefresh})
}

func (m Model) rpcSelectSubscription(sub azure.Subscription) tea.Cmd {
	return rpcActionCmd(m.client, kvcore.ActionRequest{Action: kvcore.ActionSelectSubscription, Subscription: sub})
}

func (m Model) rpcSelectVault(vault keyvault.Vault) tea.Cmd {
	return rpcActionCmd(m.client, kvcore.ActionRequest{Action: kvcore.ActionSelectVault, Vault: vault})
}

func (m Model) rpcSelectSecret(secret keyvault.Secret) tea.Cmd {
	return rpcActionCmd(m.client, kvcore.ActionRequest{Action: kvcore.ActionSelectSecret, Secret: secret})
}

func (m Model) rpcNavigateLeft() tea.Cmd {
	return rpcActionCmd(m.client, kvcore.ActionRequest{Action: kvcore.ActionNavigateLeft})
}

func (m Model) rpcBackspace() tea.Cmd {
	return rpcActionCmd(m.client, kvcore.ActionRequest{Action: kvcore.ActionBackspace})
}

func (m Model) rpcFocusNext() tea.Cmd {
	return rpcActionCmd(m.client, kvcore.ActionRequest{Action: kvcore.ActionFocusNext})
}

func (m Model) rpcFocusPrevious() tea.Cmd {
	return rpcActionCmd(m.client, kvcore.ActionRequest{Action: kvcore.ActionFocusPrevious})
}

func (m Model) rpcYank(version string) tea.Cmd {
	return rpcActionCmd(m.client, kvcore.ActionRequest{Action: kvcore.ActionPreviewSecret, Version: version})
}

func (m *Model) handleRPCMsg(msg tea.Msg) (tea.Model, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case rpcSessionCreatedMsg:
		if msg.err != nil {
			m.loading = false
			m.lastErr = msg.err.Error()
			m.status = "Failed to create rpc session"
			return m, nil, true
		}
		m.sessionID = m.client.Session()
		m.applySnapshot(msg.snapshot)
		if m.hasSubscription {
			return m, m.rpcSelectSubscription(m.currentSub), true
		}
		return m, m.rpcRefresh(), true
	case rpcStateLoadedMsg:
		if msg.err != nil {
			m.loading = false
			m.lastErr = msg.err.Error()
			m.status = "Failed to load rpc state"
			return m, nil, true
		}
		m.applySnapshot(msg.snapshot)
		return m, nil, true
	case rpcActionLoadedMsg:
		if msg.err != nil {
			m.loading = false
			m.lastErr = msg.err.Error()
			m.status = "RPC action failed"
			return m, nil, true
		}
		m.applySnapshot(msg.result.State)
		return m, nil, true
	default:
		return m, nil, false
	}
}

func (m Model) rpcDebugState() string {
	return fmt.Sprintf("rpc=%t session=%t", m.rpcEnabled(), m.sessionID != "")
}
