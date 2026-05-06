package kvapp

import (
	"github.com/karlssonsimon/lazyaz/internal/azure"
	"github.com/karlssonsimon/lazyaz/internal/azure/keyvault"
	"github.com/karlssonsimon/lazyaz/internal/cache"
)

// kvCache provides a stale-while-revalidate cache for:
//
//	subscriptions → vaults → {secrets, certs, keys} → versions
type kvCache struct {
	subscriptions *cache.Broker[azure.Subscription]
	vaults        *cache.Broker[keyvault.Vault]
	secrets       *cache.Broker[keyvault.Secret]
	versions      *cache.Broker[keyvault.SecretVersion]
	certs         *cache.Broker[keyvault.Certificate]
	certVersions  *cache.Broker[keyvault.CertificateVersion]
	keys          *cache.Broker[keyvault.Key]
	keyVersions   *cache.Broker[keyvault.KeyVersion]
}

func newCache(db *cache.DB) kvCache {
	if db != nil {
		return kvCache{
			subscriptions: cache.NewBroker(cache.NewStore[azure.Subscription](db, "subscriptions"), azure.SubscriptionKey),
			vaults:        cache.NewBroker(cache.NewStore[keyvault.Vault](db, "kv_vaults"), vaultKey),
			secrets:       cache.NewBroker(cache.NewStore[keyvault.Secret](db, "kv_secrets"), secretKey),
			versions:      cache.NewBroker(cache.NewStore[keyvault.SecretVersion](db, "kv_secret_versions"), versionKey),
			certs:         cache.NewBroker(cache.NewStore[keyvault.Certificate](db, "kv_certs"), certKey),
			certVersions:  cache.NewBroker(cache.NewStore[keyvault.CertificateVersion](db, "kv_cert_versions"), certVersionKey),
			keys:          cache.NewBroker(cache.NewStore[keyvault.Key](db, "kv_keys"), kvKeyKey),
			keyVersions:   cache.NewBroker(cache.NewStore[keyvault.KeyVersion](db, "kv_key_versions"), keyVersionKey),
		}
	}
	return kvCache{
		subscriptions: cache.NewBroker(cache.NewMap[azure.Subscription](), azure.SubscriptionKey),
		vaults:        cache.NewBroker(cache.NewMap[keyvault.Vault](), vaultKey),
		secrets:       cache.NewBroker(cache.NewMap[keyvault.Secret](), secretKey),
		versions:      cache.NewBroker(cache.NewMap[keyvault.SecretVersion](), versionKey),
		certs:         cache.NewBroker(cache.NewMap[keyvault.Certificate](), certKey),
		certVersions:  cache.NewBroker(cache.NewMap[keyvault.CertificateVersion](), certVersionKey),
		keys:          cache.NewBroker(cache.NewMap[keyvault.Key](), kvKeyKey),
		keyVersions:   cache.NewBroker(cache.NewMap[keyvault.KeyVersion](), keyVersionKey),
	}
}

// KVStores holds the shared brokers for key vault resources.
type KVStores struct {
	Subscriptions *cache.Broker[azure.Subscription]
	Vaults        *cache.Broker[keyvault.Vault]
	Secrets       *cache.Broker[keyvault.Secret]
	Versions      *cache.Broker[keyvault.SecretVersion]
	Certs         *cache.Broker[keyvault.Certificate]
	CertVersions  *cache.Broker[keyvault.CertificateVersion]
	Keys          *cache.Broker[keyvault.Key]
	KeyVersions   *cache.Broker[keyvault.KeyVersion]
}

// NewCacheWithStores creates a kvCache using pre-built shared brokers.
// Any nil broker falls back to a private in-memory map so the parent
// can opt into sharing per-broker.
func NewCacheWithStores(s KVStores) kvCache {
	c := kvCache{
		subscriptions: s.Subscriptions,
		vaults:        s.Vaults,
		secrets:       s.Secrets,
		versions:      s.Versions,
		certs:         s.Certs,
		certVersions:  s.CertVersions,
		keys:          s.Keys,
		keyVersions:   s.KeyVersions,
	}
	if c.certs == nil {
		c.certs = cache.NewBroker(cache.NewMap[keyvault.Certificate](), certKey)
	}
	if c.certVersions == nil {
		c.certVersions = cache.NewBroker(cache.NewMap[keyvault.CertificateVersion](), certVersionKey)
	}
	if c.keys == nil {
		c.keys = cache.NewBroker(cache.NewMap[keyvault.Key](), kvKeyKey)
	}
	if c.keyVersions == nil {
		c.keyVersions = cache.NewBroker(cache.NewMap[keyvault.KeyVersion](), keyVersionKey)
	}
	return c
}
