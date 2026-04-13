package azure

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armsubscriptions"
)

type Subscription struct {
	ID    string
	Name  string
	State string
}

type Tenant struct {
	ID          string
	DisplayName string
	Domain      string
}

// SubscriptionKey returns the stable identity used by cache.Broker to
// dedupe streamed subscriptions.
func SubscriptionKey(s Subscription) string { return s.ID }

func NewDefaultCredential() (azcore.TokenCredential, error) {
	return azidentity.NewDefaultAzureCredential(nil)
}

// NewCredentialForTenant creates a credential scoped to the given tenant.
// This reuses the existing az login session — no browser sign-in needed.
func NewCredentialForTenant(tenantID string) (azcore.TokenCredential, error) {
	return azidentity.NewDefaultAzureCredential(&azidentity.DefaultAzureCredentialOptions{
		TenantID:                   tenantID,
		AdditionallyAllowedTenants: []string{"*"},
	})
}

func ListTenants(ctx context.Context, cred azcore.TokenCredential) ([]Tenant, error) {
	client, err := armsubscriptions.NewTenantsClient(cred, nil)
	if err != nil {
		return nil, fmt.Errorf("create tenants client: %w", err)
	}

	var tenants []Tenant
	pager := client.NewListPager(nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list tenants: %w", err)
		}
		for _, t := range page.Value {
			if t == nil || t.TenantID == nil {
				continue
			}
			tenant := Tenant{ID: *t.TenantID}
			if t.DisplayName != nil {
				tenant.DisplayName = *t.DisplayName
			}
			if t.DefaultDomain != nil {
				tenant.Domain = *t.DefaultDomain
			}
			tenants = append(tenants, tenant)
		}
	}
	sort.Slice(tenants, func(i, j int) bool {
		nameI := strings.ToLower(strings.TrimSpace(tenants[i].DisplayName))
		nameJ := strings.ToLower(strings.TrimSpace(tenants[j].DisplayName))
		if nameI == nameJ {
			return tenants[i].ID < tenants[j].ID
		}
		return nameI < nameJ
	})
	return tenants, nil
}

func ListSubscriptions(ctx context.Context, cred azcore.TokenCredential, send func([]Subscription)) error {
	subscriptionsClient, err := armsubscriptions.NewClient(cred, nil)
	if err != nil {
		return fmt.Errorf("create subscriptions client: %w", err)
	}

	pager := subscriptionsClient.NewListPager(nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("list subscriptions: %w", err)
		}

		var batch []Subscription
		for _, subscription := range page.Value {
			if subscription == nil || subscription.SubscriptionID == nil {
				continue
			}

			entry := Subscription{ID: *subscription.SubscriptionID}
			if subscription.DisplayName != nil {
				entry.Name = *subscription.DisplayName
			}
			if subscription.State != nil {
				entry.State = string(*subscription.State)
			}

			batch = append(batch, entry)
		}
		if len(batch) > 0 {
			sort.Slice(batch, func(i, j int) bool {
				nameI := strings.ToLower(strings.TrimSpace(batch[i].Name))
				nameJ := strings.ToLower(strings.TrimSpace(batch[j].Name))
				if nameI == nameJ {
					return batch[i].ID < batch[j].ID
				}
				if nameI == "" {
					return false
				}
				if nameJ == "" {
					return true
				}
				return nameI < nameJ
			})
			send(batch)
		}
	}

	return nil
}
