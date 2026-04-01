package keyvault

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"azure-storage/internal/azure"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/keyvault/armkeyvault"
	"github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azsecrets"
)

type Vault struct {
	Name           string
	SubscriptionID string
	ResourceGroup  string
	VaultURI       string
}

type Secret struct {
	Name        string
	ContentType string
	Enabled     bool
	CreatedOn   time.Time
	UpdatedOn   time.Time
}

type SecretVersion struct {
	Version     string
	ContentType string
	Enabled     bool
	CreatedOn   time.Time
	UpdatedOn   time.Time
	ExpiresOn   time.Time
}

type Service struct {
	cred    azcore.TokenCredential
	mu      sync.Mutex
	clients map[string]*azsecrets.Client
}

func NewService(cred azcore.TokenCredential) *Service {
	return &Service{
		cred:    cred,
		clients: make(map[string]*azsecrets.Client),
	}
}

func (s *Service) getClient(vaultURI string) (*azsecrets.Client, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if c, ok := s.clients[vaultURI]; ok {
		return c, nil
	}

	c, err := azsecrets.NewClient(vaultURI, s.cred, nil)
	if err != nil {
		return nil, fmt.Errorf("create secrets client for %s: %w", vaultURI, err)
	}

	s.clients[vaultURI] = c
	return c, nil
}

func (s *Service) ListSubscriptions(ctx context.Context, send func([]azure.Subscription)) error {
	return azure.ListSubscriptions(ctx, s.cred, send)
}

func (s *Service) ListVaults(ctx context.Context, subscriptionID string, send func([]Vault)) error {
	client, err := armkeyvault.NewVaultsClient(subscriptionID, s.cred, nil)
	if err != nil {
		return fmt.Errorf("create vaults client: %w", err)
	}

	pager := client.NewListBySubscriptionPager(nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("list key vaults: %w", err)
		}

		var batch []Vault
		for _, v := range page.Value {
			if v == nil || v.Name == nil {
				continue
			}

			entry := Vault{
				Name:           *v.Name,
				SubscriptionID: subscriptionID,
				ResourceGroup:  parseResourceGroup(v.ID),
			}
			if v.Properties != nil && v.Properties.VaultURI != nil {
				entry.VaultURI = strings.TrimSuffix(*v.Properties.VaultURI, "/")
			}
			if entry.VaultURI == "" {
				entry.VaultURI = fmt.Sprintf("https://%s.vault.azure.net", *v.Name)
			}

			batch = append(batch, entry)
		}
		if len(batch) > 0 {
			sort.Slice(batch, func(i, j int) bool {
				return strings.ToLower(batch[i].Name) < strings.ToLower(batch[j].Name)
			})
			send(batch)
		}
	}

	return nil
}

func (s *Service) ListSecrets(ctx context.Context, vault Vault, send func([]Secret)) error {
	client, err := s.getClient(vault.VaultURI)
	if err != nil {
		return err
	}

	pager := client.NewListSecretPropertiesPager(nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("list secrets in %s: %w", vault.Name, err)
		}

		var batch []Secret
		for _, sp := range page.Value {
			if sp == nil || sp.ID == nil {
				continue
			}

			entry := Secret{
				Name: sp.ID.Name(),
			}
			if sp.ContentType != nil {
				entry.ContentType = *sp.ContentType
			}
			if sp.Attributes != nil {
				if sp.Attributes.Enabled != nil {
					entry.Enabled = *sp.Attributes.Enabled
				}
				if sp.Attributes.Created != nil {
					entry.CreatedOn = *sp.Attributes.Created
				}
				if sp.Attributes.Updated != nil {
					entry.UpdatedOn = *sp.Attributes.Updated
				}
			}

			batch = append(batch, entry)
		}
		if len(batch) > 0 {
			sort.Slice(batch, func(i, j int) bool {
				return strings.ToLower(batch[i].Name) < strings.ToLower(batch[j].Name)
			})
			send(batch)
		}
	}

	return nil
}

func (s *Service) ListSecretVersions(ctx context.Context, vault Vault, secretName string, send func([]SecretVersion)) error {
	client, err := s.getClient(vault.VaultURI)
	if err != nil {
		return err
	}

	pager := client.NewListSecretPropertiesVersionsPager(secretName, nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("list secret versions for %s in %s: %w", secretName, vault.Name, err)
		}

		var batch []SecretVersion
		for _, v := range page.Value {
			if v == nil || v.ID == nil {
				continue
			}

			entry := SecretVersion{
				Version: v.ID.Version(),
			}
			if v.ContentType != nil {
				entry.ContentType = *v.ContentType
			}
			if v.Attributes != nil {
				if v.Attributes.Enabled != nil {
					entry.Enabled = *v.Attributes.Enabled
				}
				if v.Attributes.Created != nil {
					entry.CreatedOn = *v.Attributes.Created
				}
				if v.Attributes.Updated != nil {
					entry.UpdatedOn = *v.Attributes.Updated
				}
				if v.Attributes.Expires != nil {
					entry.ExpiresOn = *v.Attributes.Expires
				}
			}

			batch = append(batch, entry)
		}
		if len(batch) > 0 {
			sort.Slice(batch, func(i, j int) bool {
				return batch[i].CreatedOn.After(batch[j].CreatedOn)
			})
			send(batch)
		}
	}

	return nil
}

func (s *Service) GetSecretValue(ctx context.Context, vault Vault, secretName string, version string) (string, error) {
	client, err := s.getClient(vault.VaultURI)
	if err != nil {
		return "", err
	}

	resp, err := client.GetSecret(ctx, secretName, version, nil)
	if err != nil {
		return "", fmt.Errorf("get secret %s: %w", secretName, err)
	}

	if resp.Value == nil {
		return "", nil
	}

	return *resp.Value, nil
}

func parseResourceGroup(id *string) string {
	if id == nil {
		return ""
	}
	parts := strings.Split(*id, "/")
	for i, part := range parts {
		if strings.EqualFold(part, "resourceGroups") && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return ""
}
