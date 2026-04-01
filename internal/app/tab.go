package app

import (
	"azure-storage/internal/azure"
	"azure-storage/internal/azure/blob"
	"azure-storage/internal/azure/keyvault"
	"azure-storage/internal/azure/servicebus"
	"azure-storage/internal/cache"

	tea "github.com/charmbracelet/bubbletea"
)

// TabKind identifies which type of explorer a tab contains.
type TabKind int

const (
	TabBlob TabKind = iota
	TabServiceBus
	TabKeyVault
)

func (k TabKind) String() string {
	switch k {
	case TabBlob:
		return "Blob"
	case TabServiceBus:
		return "Service Bus"
	case TabKeyVault:
		return "Key Vault"
	default:
		return "Unknown"
	}
}

// Tab represents a single tab in the application.
type Tab struct {
	ID    int
	Kind  TabKind
	Model tea.Model
}

// sharedStores holds all shared cache stores owned by the parent.
type sharedStores struct {
	subscriptions *cache.Map[azure.Subscription]

	blobAccounts   *cache.Map[blob.Account]
	blobContainers *cache.Map[blob.ContainerInfo]
	blobs          *cache.Map[blob.BlobEntry]

	sbNamespaces *cache.Map[servicebus.Namespace]
	sbEntities   *cache.Map[servicebus.Entity]
	sbTopicSubs  *cache.Map[servicebus.TopicSubscription]

	kvVaults   *cache.Map[keyvault.Vault]
	kvSecrets  *cache.Map[keyvault.Secret]
	kvVersions *cache.Map[keyvault.SecretVersion]
}

func newSharedStores() sharedStores {
	return sharedStores{
		subscriptions: cache.NewMap[azure.Subscription](),

		blobAccounts:   cache.NewMap[blob.Account](),
		blobContainers: cache.NewMap[blob.ContainerInfo](),
		blobs:          cache.NewMap[blob.BlobEntry](),

		sbNamespaces: cache.NewMap[servicebus.Namespace](),
		sbEntities:   cache.NewMap[servicebus.Entity](),
		sbTopicSubs:  cache.NewMap[servicebus.TopicSubscription](),

		kvVaults:   cache.NewMap[keyvault.Vault](),
		kvSecrets:  cache.NewMap[keyvault.Secret](),
		kvVersions: cache.NewMap[keyvault.SecretVersion](),
	}
}
