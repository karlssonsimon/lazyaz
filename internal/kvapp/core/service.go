package core

import (
	"errors"
	"fmt"

	"azure-storage/internal/azure"
	"azure-storage/internal/azure/keyvault"
)

type Action string

const (
	ActionRefresh            Action = "kv.refresh"
	ActionFocusNext          Action = "kv.focus.next"
	ActionFocusPrevious      Action = "kv.focus.previous"
	ActionNavigateLeft       Action = "kv.navigate.left"
	ActionBackspace          Action = "kv.navigate.backspace"
	ActionSelectSubscription Action = "kv.select.subscription"
	ActionSelectVault        Action = "kv.select.vault"
	ActionSelectSecret       Action = "kv.select.secret"
	ActionPreviewSecret      Action = "kv.preview.secret"
)

type ActionRequest struct {
	Action       Action
	Subscription azure.Subscription
	Vault        keyvault.Vault
	Secret       keyvault.Secret
	Version      string
}

type ActionResult struct {
	LoadRequest LoadRequest
	Status      string
}

type Service struct{ session *Session }

func NewService(session *Session) *Service {
	if session == nil {
		s := NewSession()
		session = &s
	}
	return &Service{session: session}
}

func (s *Service) Session() *Session { return s.session }

func (s *Service) Dispatch(req ActionRequest) (ActionResult, error) {
	session := s.session
	if session == nil {
		return ActionResult{}, fmt.Errorf("key vault service has no session")
	}
	switch req.Action {
	case ActionRefresh:
		return ActionResult{LoadRequest: session.RefreshRequest()}, nil
	case ActionFocusNext:
		session.NextFocus()
		return ActionResult{}, nil
	case ActionFocusPrevious:
		session.PreviousFocus()
		return ActionResult{}, nil
	case ActionNavigateLeft:
		return ActionResult{Status: session.NavigateLeft()}, nil
	case ActionBackspace:
		return ActionResult{Status: session.Backspace()}, nil
	case ActionSelectSubscription:
		return ActionResult{LoadRequest: session.SelectSubscriptionRequest(req.Subscription)}, nil
	case ActionSelectVault:
		return ActionResult{LoadRequest: session.SelectVaultRequest(req.Vault)}, nil
	case ActionSelectSecret:
		return ActionResult{LoadRequest: session.SelectSecretRequest(req.Secret)}, nil
	case ActionPreviewSecret:
		return ActionResult{LoadRequest: session.PreviewSecretRequest(req.Version)}, nil
	default:
		return ActionResult{}, fmt.Errorf("unsupported key vault action %q", req.Action)
	}
}

type Snapshot struct {
	Focus               string              `json:"focus"`
	HasSubscription     bool                `json:"has_subscription"`
	CurrentSubscription *SubscriptionState  `json:"current_subscription,omitempty"`
	HasVault            bool                `json:"has_vault"`
	CurrentVault        *VaultState         `json:"current_vault,omitempty"`
	HasSecret           bool                `json:"has_secret"`
	CurrentSecret       *SecretState        `json:"current_secret,omitempty"`
	PreviewOpen         bool                `json:"preview_open"`
	PreviewValue        string              `json:"preview_value,omitempty"`
	PreviewVersion      string              `json:"preview_version,omitempty"`
	Subscriptions       []SubscriptionState `json:"subscriptions"`
	Vaults              []VaultState        `json:"vaults"`
	Secrets             []SecretState       `json:"secrets"`
	Versions            []VersionState      `json:"versions"`
	Loading             bool                `json:"loading"`
	Status              string              `json:"status"`
	LastErr             string              `json:"last_err"`
}

type SubscriptionState struct{ ID, Name, State string }
type VaultState struct{ Name, SubscriptionID, VaultURI string }
type SecretState struct{ Name string }
type VersionState struct{ Version string }

func (s *Service) Snapshot() Snapshot {
	session := s.session
	snapshot := Snapshot{Focus: paneName(session.Focus), HasSubscription: session.HasSubscription, HasVault: session.HasVault, HasSecret: session.HasSecret, PreviewOpen: session.PreviewOpen, PreviewValue: session.PreviewValue, PreviewVersion: session.PreviewVersion, Loading: session.Loading, Status: session.Status, LastErr: session.LastErr}
	if session.HasSubscription {
		snapshot.CurrentSubscription = &SubscriptionState{ID: session.CurrentSubscription.ID, Name: session.CurrentSubscription.Name, State: session.CurrentSubscription.State}
	}
	if session.HasVault {
		snapshot.CurrentVault = &VaultState{Name: session.CurrentVault.Name, SubscriptionID: session.CurrentVault.SubscriptionID, VaultURI: session.CurrentVault.VaultURI}
	}
	if session.HasSecret {
		snapshot.CurrentSecret = &SecretState{Name: session.CurrentSecret.Name}
	}
	for _, sub := range session.Subscriptions {
		snapshot.Subscriptions = append(snapshot.Subscriptions, SubscriptionState{ID: sub.ID, Name: sub.Name, State: sub.State})
	}
	for _, vault := range session.Vaults {
		snapshot.Vaults = append(snapshot.Vaults, VaultState{Name: vault.Name, SubscriptionID: vault.SubscriptionID, VaultURI: vault.VaultURI})
	}
	for _, secret := range session.Secrets {
		snapshot.Secrets = append(snapshot.Secrets, SecretState{Name: secret.Name})
	}
	for _, version := range session.Versions {
		snapshot.Versions = append(snapshot.Versions, VersionState{Version: version.Version})
	}
	return snapshot
}

func paneName(pane Pane) string {
	switch pane {
	case SubscriptionsPane:
		return "subscriptions"
	case VaultsPane:
		return "vaults"
	case SecretsPane:
		return "secrets"
	case VersionsPane:
		return "versions"
	case PreviewPane:
		return "preview"
	default:
		return "subscriptions"
	}
}

func PaneFromName(name string) Pane {
	switch name {
	case "subscriptions":
		return SubscriptionsPane
	case "vaults":
		return VaultsPane
	case "secrets":
		return SecretsPane
	case "versions":
		return VersionsPane
	case "preview":
		return PreviewPane
	default:
		return SubscriptionsPane
	}
}

func ErrorFromResponse(message string) error {
	if message == "" {
		return nil
	}
	return errors.New(message)
}
