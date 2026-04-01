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

func NewDefaultCredential() (azcore.TokenCredential, error) {
	return azidentity.NewDefaultAzureCredential(nil)
}

func ListSubscriptions(ctx context.Context, cred azcore.TokenCredential) ([]Subscription, error) {
	subscriptionsClient, err := armsubscriptions.NewClient(cred, nil)
	if err != nil {
		return nil, fmt.Errorf("create subscriptions client: %w", err)
	}

	subscriptions := make([]Subscription, 0)
	pager := subscriptionsClient.NewListPager(nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list subscriptions: %w", err)
		}

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

			subscriptions = append(subscriptions, entry)
		}
	}

	sort.Slice(subscriptions, func(i, j int) bool {
		nameI := strings.ToLower(strings.TrimSpace(subscriptions[i].Name))
		nameJ := strings.ToLower(strings.TrimSpace(subscriptions[j].Name))
		if nameI == nameJ {
			return subscriptions[i].ID < subscriptions[j].ID
		}
		if nameI == "" {
			return false
		}
		if nameJ == "" {
			return true
		}
		return nameI < nameJ
	})

	return subscriptions, nil
}
