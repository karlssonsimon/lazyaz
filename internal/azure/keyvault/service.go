package keyvault

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/karlssonsimon/lazyaz/internal/azure"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/keyvault/armkeyvault"
	"github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azcertificates"
	"github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azkeys"
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

// Certificate is the metadata view of a vault certificate. The actual
// cert bytes (PFX/PEM) are intentionally not stored here — callers fetch
// them on demand if needed.
type Certificate struct {
	Name       string
	Enabled    bool
	Thumbprint string // hex-encoded SHA-1 from the SDK's X509Thumbprint
	CreatedOn  time.Time
	UpdatedOn  time.Time
	NotBefore  time.Time
	Expires    time.Time
}

type CertificateVersion struct {
	Version    string
	Enabled    bool
	Thumbprint string
	CreatedOn  time.Time
	UpdatedOn  time.Time
	NotBefore  time.Time
	Expires    time.Time
}

// Key holds list-level metadata for a vault key. The Azure list API
// only returns Name + Attributes + Managed flag — algorithm details
// (KeyType / KeySize / Curve) require a per-key GetKey call and aren't
// fetched up front. Private key material is never returned by the API.
type Key struct {
	Name      string
	Enabled   bool
	Managed   bool // true when the key backs a certificate and is auto-rotated
	CreatedOn time.Time
	UpdatedOn time.Time
	NotBefore time.Time
	Expires   time.Time
}

type KeyVersion struct {
	Version   string
	Enabled   bool
	CreatedOn time.Time
	UpdatedOn time.Time
	NotBefore time.Time
	Expires   time.Time
}

// Key functions for cache deduplication.
func VaultKey(v Vault) string                       { return v.Name }
func SecretKey(s Secret) string                     { return s.Name }
func VersionKey(v SecretVersion) string             { return v.Version }
func CertificateKey(c Certificate) string           { return c.Name }
func CertificateVersionKey(v CertificateVersion) string { return v.Version }
func KvKeyKey(k Key) string                         { return k.Name }
func KeyVersionKey(v KeyVersion) string             { return v.Version }

type Service struct {
	cred azcore.TokenCredential
	mu   sync.Mutex
	// Three independent client caches, one per Azure SDK package. They
	// share the same vaultURI keyspace but the SDK clients themselves
	// aren't interchangeable.
	secretsClients map[string]*azsecrets.Client
	certsClients   map[string]*azcertificates.Client
	keysClients    map[string]*azkeys.Client
}

func NewService(cred azcore.TokenCredential) *Service {
	return &Service{
		cred:           cred,
		secretsClients: make(map[string]*azsecrets.Client),
		certsClients:   make(map[string]*azcertificates.Client),
		keysClients:    make(map[string]*azkeys.Client),
	}
}

func (s *Service) Credential() azcore.TokenCredential {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cred
}

// SetCredential swaps the credential and clears all cached clients so
// they are re-created with the new identity on next use.
func (s *Service) SetCredential(cred azcore.TokenCredential) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cred = cred
	s.secretsClients = make(map[string]*azsecrets.Client)
	s.certsClients = make(map[string]*azcertificates.Client)
	s.keysClients = make(map[string]*azkeys.Client)
}

func (s *Service) getClient(vaultURI string) (*azsecrets.Client, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if c, ok := s.secretsClients[vaultURI]; ok {
		return c, nil
	}

	c, err := azsecrets.NewClient(vaultURI, s.cred, nil)
	if err != nil {
		return nil, fmt.Errorf("create secrets client for %s: %w", vaultURI, err)
	}

	s.secretsClients[vaultURI] = c
	return c, nil
}

func (s *Service) getCertsClient(vaultURI string) (*azcertificates.Client, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if c, ok := s.certsClients[vaultURI]; ok {
		return c, nil
	}
	c, err := azcertificates.NewClient(vaultURI, s.cred, nil)
	if err != nil {
		return nil, fmt.Errorf("create certificates client for %s: %w", vaultURI, err)
	}
	s.certsClients[vaultURI] = c
	return c, nil
}

func (s *Service) getKeysClient(vaultURI string) (*azkeys.Client, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if c, ok := s.keysClients[vaultURI]; ok {
		return c, nil
	}
	c, err := azkeys.NewClient(vaultURI, s.cred, nil)
	if err != nil {
		return nil, fmt.Errorf("create keys client for %s: %w", vaultURI, err)
	}
	s.keysClients[vaultURI] = c
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

// SetSecret creates a new secret (or a new version of an existing one)
// with the given value. Azure Key Vault treats create-vs-update as the
// same operation — SetSecret returns a 200 in both cases, producing a
// new version row either way. The caller is responsible for any
// confirm-before-overwrite UX.
func (s *Service) SetSecret(ctx context.Context, vault Vault, name, value string) error {
	client, err := s.getClient(vault.VaultURI)
	if err != nil {
		return err
	}
	if _, err := client.SetSecret(ctx, name, azsecrets.SetSecretParameters{Value: &value}, nil); err != nil {
		return fmt.Errorf("set secret %s in %s: %w", name, vault.Name, err)
	}
	return nil
}

// DeleteSecret removes a secret. With soft-delete enabled (the vault
// default), this moves the secret to the recovery bin where it can be
// purged or recovered for the configured retention period.
func (s *Service) DeleteSecret(ctx context.Context, vault Vault, name string) error {
	client, err := s.getClient(vault.VaultURI)
	if err != nil {
		return err
	}
	if _, err := client.DeleteSecret(ctx, name, nil); err != nil {
		return fmt.Errorf("delete secret %s in %s: %w", name, vault.Name, err)
	}
	return nil
}

// DeleteCertificate removes a certificate (all versions). Soft-delete
// applies the same way as for secrets.
func (s *Service) DeleteCertificate(ctx context.Context, vault Vault, name string) error {
	client, err := s.getCertsClient(vault.VaultURI)
	if err != nil {
		return err
	}
	if _, err := client.DeleteCertificate(ctx, name, nil); err != nil {
		return fmt.Errorf("delete certificate %s in %s: %w", name, vault.Name, err)
	}
	return nil
}

// DeleteKey removes a key (all versions). Soft-delete applies. Note
// that a "managed" key (one auto-created to back a certificate) can't
// be deleted directly — you delete the certificate instead.
func (s *Service) DeleteKey(ctx context.Context, vault Vault, name string) error {
	client, err := s.getKeysClient(vault.VaultURI)
	if err != nil {
		return err
	}
	if _, err := client.DeleteKey(ctx, name, nil); err != nil {
		return fmt.Errorf("delete key %s in %s: %w", name, vault.Name, err)
	}
	return nil
}

// CreateKey generates a new key in the vault. kty is "RSA" or "EC".
// For RSA, keySize is the modulus length in bits (2048/3072/4096) and
// curve is ignored. For EC, curve is the named curve (P-256/P-384/P-521)
// and keySize is ignored. Mirrors the create-vs-update semantics of
// SetSecret: re-using a name produces a new version under the same name.
func (s *Service) CreateKey(ctx context.Context, vault Vault, name, kty string, keySize int32, curve string) error {
	client, err := s.getKeysClient(vault.VaultURI)
	if err != nil {
		return err
	}
	keyType := azkeys.KeyType(kty)
	params := azkeys.CreateKeyParameters{Kty: &keyType}
	switch kty {
	case "RSA":
		params.KeySize = &keySize
	case "EC":
		c := azkeys.CurveName(curve)
		params.Curve = &c
	}
	if _, err := client.CreateKey(ctx, name, params, nil); err != nil {
		return fmt.Errorf("create key %s in %s: %w", name, vault.Name, err)
	}
	return nil
}

// ImportCertificate imports a PFX-encoded certificate (with private key)
// into the vault. pfxBytes is raw PFX content; password is the PFX
// password (empty when unprotected). Producing a base64 encoding here
// keeps the call site simple — the SDK's ImportCertificate is base64-only.
func (s *Service) ImportCertificate(ctx context.Context, vault Vault, name string, pfxBytes []byte, password string) error {
	if len(pfxBytes) == 0 {
		return fmt.Errorf("PFX content is empty")
	}
	client, err := s.getCertsClient(vault.VaultURI)
	if err != nil {
		return err
	}
	encoded := base64.StdEncoding.EncodeToString(pfxBytes)
	params := azcertificates.ImportCertificateParameters{Base64EncodedCertificate: &encoded}
	if password != "" {
		params.Password = &password
	}
	if _, err := client.ImportCertificate(ctx, name, params, nil); err != nil {
		return fmt.Errorf("import certificate %s in %s: %w", name, vault.Name, err)
	}
	return nil
}

// ListCertificates streams certificate metadata for the vault. Mirrors
// ListSecrets in shape so the cache broker plumbing works the same way.
func (s *Service) ListCertificates(ctx context.Context, vault Vault, send func([]Certificate)) error {
	client, err := s.getCertsClient(vault.VaultURI)
	if err != nil {
		return err
	}
	pager := client.NewListCertificatePropertiesPager(nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("list certificates in %s: %w", vault.Name, err)
		}
		var batch []Certificate
		for _, cp := range page.Value {
			if cp == nil || cp.ID == nil {
				continue
			}
			entry := Certificate{
				Name:       cp.ID.Name(),
				Thumbprint: hex.EncodeToString(cp.X509Thumbprint),
			}
			if a := cp.Attributes; a != nil {
				if a.Enabled != nil {
					entry.Enabled = *a.Enabled
				}
				if a.Created != nil {
					entry.CreatedOn = *a.Created
				}
				if a.Updated != nil {
					entry.UpdatedOn = *a.Updated
				}
				if a.NotBefore != nil {
					entry.NotBefore = *a.NotBefore
				}
				if a.Expires != nil {
					entry.Expires = *a.Expires
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

func (s *Service) ListCertificateVersions(ctx context.Context, vault Vault, certName string, send func([]CertificateVersion)) error {
	client, err := s.getCertsClient(vault.VaultURI)
	if err != nil {
		return err
	}
	pager := client.NewListCertificatePropertiesVersionsPager(certName, nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("list certificate versions for %s in %s: %w", certName, vault.Name, err)
		}
		var batch []CertificateVersion
		for _, cp := range page.Value {
			if cp == nil || cp.ID == nil {
				continue
			}
			entry := CertificateVersion{
				Version:    cp.ID.Version(),
				Thumbprint: hex.EncodeToString(cp.X509Thumbprint),
			}
			if a := cp.Attributes; a != nil {
				if a.Enabled != nil {
					entry.Enabled = *a.Enabled
				}
				if a.Created != nil {
					entry.CreatedOn = *a.Created
				}
				if a.Updated != nil {
					entry.UpdatedOn = *a.Updated
				}
				if a.NotBefore != nil {
					entry.NotBefore = *a.NotBefore
				}
				if a.Expires != nil {
					entry.Expires = *a.Expires
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

func (s *Service) ListKeys(ctx context.Context, vault Vault, send func([]Key)) error {
	client, err := s.getKeysClient(vault.VaultURI)
	if err != nil {
		return err
	}
	pager := client.NewListKeyPropertiesPager(nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("list keys in %s: %w", vault.Name, err)
		}
		var batch []Key
		for _, kp := range page.Value {
			if kp == nil || kp.KID == nil {
				continue
			}
			entry := Key{Name: kp.KID.Name()}
			if kp.Managed != nil {
				entry.Managed = *kp.Managed
			}
			if a := kp.Attributes; a != nil {
				if a.Enabled != nil {
					entry.Enabled = *a.Enabled
				}
				if a.Created != nil {
					entry.CreatedOn = *a.Created
				}
				if a.Updated != nil {
					entry.UpdatedOn = *a.Updated
				}
				if a.NotBefore != nil {
					entry.NotBefore = *a.NotBefore
				}
				if a.Expires != nil {
					entry.Expires = *a.Expires
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

func (s *Service) ListKeyVersions(ctx context.Context, vault Vault, keyName string, send func([]KeyVersion)) error {
	client, err := s.getKeysClient(vault.VaultURI)
	if err != nil {
		return err
	}
	pager := client.NewListKeyPropertiesVersionsPager(keyName, nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("list key versions for %s in %s: %w", keyName, vault.Name, err)
		}
		var batch []KeyVersion
		for _, kp := range page.Value {
			if kp == nil || kp.KID == nil {
				continue
			}
			entry := KeyVersion{Version: kp.KID.Version()}
			if a := kp.Attributes; a != nil {
				if a.Enabled != nil {
					entry.Enabled = *a.Enabled
				}
				if a.Created != nil {
					entry.CreatedOn = *a.Created
				}
				if a.Updated != nil {
					entry.UpdatedOn = *a.Updated
				}
				if a.NotBefore != nil {
					entry.NotBefore = *a.NotBefore
				}
				if a.Expires != nil {
					entry.Expires = *a.Expires
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
