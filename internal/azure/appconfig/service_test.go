package appconfig

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/data/azappconfig"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/appconfiguration/armappconfiguration"
)

func strPtr(s string) *string { return &s }

func TestSettingKeyDistinguishesLabels(t *testing.T) {
	a := Setting{Key: "db/host", Label: ""}
	b := Setting{Key: "db/host", Label: "prod"}
	if SettingKey(a) == SettingKey(b) {
		t.Fatal("same key under different labels must not collide in the cache")
	}
}

func TestKeyVaultReferenceParses(t *testing.T) {
	s := Setting{
		ContentType: "application/vnd.microsoft.appconfig.keyvaultref+json;charset=utf-8",
		Value:       `{"uri":"https://kv-htg-prod.vault.azure.net/secrets/DbPassword/abc123"}`,
	}
	ref, ok := s.KeyVaultReference()
	if !ok {
		t.Fatal("reference not recognised")
	}
	if ref.VaultName != "kv-htg-prod" || ref.SecretName != "DbPassword" || ref.Version != "abc123" ||
		ref.VaultURI != "https://kv-htg-prod.vault.azure.net" {
		t.Fatalf("parsed = %+v", ref)
	}

	s.Value = `{"uri":"https://kv-htg-prod.vault.azure.net/secrets/DbPassword"}`
	if ref, ok := s.KeyVaultReference(); !ok || ref.Version != "" {
		t.Fatalf("latest reference: ok=%v ref=%+v", ok, ref)
	}

	plain := Setting{ContentType: "text/plain", Value: s.Value}
	if _, ok := plain.KeyVaultReference(); ok {
		t.Fatal("plain content type must not parse as a reference")
	}
	broken := Setting{ContentType: s.ContentType, Value: `{"uri":"not a url"}`}
	if _, ok := broken.KeyVaultReference(); ok {
		t.Fatal("malformed reference must not parse")
	}
}

func TestFeatureFlagDetection(t *testing.T) {
	byType := Setting{Key: "x", ContentType: "application/vnd.microsoft.appconfig.ff+json;charset=utf-8"}
	byKey := Setting{Key: ".appconfig.featureflag/beta"}
	if !byType.IsFeatureFlag() || !byKey.IsFeatureFlag() {
		t.Fatal("feature flag not detected")
	}
	if byKey.DisplayKey() != "beta" {
		t.Fatalf("DisplayKey = %q", byKey.DisplayKey())
	}
	if (Setting{Key: "plain"}).IsFeatureFlag() {
		t.Fatal("plain key flagged")
	}
}

func TestFromSDKDereferences(t *testing.T) {
	ts := time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)
	ro := true
	etag := azcore.ETag(`"abc"`)
	got := fromSDK(azappconfig.Setting{
		Key: strPtr("k"), Label: strPtr("l"), Value: strPtr("v"), ContentType: strPtr("text/plain"),
		LastModified: &ts, IsReadOnly: &ro, ETag: &etag, Tags: map[string]string{"env": "prod"},
	})
	if got.Key != "k" || got.Label != "l" || got.Value != "v" || got.ContentType != "text/plain" ||
		!got.LastModified.Equal(ts) || !got.ReadOnly || got.ETag != `"abc"` || got.Tags["env"] != "prod" {
		t.Fatalf("fromSDK = %+v", got)
	}
	if zero := fromSDK(azappconfig.Setting{}); zero.Key != "" || zero.ReadOnly || !zero.LastModified.IsZero() {
		t.Fatalf("nil pointers should map to zero values: %+v", zero)
	}
}

func TestParseResourceGroup(t *testing.T) {
	id := "/subscriptions/s/resourceGroups/rg-config/providers/Microsoft.AppConfiguration/configurationStores/st"
	if got := parseResourceGroup(&id); got != "rg-config" {
		t.Fatalf("parseResourceGroup = %q", got)
	}
	if parseResourceGroup(nil) != "" {
		t.Fatal("nil id should give empty")
	}
}

func TestIsAuthError(t *testing.T) {
	if !isAuthError(&azcore.ResponseError{StatusCode: 403}) || !isAuthError(&azcore.ResponseError{StatusCode: 401}) {
		t.Fatal("401/403 should count as auth errors")
	}
	if isAuthError(&azcore.ResponseError{StatusCode: 404}) || isAuthError(errors.New("boom")) {
		t.Fatal("other errors must not trigger the fallback")
	}
	wrapped := fmt.Errorf("list settings in x: %w", &azcore.ResponseError{StatusCode: 403})
	if !isAuthError(wrapped) {
		t.Fatal("wrapped auth error not detected")
	}
}

func TestPickAccessKeyPrefersReadOnly(t *testing.T) {
	rw, ro := "Endpoint=rw", "Endpoint=ro"
	yes, no := true, false
	keys := []*armappconfiguration.APIKey{
		{ConnectionString: &rw, ReadOnly: &no},
		nil,
		{ConnectionString: &ro, ReadOnly: &yes},
	}
	if got := pickAccessKey(keys); got != ro {
		t.Fatalf("pickAccessKey = %q, want the read-only key", got)
	}
	if got := pickAccessKey(keys[:1]); got != rw {
		t.Fatalf("pickAccessKey without a read-only key = %q, want the read-write one", got)
	}
	if got := pickAccessKey(nil); got != "" {
		t.Fatalf("pickAccessKey(nil) = %q", got)
	}
}
