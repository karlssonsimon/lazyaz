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
