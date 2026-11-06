// Package appconfig is the Azure App Configuration client used by the
// App Configuration explorer: stores via ARM, key-values via the data
// plane. Read-only for now.
package appconfig

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/karlssonsimon/lazyaz/internal/azure"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/data/azappconfig"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/appconfiguration/armappconfiguration"
)

// Store is an App Configuration store (an ARM resource) and the
// endpoint its key-values are read from.
type Store struct {
	Name           string
	SubscriptionID string
	ResourceGroup  string
	Endpoint       string
	Location       string
}

// Setting is one key-value. Key and Label together identify it; the
// empty label is a real, distinct label the portal shows as "(No label)".
type Setting struct {
	Key          string
	Label        string
	Value        string
	ContentType  string
	LastModified time.Time
	ReadOnly     bool
	Tags         map[string]string
	ETag         string
}

// Key functions for cache deduplication.
func StoreKey(s Store) string { return s.Name }

// SettingKey identifies a setting by key and label; the separator
// cannot appear in either.
func SettingKey(s Setting) string { return s.Key + "\x00" + s.Label }

const (
	// Content types App Configuration uses for its two special kinds
	// of value. Compared by prefix, since the charset suffix varies.
	contentTypeKeyVaultRef = "application/vnd.microsoft.appconfig.keyvaultref+json"
	contentTypeFeatureFlag = "application/vnd.microsoft.appconfig.ff+json"

	// FeatureFlagPrefix is the key prefix of every feature flag.
	FeatureFlagPrefix = ".appconfig.featureflag/"
)

// IsKeyVaultReference reports whether the value points at a Key Vault
// secret rather than holding the value itself.
func (s Setting) IsKeyVaultReference() bool {
	return strings.HasPrefix(strings.ToLower(s.ContentType), contentTypeKeyVaultRef)
}

// IsFeatureFlag reports whether the setting is a feature flag.
func (s Setting) IsFeatureFlag() bool {
	return strings.HasPrefix(strings.ToLower(s.ContentType), contentTypeFeatureFlag) ||
		strings.HasPrefix(s.Key, FeatureFlagPrefix)
}

// DisplayKey is the key without the feature-flag prefix, for lists.
func (s Setting) DisplayKey() string {
	return strings.TrimPrefix(s.Key, FeatureFlagPrefix)
}

// KeyVaultReference is the parsed target of a Key Vault reference.
type KeyVaultReference struct {
	VaultURI   string // https://<vault>.vault.azure.net
	VaultName  string
	SecretName string
	Version    string // empty for "latest"
}

// KeyVaultReference parses the reference value, which is JSON of the
// form {"uri":"https://<vault>.vault.azure.net/secrets/<name>[/<version>]"}.
func (s Setting) KeyVaultReference() (KeyVaultReference, bool) {
	if !s.IsKeyVaultReference() {
		return KeyVaultReference{}, false
	}
	var body struct {
		URI string `json:"uri"`
	}
	if err := json.Unmarshal([]byte(s.Value), &body); err != nil || body.URI == "" {
		return KeyVaultReference{}, false
	}
	u, err := url.Parse(body.URI)
	if err != nil || u.Host == "" {
		return KeyVaultReference{}, false
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 2 || !strings.EqualFold(parts[0], "secrets") || parts[1] == "" {
		return KeyVaultReference{}, false
	}
	ref := KeyVaultReference{
		VaultURI:   u.Scheme + "://" + u.Host,
		VaultName:  strings.SplitN(u.Host, ".", 2)[0],
		SecretName: parts[1],
	}
	if len(parts) > 2 {
		ref.Version = parts[2]
	}
	return ref, true
}

// Service wraps the SDK clients. Data-plane clients are cached per
// store endpoint and rebuilt when the credential changes. Two caches:
// AAD clients, and access-key clients used as a fallback when the
// caller can list the store through ARM but holds no data-plane role.
type Service struct {
	cred           azcore.TokenCredential
	mu             sync.Mutex
	clients        map[string]*azappconfig.Client
	connStrClients map[string]*azappconfig.Client
}

func NewService(cred azcore.TokenCredential) *Service {
	return &Service{
		cred:           cred,
		clients:        make(map[string]*azappconfig.Client),
		connStrClients: make(map[string]*azappconfig.Client),
	}
}

func (s *Service) Credential() azcore.TokenCredential {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cred
}

// SetCredential swaps the credential and drops the cached clients so
// they are re-created with the new identity on next use.
func (s *Service) SetCredential(cred azcore.TokenCredential) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cred = cred
	s.clients = make(map[string]*azappconfig.Client)
	s.connStrClients = make(map[string]*azappconfig.Client)
}

func (s *Service) getClient(endpoint string) (*azappconfig.Client, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if c, ok := s.clients[endpoint]; ok {
		return c, nil
	}
	c, err := azappconfig.NewClient(endpoint, s.cred, nil)
	if err != nil {
		return nil, fmt.Errorf("create app configuration client for %s: %w", endpoint, err)
	}
	s.clients[endpoint] = c
	return c, nil
}

// getConnStrClient builds a client from the store's access keys, read
// through ARM. Owner and Contributor can list keys even without the
// App Configuration Data Reader role, which is the common gap. A
// read-only key is preferred; stores with local authentication
// disabled have no usable keys and the fallback reports that.
func (s *Service) getConnStrClient(ctx context.Context, store Store) (*azappconfig.Client, error) {
	s.mu.Lock()
	if c, ok := s.connStrClients[store.Endpoint]; ok {
		s.mu.Unlock()
		return c, nil
	}
	cred := s.cred
	s.mu.Unlock()

	if store.SubscriptionID == "" || store.ResourceGroup == "" || store.Name == "" {
		return nil, fmt.Errorf("insufficient store metadata for access key fallback")
	}
	arm, err := armappconfiguration.NewConfigurationStoresClient(store.SubscriptionID, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("create configuration stores client for keys: %w", err)
	}
	var keys []*armappconfiguration.APIKey
	pager := arm.NewListKeysPager(store.ResourceGroup, store.Name, nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list access keys for %s: %w", store.Name, err)
		}
		keys = append(keys, page.Value...)
	}
	connStr := pickAccessKey(keys)
	if connStr == "" {
		return nil, fmt.Errorf("no access key available for %s (local authentication may be disabled)", store.Name)
	}
	c, err := azappconfig.NewClientFromConnectionString(connStr, nil)
	if err != nil {
		return nil, fmt.Errorf("create access key client for %s: %w", store.Name, err)
	}
	s.mu.Lock()
	if existing, ok := s.connStrClients[store.Endpoint]; ok {
		s.mu.Unlock()
		return existing, nil
	}
	s.connStrClients[store.Endpoint] = c
	s.mu.Unlock()
	return c, nil
}

// pickAccessKey returns a read-only key's connection string when one
// exists, otherwise any key's.
func pickAccessKey(keys []*armappconfiguration.APIKey) string {
	fallback := ""
	for _, k := range keys {
		if k == nil || k.ConnectionString == nil || *k.ConnectionString == "" {
			continue
		}
		if k.ReadOnly != nil && *k.ReadOnly {
			return *k.ConnectionString
		}
		if fallback == "" {
			fallback = *k.ConnectionString
		}
	}
	return fallback
}

// withFallback runs fn with an AAD client and, on an authorization
// failure, once more with an access-key client. Same shape as the
// Service Bus connection-string fallback.
func (s *Service) withFallback(ctx context.Context, store Store, label string, fn func(*azappconfig.Client) error) error {
	aad, err := s.getClient(store.Endpoint)
	if err != nil {
		return err
	}
	err = fn(aad)
	if err == nil || !isAuthError(err) {
		return err
	}
	fb, fbErr := s.getConnStrClient(ctx, store)
	if fbErr != nil {
		return fmt.Errorf("%s with AAD failed: %v; access key fallback failed: %w", label, err, fbErr)
	}
	if err := fn(fb); err != nil {
		return fmt.Errorf("%s with access key fallback: %w", label, err)
	}
	return nil
}

// isAuthError reports a 401 or 403 from the data plane: the identity is
// valid but holds no data-plane role on the store.
func isAuthError(err error) bool {
	var re *azcore.ResponseError
	if errors.As(err, &re) {
		return re.StatusCode == http.StatusUnauthorized || re.StatusCode == http.StatusForbidden
	}
	return false
}

func (s *Service) ListSubscriptions(ctx context.Context, send func([]azure.Subscription)) error {
	return azure.ListSubscriptions(ctx, s.Credential(), send)
}

// ListStores lists the subscription's stores, sorted by name.
func (s *Service) ListStores(ctx context.Context, subscriptionID string, send func([]Store)) error {
	client, err := armappconfiguration.NewConfigurationStoresClient(subscriptionID, s.Credential(), nil)
	if err != nil {
		return fmt.Errorf("create configuration stores client: %w", err)
	}

	pager := client.NewListPager(nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("list app configuration stores: %w", err)
		}

		var batch []Store
		for _, st := range page.Value {
			if st == nil || st.Name == nil {
				continue
			}
			entry := Store{
				Name:           *st.Name,
				SubscriptionID: subscriptionID,
				ResourceGroup:  parseResourceGroup(st.ID),
			}
			if st.Location != nil {
				entry.Location = *st.Location
			}
			if st.Properties != nil && st.Properties.Endpoint != nil {
				entry.Endpoint = strings.TrimSuffix(*st.Properties.Endpoint, "/")
			}
			if entry.Endpoint == "" {
				entry.Endpoint = fmt.Sprintf("https://%s.azconfig.io", *st.Name)
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

// ListSettings lists key-values matching the filters. An empty key
// filter means every key; an empty label filter means every label.
// Filters take the service's wildcard syntax ("app/*", "prod,staging").
// Values are included, so a preview needs no second call.
func (s *Service) ListSettings(ctx context.Context, store Store, keyFilter, labelFilter string, send func([]Setting)) error {
	selector := azappconfig.SettingSelector{}
	if keyFilter != "" {
		selector.KeyFilter = &keyFilter
	}
	if labelFilter != "" {
		selector.LabelFilter = &labelFilter
	}
	return s.withFallback(ctx, store, "list settings in "+store.Name, func(client *azappconfig.Client) error {
		pager := client.NewListSettingsPager(selector, nil)
		for pager.More() {
			page, err := pager.NextPage(ctx)
			if err != nil {
				return fmt.Errorf("list settings in %s: %w", store.Name, err)
			}
			batch := make([]Setting, 0, len(page.Settings))
			for _, st := range page.Settings {
				batch = append(batch, fromSDK(st))
			}
			if len(batch) > 0 {
				send(batch)
			}
		}
		return nil
	})
}

// GetSetting fetches one key-value. label "" selects the unlabelled
// value, as the service defines it.
func (s *Service) GetSetting(ctx context.Context, store Store, key, label string) (Setting, error) {
	var out Setting
	err := s.withFallback(ctx, store, "get setting "+key, func(client *azappconfig.Client) error {
		resp, err := client.GetSetting(ctx, key, &azappconfig.GetSettingOptions{Label: &label})
		if err != nil {
			return fmt.Errorf("get setting %s: %w", key, err)
		}
		out = fromSDK(resp.Setting)
		return nil
	})
	return out, err
}

// ListRevisions lists the history of one key-value, newest first as the
// service returns it.
func (s *Service) ListRevisions(ctx context.Context, store Store, key, label string, send func([]Setting)) error {
	selector := azappconfig.SettingSelector{KeyFilter: &key}
	if label != "" {
		selector.LabelFilter = &label
	} else {
		noLabel := "\x00"
		selector.LabelFilter = &noLabel
	}
	return s.withFallback(ctx, store, "list revisions of "+key, func(client *azappconfig.Client) error {
		pager := client.NewListRevisionsPager(selector, nil)
		for pager.More() {
			page, err := pager.NextPage(ctx)
			if err != nil {
				return fmt.Errorf("list revisions of %s: %w", key, err)
			}
			batch := make([]Setting, 0, len(page.Settings))
			for _, st := range page.Settings {
				batch = append(batch, fromSDK(st))
			}
			if len(batch) > 0 {
				send(batch)
			}
		}
		return nil
	})
}

func fromSDK(st azappconfig.Setting) Setting {
	out := Setting{Tags: st.Tags}
	deref := func(dst *string, src *string) {
		if src != nil {
			*dst = *src
		}
	}
	deref(&out.Key, st.Key)
	deref(&out.Label, st.Label)
	deref(&out.Value, st.Value)
	deref(&out.ContentType, st.ContentType)
	if st.LastModified != nil {
		out.LastModified = *st.LastModified
	}
	if st.IsReadOnly != nil {
		out.ReadOnly = *st.IsReadOnly
	}
	if st.ETag != nil {
		out.ETag = string(*st.ETag)
	}
	return out
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
