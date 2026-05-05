package app

import (
	"strings"

	"github.com/karlssonsimon/lazyaz/internal/activity"
	"github.com/karlssonsimon/lazyaz/internal/azure"
	"github.com/karlssonsimon/lazyaz/internal/azure/blob"
	"github.com/karlssonsimon/lazyaz/internal/azure/keyvault"
	"github.com/karlssonsimon/lazyaz/internal/azure/servicebus"
	"github.com/karlssonsimon/lazyaz/internal/cache"
	"github.com/karlssonsimon/lazyaz/internal/ui"

	tea "charm.land/bubbletea/v2"
)

// TabKind identifies which type of explorer a tab contains.
type TabKind int

const (
	TabBlob TabKind = iota
	TabServiceBus
	TabKeyVault
	TabDashboard
)

func (k TabKind) String() string {
	switch k {
	case TabBlob:
		return "Blob"
	case TabServiceBus:
		return "Service Bus"
	case TabKeyVault:
		return "Key Vault"
	case TabDashboard:
		return "Dashboard"
	default:
		return "Unknown"
	}
}

// Icon returns the glyph used in the tab bar to identify a kind, drawn
// from the supplied icon set. The icon set is config-driven — see
// ui.NewIcons — so terminal-safe Unicode (default) and Nerd Fonts both
// work via the same code path.
func (k TabKind) Icon(icons ui.Icons) string {
	switch k {
	case TabBlob:
		return icons.TabBlob
	case TabServiceBus:
		return icons.TabServiceBus
	case TabKeyVault:
		return icons.TabKeyVault
	case TabDashboard:
		return icons.TabDashboard
	}
	return "·"
}

// TabKindFromString parses a config-supplied tab kind name into a
// TabKind. Recognized values (case-insensitive): "blob", "servicebus",
// "keyvault", "dashboard". Returns ok=false on anything else so the
// caller can warn.
func TabKindFromString(s string) (TabKind, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "blob":
		return TabBlob, true
	case "servicebus", "service-bus", "service_bus":
		return TabServiceBus, true
	case "keyvault", "key-vault", "key_vault":
		return TabKeyVault, true
	case "dashboard":
		return TabDashboard, true
	default:
		return 0, false
	}
}

// Tab represents a single tab in the application.
//
// Label, when non-empty, overrides the default Kind.String() name shown
// in the tab bar. Used for connection-string / Azurite tabs whose name
// (e.g. "Azurite (local)") communicates more than the bare TabKind.
type Tab struct {
	ID    int
	Kind  TabKind
	Label string
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

	kvVaults       *cache.Broker[keyvault.Vault]
	kvSecrets      *cache.Broker[keyvault.Secret]
	kvVersions     *cache.Broker[keyvault.SecretVersion]
	kvCerts        *cache.Broker[keyvault.Certificate]
	kvCertVersions *cache.Broker[keyvault.CertificateVersion]
	kvKeys         *cache.Broker[keyvault.Key]
	kvKeyVersions  *cache.Broker[keyvault.KeyVersion]
}

// bindRegistry wires r into every broker so activity tracking is
// automatic. Called once at app startup.
func (b *sharedBrokers) bindRegistry(r *activity.Registry) {
	b.subscriptions.SetRegistry(r)
	b.blobAccounts.SetRegistry(r)
	b.blobContainers.SetRegistry(r)
	b.blobs.SetRegistry(r)
	b.sbNamespaces.SetRegistry(r)
	b.sbEntities.SetRegistry(r)
	b.sbTopicSubs.SetRegistry(r)
	b.kvVaults.SetRegistry(r)
	b.kvSecrets.SetRegistry(r)
	b.kvVersions.SetRegistry(r)
	b.kvCerts.SetRegistry(r)
	b.kvCertVersions.SetRegistry(r)
	b.kvKeys.SetRegistry(r)
	b.kvKeyVersions.SetRegistry(r)
}

// resetAll cancels all active streams and clears all cached data.
// Called after az login to invalidate stale tenant-scoped data.
func (b *sharedBrokers) resetAll() {
	b.subscriptions.Reset()
	b.blobAccounts.Reset()
	b.blobContainers.Reset()
	b.blobs.Reset()
	b.sbNamespaces.Reset()
	b.sbEntities.Reset()
	b.sbTopicSubs.Reset()
	b.kvVaults.Reset()
	b.kvSecrets.Reset()
	b.kvVersions.Reset()
	b.kvCerts.Reset()
	b.kvCertVersions.Reset()
	b.kvKeys.Reset()
	b.kvKeyVersions.Reset()
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

			kvVaults:       cache.NewBroker(cache.NewStore[keyvault.Vault](db, "kv_vaults"), keyvault.VaultKey),
			kvSecrets:      cache.NewBroker(cache.NewStore[keyvault.Secret](db, "kv_secrets"), keyvault.SecretKey),
			kvVersions:     cache.NewBroker(cache.NewStore[keyvault.SecretVersion](db, "kv_secret_versions"), keyvault.VersionKey),
			kvCerts:        cache.NewBroker(cache.NewStore[keyvault.Certificate](db, "kv_certs"), keyvault.CertificateKey),
			kvCertVersions: cache.NewBroker(cache.NewStore[keyvault.CertificateVersion](db, "kv_cert_versions"), keyvault.CertificateVersionKey),
			kvKeys:         cache.NewBroker(cache.NewStore[keyvault.Key](db, "kv_keys"), keyvault.KvKeyKey),
			kvKeyVersions:  cache.NewBroker(cache.NewStore[keyvault.KeyVersion](db, "kv_key_versions"), keyvault.KeyVersionKey),
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

		kvVaults:       cache.NewBroker(cache.NewMap[keyvault.Vault](), keyvault.VaultKey),
		kvSecrets:      cache.NewBroker(cache.NewMap[keyvault.Secret](), keyvault.SecretKey),
		kvVersions:     cache.NewBroker(cache.NewMap[keyvault.SecretVersion](), keyvault.VersionKey),
		kvCerts:        cache.NewBroker(cache.NewMap[keyvault.Certificate](), keyvault.CertificateKey),
		kvCertVersions: cache.NewBroker(cache.NewMap[keyvault.CertificateVersion](), keyvault.CertificateVersionKey),
		kvKeys:         cache.NewBroker(cache.NewMap[keyvault.Key](), keyvault.KvKeyKey),
		kvKeyVersions:  cache.NewBroker(cache.NewMap[keyvault.KeyVersion](), keyvault.KeyVersionKey),
	}
}
