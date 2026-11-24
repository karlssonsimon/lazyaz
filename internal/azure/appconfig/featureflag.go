package appconfig

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Feature flags are key-values with a reserved key prefix and content
// type whose value is a JSON document; "enabled" is the switch. The
// document can carry filters and other fields that must survive a
// toggle untouched, so edits go through a map rather than a struct.

// FeatureFlagContentType is the content type that marks a feature flag.
const FeatureFlagContentType = "application/vnd.microsoft.appconfig.ff+json;charset=utf-8"

// FeatureFlag is the parsed state of a flag's value.
type FeatureFlag struct {
	ID          string
	Description string
	Enabled     bool
	// HasFilters is true when the flag carries client filters, so a
	// plain on/off is not the whole story.
	HasFilters bool
}

// ParseFeatureFlag reads a flag's value. ok is false for settings that
// are not flags or whose value does not parse.
func ParseFeatureFlag(s Setting) (FeatureFlag, bool) {
	if !s.IsFeatureFlag() {
		return FeatureFlag{}, false
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(s.Value), &doc); err != nil {
		return FeatureFlag{}, false
	}
	ff := FeatureFlag{ID: s.DisplayKey()}
	if id, ok := doc["id"].(string); ok && id != "" {
		ff.ID = id
	}
	if d, ok := doc["description"].(string); ok {
		ff.Description = d
	}
	if e, ok := doc["enabled"].(bool); ok {
		ff.Enabled = e
	}
	if cond, ok := doc["conditions"].(map[string]any); ok {
		if filters, ok := cond["client_filters"].([]any); ok && len(filters) > 0 {
			ff.HasFilters = true
		}
	}
	return ff, true
}

// ToggleFeatureFlagValue returns the flag's value with "enabled" set,
// every other field preserved as the document had it.
func ToggleFeatureFlagValue(value string, enabled bool) (string, error) {
	var doc map[string]any
	if err := json.Unmarshal([]byte(value), &doc); err != nil {
		return "", fmt.Errorf("feature flag value is not valid JSON: %w", err)
	}
	doc["enabled"] = enabled
	out, err := json.Marshal(doc)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// NewFeatureFlagValue builds the value for a new flag in the shape the
// portal creates: id, description, enabled, and an empty filter list.
func NewFeatureFlagValue(id, description string, enabled bool) string {
	doc := map[string]any{
		"id":          id,
		"description": description,
		"enabled":     enabled,
		"conditions":  map[string]any{"client_filters": []any{}},
	}
	out, _ := json.Marshal(doc)
	return string(out)
}

// FeatureFlagKey is the stored key for a flag name.
func FeatureFlagKey(name string) string {
	return FeatureFlagPrefix + name
}

// ValidateFeatureFlagName checks a flag name: it becomes part of the
// key, so the key rules apply, and the portal also refuses ":" and "/".
func ValidateFeatureFlagName(name string) string {
	switch {
	case strings.TrimSpace(name) == "":
		return "name is required"
	case strings.ContainsAny(name, "*,\\:/"):
		return `must not contain "*", ",", "\", ":" or "/"`
	case len(name) > 1000:
		return "must be at most 1,000 characters"
	}
	return ""
}
