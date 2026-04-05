package core

import (
	"fmt"

	"azure-storage/internal/azure"
	"azure-storage/internal/azure/keyvault"
	"azure-storage/internal/ui"
)

type Pane int

const (
	SubscriptionsPane Pane = iota
	VaultsPane
	SecretsPane
	VersionsPane
	PreviewPane
)

type LoadKind int

const (
	LoadNone LoadKind = iota
	LoadSubscriptions
	LoadVaults
	LoadSecrets
	LoadVersions
	LoadPreviewSecret
)

type LoadRequest struct {
	Kind           LoadKind
	SubscriptionID string
	Vault          keyvault.Vault
	SecretName     string
	Version        string
	Force          bool
	Status         string
}

type Session struct {
	Focus               Pane
	Subscriptions       []azure.Subscription
	Vaults              []keyvault.Vault
	Secrets             []keyvault.Secret
	Versions            []keyvault.SecretVersion
	HasSubscription     bool
	CurrentSubscription azure.Subscription
	HasVault            bool
	CurrentVault        keyvault.Vault
	HasSecret           bool
	CurrentSecret       keyvault.Secret
	PreviewOpen         bool
	PreviewValue        string
	PreviewVersion      string
	Loading             bool
	Status              string
	LastErr             string
}

func NewSession() Session {
	return Session{Focus: SubscriptionsPane}
}

func (s *Session) BeginLoading(status string) {
	s.Loading = true
	s.LastErr = ""
	s.Status = status
}

func (s *Session) ClearError() { s.LastErr = "" }

func (s *Session) SetError(status string, err error) {
	s.Loading = false
	s.Status = status
	if err == nil {
		s.LastErr = ""
		return
	}
	s.LastErr = err.Error()
}

func (s *Session) SetStatus(status string) { s.Status = status }

func (s *Session) SelectSubscription(sub azure.Subscription) {
	s.CurrentSubscription = sub
	s.HasSubscription = true
	s.HasVault = false
	s.HasSecret = false
	s.CurrentVault = keyvault.Vault{}
	s.CurrentSecret = keyvault.Secret{}
	s.PreviewOpen = false
	s.PreviewValue = ""
	s.PreviewVersion = ""
	s.Focus = VaultsPane
	s.Vaults = nil
	s.Secrets = nil
	s.Versions = nil
}

func (s *Session) SelectVault(vault keyvault.Vault) {
	s.CurrentVault = vault
	s.HasVault = true
	s.HasSecret = false
	s.CurrentSecret = keyvault.Secret{}
	s.PreviewOpen = false
	s.PreviewValue = ""
	s.PreviewVersion = ""
	s.Focus = SecretsPane
	s.Secrets = nil
	s.Versions = nil
}

func (s *Session) SelectSecret(secret keyvault.Secret) {
	s.CurrentSecret = secret
	s.HasSecret = true
	s.PreviewOpen = false
	s.PreviewValue = ""
	s.PreviewVersion = ""
	s.Focus = VersionsPane
	s.Versions = nil
}

func (s *Session) NextFocus() {
	count := 4
	if s.PreviewOpen {
		count = 5
	}
	s.Focus = Pane((int(s.Focus) + 1) % count)
}

func (s *Session) PreviousFocus() {
	s.Focus--
	if s.Focus >= 0 {
		return
	}
	if s.PreviewOpen {
		s.Focus = PreviewPane
		return
	}
	s.Focus = VersionsPane
}

func (s *Session) NavigateLeft() string {
	switch s.Focus {
	case PreviewPane:
		s.PreviewOpen = false
		s.PreviewValue = ""
		s.PreviewVersion = ""
		s.Focus = VersionsPane
		return "Focus: versions"
	case VersionsPane:
		s.Focus = SecretsPane
		return "Focus: secrets"
	case SecretsPane:
		s.Focus = VaultsPane
		return "Focus: vaults"
	case VaultsPane:
		s.Focus = SubscriptionsPane
		return "Focus: subscriptions"
	default:
		return ""
	}
}

func (s *Session) Backspace() string {
	if s.Focus == PreviewPane {
		s.PreviewOpen = false
		s.PreviewValue = ""
		s.PreviewVersion = ""
		s.Focus = VersionsPane
		return "Focus: versions"
	}
	if s.Focus == VersionsPane {
		s.Focus = SecretsPane
		return "Focus: secrets"
	}
	return ""
}

func (s *Session) RefreshRequest() LoadRequest {
	if !s.HasSubscription {
		return LoadRequest{Kind: LoadSubscriptions, Force: true, Status: "Refreshing subscriptions..."}
	}
	if s.Focus == SubscriptionsPane {
		return LoadRequest{Kind: LoadSubscriptions, Force: true, Status: "Refreshing subscriptions..."}
	}
	if !s.HasVault || s.Focus == VaultsPane {
		return LoadRequest{Kind: LoadVaults, SubscriptionID: s.CurrentSubscription.ID, Force: true, Status: fmt.Sprintf("Loading key vaults in %s", ui.SubscriptionDisplayName(s.CurrentSubscription))}
	}
	if !s.HasSecret || s.Focus == SecretsPane {
		return LoadRequest{Kind: LoadSecrets, Vault: s.CurrentVault, Force: true, Status: fmt.Sprintf("Loading secrets in %s", s.CurrentVault.Name)}
	}
	if s.Focus == PreviewPane {
		return LoadRequest{Kind: LoadPreviewSecret, Vault: s.CurrentVault, SecretName: s.CurrentSecret.Name, Version: s.PreviewVersion, Status: fmt.Sprintf("Loading secret value for %s", s.CurrentSecret.Name)}
	}
	return LoadRequest{Kind: LoadVersions, Vault: s.CurrentVault, SecretName: s.CurrentSecret.Name, Force: true, Status: fmt.Sprintf("Loading versions for %s", s.CurrentSecret.Name)}
}

func (s *Session) SelectSubscriptionRequest(sub azure.Subscription) LoadRequest {
	s.SelectSubscription(sub)
	return LoadRequest{Kind: LoadVaults, SubscriptionID: sub.ID, Status: fmt.Sprintf("Loading key vaults in %s", ui.SubscriptionDisplayName(sub))}
}

func (s *Session) SelectVaultRequest(vault keyvault.Vault) LoadRequest {
	s.SelectVault(vault)
	return LoadRequest{Kind: LoadSecrets, Vault: vault, Status: fmt.Sprintf("Loading secrets in %s", vault.Name)}
}

func (s *Session) SelectSecretRequest(secret keyvault.Secret) LoadRequest {
	s.SelectSecret(secret)
	return LoadRequest{Kind: LoadVersions, Vault: s.CurrentVault, SecretName: secret.Name, Status: fmt.Sprintf("Loading versions for %s", secret.Name)}
}

func (s *Session) PreviewSecretRequest(version string) LoadRequest {
	if !s.HasVault {
		return LoadRequest{}
	}
	secretName := s.CurrentSecret.Name
	if s.Focus == VersionsPane {
		s.PreviewOpen = true
		s.PreviewVersion = version
		s.PreviewValue = ""
		s.Focus = PreviewPane
		return LoadRequest{Kind: LoadPreviewSecret, Vault: s.CurrentVault, SecretName: secretName, Version: version, Status: fmt.Sprintf("Fetching secret value for %s@%s...", secretName, version)}
	}
	if s.Focus == SecretsPane {
		s.PreviewOpen = true
		s.PreviewVersion = ""
		s.PreviewValue = ""
		s.Focus = PreviewPane
		return LoadRequest{Kind: LoadPreviewSecret, Vault: s.CurrentVault, SecretName: secretName, Status: fmt.Sprintf("Fetching secret value for %s...", secretName)}
	}
	return LoadRequest{}
}

func (s Session) AcceptVaultsResult(subscriptionID string) bool {
	return s.HasSubscription && s.CurrentSubscription.ID == subscriptionID
}

func (s Session) AcceptSecretsResult(vault keyvault.Vault) bool {
	return s.HasVault && s.CurrentVault.Name == vault.Name
}

func (s Session) AcceptVersionsResult(vault keyvault.Vault, secretName string) bool {
	return s.HasSecret && s.CurrentVault.Name == vault.Name && s.CurrentSecret.Name == secretName
}

func (s *Session) ApplySubscriptionsResult(subscriptions []azure.Subscription, done bool, err error) {
	if err != nil {
		s.SetError("Failed to load subscriptions", err)
		return
	}
	s.ClearError()
	s.Subscriptions = subscriptions
	if done {
		s.Loading = false
		s.Status = fmt.Sprintf("Loaded %d subscriptions.", len(subscriptions))
	}
}

func (s *Session) ApplyVaultsResult(subscriptionID string, vaults []keyvault.Vault, done bool, err error) bool {
	if !s.AcceptVaultsResult(subscriptionID) {
		return false
	}
	if err != nil {
		s.SetError(fmt.Sprintf("Failed to load key vaults in %s", ui.SubscriptionDisplayName(s.CurrentSubscription)), err)
		return true
	}
	s.ClearError()
	s.Vaults = vaults
	if done {
		s.Loading = false
		s.Status = fmt.Sprintf("Loaded %d vaults.", len(vaults))
	}
	return true
}

func (s *Session) ApplySecretsResult(vault keyvault.Vault, secrets []keyvault.Secret, done bool, err error) bool {
	if !s.AcceptSecretsResult(vault) {
		return false
	}
	if err != nil {
		s.SetError(fmt.Sprintf("Failed to load secrets in %s", vault.Name), err)
		return true
	}
	s.ClearError()
	s.Secrets = secrets
	if done {
		s.Loading = false
		s.Status = fmt.Sprintf("Loaded %d secrets from %s.", len(secrets), vault.Name)
	}
	return true
}

func (s *Session) ApplyVersionsResult(vault keyvault.Vault, secretName string, versions []keyvault.SecretVersion, done bool, err error) bool {
	if !s.AcceptVersionsResult(vault, secretName) {
		return false
	}
	if err != nil {
		s.SetError(fmt.Sprintf("Failed to load versions for %s", secretName), err)
		return true
	}
	s.ClearError()
	s.Versions = versions
	if done {
		s.Loading = false
		s.Status = fmt.Sprintf("Loaded %d versions for %s.", len(versions), secretName)
	}
	return true
}

func (s *Session) ApplyPreviewResult(secretName string, version string, value string, err error) {
	s.Loading = false
	if err != nil {
		s.LastErr = err.Error()
		s.Status = "Failed to load secret value"
		return
	}
	s.ClearError()
	s.PreviewOpen = true
	s.PreviewValue = value
	s.PreviewVersion = version
	s.Focus = PreviewPane
	if version != "" {
		s.Status = fmt.Sprintf("Previewing %s@%s", secretName, version)
		return
	}
	s.Status = fmt.Sprintf("Previewing %s", secretName)
}
