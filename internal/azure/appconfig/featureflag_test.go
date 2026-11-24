package appconfig

import (
	"encoding/json"
	"testing"
)

func flagSetting(value string) Setting {
	return Setting{Key: FeatureFlagKey("beta"), ContentType: FeatureFlagContentType, Value: value}
}

func TestParseFeatureFlag(t *testing.T) {
	ff, ok := ParseFeatureFlag(flagSetting(`{"id":"beta","description":"Beta UI","enabled":true,"conditions":{"client_filters":[{"name":"Percentage"}]}}`))
	if !ok || ff.ID != "beta" || ff.Description != "Beta UI" || !ff.Enabled || !ff.HasFilters {
		t.Fatalf("parsed = %+v ok=%v", ff, ok)
	}
	if _, ok := ParseFeatureFlag(Setting{Key: "plain", Value: `{"enabled":true}`}); ok {
		t.Fatal("a plain key must not parse as a flag")
	}
	if _, ok := ParseFeatureFlag(flagSetting("{broken")); ok {
		t.Fatal("broken JSON must not parse")
	}
}

// Toggling flips only "enabled"; filters and unknown fields survive.
func TestToggleFeatureFlagValuePreservesRest(t *testing.T) {
	in := `{"id":"beta","enabled":false,"conditions":{"client_filters":[{"name":"Percentage","parameters":{"Value":50}}]},"custom":"x"}`
	out, err := ToggleFeatureFlagValue(in, true)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatal(err)
	}
	if doc["enabled"] != true || doc["custom"] != "x" {
		t.Fatalf("toggle lost fields: %s", out)
	}
	filters := doc["conditions"].(map[string]any)["client_filters"].([]any)
	if len(filters) != 1 {
		t.Fatalf("filters lost: %s", out)
	}
	if _, err := ToggleFeatureFlagValue("{broken", true); err == nil {
		t.Fatal("broken value should error")
	}
}

func TestNewFeatureFlagValueAndName(t *testing.T) {
	ff, ok := ParseFeatureFlag(Setting{Key: FeatureFlagKey("new-ui"), ContentType: FeatureFlagContentType, Value: NewFeatureFlagValue("new-ui", "New UI", false)})
	if !ok || ff.ID != "new-ui" || ff.Description != "New UI" || ff.Enabled || ff.HasFilters {
		t.Fatalf("new flag parsed = %+v ok=%v", ff, ok)
	}
	for _, bad := range []string{"", "a/b", "a:b", "a*b"} {
		if ValidateFeatureFlagName(bad) == "" {
			t.Errorf("ValidateFeatureFlagName(%q) accepted", bad)
		}
	}
	if msg := ValidateFeatureFlagName("new-ui"); msg != "" {
		t.Errorf("good name rejected: %s", msg)
	}
}
