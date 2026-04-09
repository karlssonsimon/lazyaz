package app

import (
	"strings"

	"github.com/karlssonsimon/lazyaz/internal/azure"
	"github.com/karlssonsimon/lazyaz/internal/azure/blob"
	"github.com/karlssonsimon/lazyaz/internal/azure/keyvault"
	"github.com/karlssonsimon/lazyaz/internal/azure/servicebus"
	"github.com/karlssonsimon/lazyaz/internal/cache"

	tea "charm.land/bubbletea/v2"
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

// sharedBrokers holds all shared brokers owned by the parent.
// Each broker manages both the cache store and the in-flight fetch
// coordination, so tabs that request the same data share a single
// API call.
type sharedBrokers struct {
	subscriptions *cache.Broker[azure.Subscription]

	blobAccounts   *cache.Broker[blob.Account]
	blobContainers *cache.Broker[blob.ContainerInfo]
	blobs          *cache.Broker[blob.BlobEntry]

	sbNamespaces *cache.Broker[servicebus.Namespace]
	sbEntities   *cache.Broker[servicebus.Entity]
	sbTopicSubs  *cache.Broker[servicebus.TopicSubscription]

	kvVaults   *cache.Broker[keyvault.Vault]
	kvSecrets  *cache.Broker[keyvault.Secret]
	kvVersions *cache.Broker[keyvault.SecretVersion]
}

func newSharedBrokers(db *cache.DB) sharedBrokers {
	if db != nil {
		return sharedBrokers{
			subscriptions: cache.NewBroker(cache.NewStore[azure.Subscription](db, "subscriptions"), azure.SubscriptionKey),

			blobAccounts:   cache.NewBroker(cache.NewStore[blob.Account](db, "blob_accounts"), blob.AccountKey),
			blobContainers: cache.NewBroker(cache.NewStore[blob.ContainerInfo](db, "blob_containers"), blob.ContainerKey),
			blobs:          cache.NewBroker(cache.NewStore[blob.BlobEntry](db, "blobs"), blob.BlobEntryKey),

			sbNamespaces: cache.NewBroker(cache.NewStore[servicebus.Namespace](db, "sb_namespaces"), servicebus.NamespaceKey),
			sbEntities:   cache.NewBroker(cache.NewStore[servicebus.Entity](db, "sb_entities"), servicebus.EntityKey),
			sbTopicSubs:  cache.NewBroker(cache.NewStore[servicebus.TopicSubscription](db, "sb_topic_subs"), servicebus.TopicSubscriptionKey),

			kvVaults:   cache.NewBroker(cache.NewStore[keyvault.Vault](db, "kv_vaults"), keyvault.VaultKey),
			kvSecrets:  cache.NewBroker(cache.NewStore[keyvault.Secret](db, "kv_secrets"), keyvault.SecretKey),
			kvVersions: cache.NewBroker(cache.NewStore[keyvault.SecretVersion](db, "kv_secret_versions"), keyvault.VersionKey),
		}
	}
	return sharedBrokers{
		subscriptions: cache.NewBroker(cache.NewMap[azure.Subscription](), azure.SubscriptionKey),

		blobAccounts:   cache.NewBroker(cache.NewMap[blob.Account](), blob.AccountKey),
		blobContainers: cache.NewBroker(cache.NewMap[blob.ContainerInfo](), blob.ContainerKey),
		blobs:          cache.NewBroker(cache.NewMap[blob.BlobEntry](), blob.BlobEntryKey),

		sbNamespaces: cache.NewBroker(cache.NewMap[servicebus.Namespace](), servicebus.NamespaceKey),
		sbEntities:   cache.NewBroker(cache.NewMap[servicebus.Entity](), servicebus.EntityKey),
		sbTopicSubs:  cache.NewBroker(cache.NewMap[servicebus.TopicSubscription](), servicebus.TopicSubscriptionKey),

		kvVaults:   cache.NewBroker(cache.NewMap[keyvault.Vault](), keyvault.VaultKey),
		kvSecrets:  cache.NewBroker(cache.NewMap[keyvault.Secret](), keyvault.SecretKey),
		kvVersions: cache.NewBroker(cache.NewMap[keyvault.SecretVersion](), keyvault.VersionKey),
	}
}
