package app

import (
	"strings"

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

// TabKindFromString parses a config-supplied tab kind name into a
// TabKind. Recognized values (case-insensitive): "blob", "servicebus",
// "keyvault". Returns ok=false on anything else so the caller can warn.
func TabKindFromString(s string) (TabKind, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "blob":
		return TabBlob, true
	case "servicebus", "service-bus", "service_bus":
		return TabServiceBus, true
	case "keyvault", "key-vault", "key_vault":
		return TabKeyVault, true
	default:
		return 0, false
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
	subscriptions cache.Store[azure.Subscription]

	blobAccounts   cache.Store[blob.Account]
	blobContainers cache.Store[blob.ContainerInfo]
	blobs          cache.Store[blob.BlobEntry]

	sbNamespaces cache.Store[servicebus.Namespace]
	sbEntities   cache.Store[servicebus.Entity]
	sbTopicSubs  cache.Store[servicebus.TopicSubscription]

	kvVaults   cache.Store[keyvault.Vault]
	kvSecrets  cache.Store[keyvault.Secret]
	kvVersions cache.Store[keyvault.SecretVersion]
}

func newSharedStores(db *cache.DB) sharedStores {
	if db != nil {
		return sharedStores{
			subscriptions: cache.NewStore[azure.Subscription](db, "subscriptions"),

			blobAccounts:   cache.NewStore[blob.Account](db, "blob_accounts"),
			blobContainers: cache.NewStore[blob.ContainerInfo](db, "blob_containers"),
			blobs:          cache.NewStore[blob.BlobEntry](db, "blobs"),

			sbNamespaces: cache.NewStore[servicebus.Namespace](db, "sb_namespaces"),
			sbEntities:   cache.NewStore[servicebus.Entity](db, "sb_entities"),
			sbTopicSubs:  cache.NewStore[servicebus.TopicSubscription](db, "sb_topic_subs"),

			kvVaults:   cache.NewStore[keyvault.Vault](db, "kv_vaults"),
			kvSecrets:  cache.NewStore[keyvault.Secret](db, "kv_secrets"),
			kvVersions: cache.NewStore[keyvault.SecretVersion](db, "kv_secret_versions"),
		}
	}
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
