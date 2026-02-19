package ui

import (
	"strings"

	"azure-storage/internal/azure"
)

func SubscriptionDisplayName(sub azure.Subscription) string {
	if strings.TrimSpace(sub.Name) != "" {
		return sub.Name
	}
	if strings.TrimSpace(sub.ID) == "" {
		return "-"
	}
	return sub.ID
}
