package keymap

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestBindingMatches(t *testing.T) {
	b := New("ctrl+c", "q")
	if !b.Matches("ctrl+c") {
		t.Fatal("expected ctrl+c to match")
	}
	if !b.Matches("q") {
		t.Fatal("expected q to match")
	}
	if b.Matches("x") {
		t.Fatal("expected x not to match")
	}
}

func TestBindingLabel(t *testing.T) {
	if got := New("ctrl+c", "q").Label(); got != "ctrl+c/q" {
		t.Fatalf("got %q, want ctrl+c/q", got)
	}
	if got := New(" ").Label(); got != "space" {
		t.Fatalf("got %q, want space", got)
	}
	if got := New().Label(); got != "" {
		t.Fatalf("got %q, want empty", got)
	}
}

func TestDefaultHasAllFields(t *testing.T) {
	km := Default()
	v := reflect.ValueOf(km)
	tt := v.Type()
	for i := 0; i < tt.NumField(); i++ {
		f := v.Field(i)
		b := f.Interface().(Binding)
		switch tt.Field(i).Name {
		case "OpenFocusedAlt", "NavigateLeft":
			continue
		}
		if len(b.Keys) == 0 {
			t.Errorf("Default() field %s has no keys", tt.Field(i).Name)
		}
	}
}

func TestAllFieldsHaveJSONTag(t *testing.T) {
	tt := reflect.TypeOf(Keymap{})
	for i := 0; i < tt.NumField(); i++ {
		tag := tt.Field(i).Tag.Get("json")
		if tag == "" || tag == "-" {
			t.Errorf("field %s has no json tag", tt.Field(i).Name)
		}
	}
}

func TestJSONRoundTrip(t *testing.T) {
	km := Default()

	// Build JSON from default.
	bindings := make(map[string][]string)
	v := reflect.ValueOf(km)
	tt := v.Type()
	for i := 0; i < tt.NumField(); i++ {
		tag := tt.Field(i).Tag.Get("json")
		b := v.Field(i).Interface().(Binding)
		bindings[tag] = b.Keys
	}

	// Merge back into a fresh default — should be identical.
	km2 := Default()
	mergeBindings(&km2, bindings)

	for i := 0; i < tt.NumField(); i++ {
		b1 := v.Field(i).Interface().(Binding)
		b2 := reflect.ValueOf(km2).Field(i).Interface().(Binding)
		if !reflect.DeepEqual(b1.Keys, b2.Keys) {
			t.Errorf("field %s mismatch after round-trip: %v != %v", tt.Field(i).Name, b1.Keys, b2.Keys)
		}
	}
}

func TestMergePartialOverride(t *testing.T) {
	km := Default()
	mergeBindings(&km, map[string][]string{
		"quit": {"ctrl+q"},
	})

	if !km.Quit.Matches("ctrl+q") {
		t.Fatal("expected quit to match ctrl+q after override")
	}
	if km.Quit.Matches("q") {
		t.Fatal("expected old q binding to be replaced")
	}
	// Other bindings should be unchanged.
	if !km.NextFocus.Matches("tab") {
		t.Fatal("expected next_focus to remain \"tab\"")
	}
	for _, key := range []string{"l", "right"} {
		if !km.OpenFocusedAlt.Matches(key) {
			t.Fatalf("expected open_focused_alt to remain %q", key)
		}
	}
}

func TestDefaultMillerColumnFocusKeys(t *testing.T) {
	km := Default()
	assertMillerColumnFocusKeys(t, km)
}

func TestLoadStockDefaultMillerColumnFocusKeys(t *testing.T) {
	km := Load(t.TempDir())
	assertMillerColumnFocusKeys(t, km)
}

func TestDefaultDashboardWidgetFocusKeys(t *testing.T) {
	km := Default()
	assertDashboardWidgetFocusKeys(t, km)
}

func TestLoadStockDefaultDashboardWidgetFocusKeys(t *testing.T) {
	km := Load(t.TempDir())
	assertDashboardWidgetFocusKeys(t, km)
}

func assertDashboardWidgetFocusKeys(t *testing.T, km Keymap) {
	t.Helper()
	// Dashboard reserves ctrl+h/j/k/l (and alt+) for spatial widget
	// navigation. Bare h/l/j/k stay free for explorer-style drill-in
	// inside scrollable widgets so the chord doesn't double-fire.
	for _, key := range []string{"ctrl+h", "alt+h"} {
		if !km.WidgetLeft.Matches(key) {
			t.Fatalf("widget_left should match %q", key)
		}
	}
	for _, key := range []string{"ctrl+l", "alt+l"} {
		if !km.WidgetRight.Matches(key) {
			t.Fatalf("widget_right should match %q", key)
		}
	}
	if km.WidgetLeft.Matches("h") || km.WidgetRight.Matches("l") {
		t.Fatalf("widget left/right must not consume bare h/l (reserved for drill-in)")
	}
	if km.WidgetUp.Matches("k") || km.WidgetDown.Matches("j") {
		t.Fatalf("widget row focus must not consume j/k row movement")
	}
}

func assertMillerColumnFocusKeys(t *testing.T, km Keymap) {
	t.Helper()
	// In Miller-column UIs (yazi, ranger), the focused column is always
	// the deepest one with data — moving "right" means drilling into
	// the selected row, not just shifting focus to a pre-rendered column.
	// h/l therefore map to drill-in / back, while tab / shift+tab keep
	// the conventional next/previous-focus role for the few non-Miller
	// places (overlays, preview pane).
	for _, key := range []string{"l", "right"} {
		if !km.OpenFocusedAlt.Matches(key) {
			t.Fatalf("open_focused_alt should match %q (drill-in)", key)
		}
	}
	for _, key := range []string{"h", "left"} {
		if !km.NavigateLeft.Matches(key) {
			t.Fatalf("navigate_left should match %q (back)", key)
		}
	}
	if !km.NextFocus.Matches("tab") || !km.PreviousFocus.Matches("shift+tab") {
		t.Fatalf("tab / shift+tab must keep next/previous-focus role")
	}
	if km.NextFocus.Matches("l") || km.PreviousFocus.Matches("h") {
		t.Fatalf("next/previous_focus must not steal h/l from drill-in")
	}
	if !km.BackspaceUp.Matches("backspace") {
		t.Fatalf("backspace should handle go up/back")
	}
}

func TestJumpIndex(t *testing.T) {
	km := Default()
	if idx, ok := km.JumpIndex("3"); !ok || idx != 2 {
		t.Fatalf("expected 3 → index 2, got %d, %v", idx, ok)
	}
	if _, ok := km.JumpIndex("x"); ok {
		t.Fatal("expected no match for x")
	}
}

func TestLoadFromDir(t *testing.T) {
	dir := t.TempDir()
	keymapsDir := filepath.Join(dir, "keymaps")
	os.MkdirAll(keymapsDir, 0o755)

	// Write a custom keymap.
	custom := map[string]any{
		"name": "custom",
		"bindings": map[string][]string{
			"quit": {"ctrl+q"},
		},
	}
	data, _ := json.Marshal(custom)
	os.WriteFile(filepath.Join(keymapsDir, "custom.json"), data, 0o644)

	// Write config selecting it.
	os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"keymap": "custom"}`), 0o644)

	km := Load(dir)
	if !km.Quit.Matches("ctrl+q") {
		t.Fatal("expected custom quit binding")
	}
	if km.Quit.Matches("q") {
		t.Fatal("expected old q binding replaced")
	}
	// Non-overridden binding should use default.
	if !km.NextFocus.Matches("tab") {
		t.Fatalf("expected default next_focus to match \"tab\"")
	}
	for _, key := range []string{"l", "right"} {
		if !km.OpenFocusedAlt.Matches(key) {
			t.Fatalf("expected default open_focused_alt to match %q", key)
		}
	}
}

func TestLoadInlineBindings(t *testing.T) {
	dir := t.TempDir()

	// Config with inline bindings and no keymap file reference.
	cfg := `{"bindings": {"quit": ["ctrl+q"], "next_focus": ["ctrl+n"]}}`
	os.WriteFile(filepath.Join(dir, "config.json"), []byte(cfg), 0o644)

	km := Load(dir)
	if !km.Quit.Matches("ctrl+q") {
		t.Fatal("expected inline quit binding")
	}
	if !km.NextFocus.Matches("ctrl+n") {
		t.Fatal("expected inline next_focus binding")
	}
	// Non-overridden binding should use default.
	if !km.RefreshScope.Matches("r") {
		t.Fatal("expected default refresh_scope")
	}
}

func TestLoadInlineBindingsOverrideKeymapFile(t *testing.T) {
	dir := t.TempDir()
	keymapsDir := filepath.Join(dir, "keymaps")
	os.MkdirAll(keymapsDir, 0o755)

	// Keymap file sets quit to ctrl+w.
	custom := map[string]any{
		"name":     "custom",
		"bindings": map[string][]string{"quit": {"ctrl+w"}},
	}
	data, _ := json.Marshal(custom)
	os.WriteFile(filepath.Join(keymapsDir, "custom.json"), data, 0o644)

	// Config selects the keymap but also inlines a quit override.
	cfg := `{"keymap": "custom", "bindings": {"quit": ["ctrl+q"]}}`
	os.WriteFile(filepath.Join(dir, "config.json"), []byte(cfg), 0o644)

	km := Load(dir)
	if !km.Quit.Matches("ctrl+q") {
		t.Fatal("expected inline binding to override keymap file")
	}
	if km.Quit.Matches("ctrl+w") {
		t.Fatal("expected keymap file binding to be overridden")
	}
}

func TestLoadFallsBackToDefault(t *testing.T) {
	km := Load("")
	if !km.Quit.Matches("ctrl+c") {
		t.Fatal("expected default quit binding on empty config dir")
	}
}

func TestEmbeddedDefaultJSON(t *testing.T) {
	data, err := stockKeymapsFS.ReadFile("keymaps/default.json")
	if err != nil {
		t.Fatalf("embedded default.json not found: %v", err)
	}

	var f keymapFile
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatalf("invalid embedded JSON: %v", err)
	}

	// Every struct field's json tag should have a corresponding entry.
	tt := reflect.TypeOf(Keymap{})
	for i := 0; i < tt.NumField(); i++ {
		tag := tt.Field(i).Tag.Get("json")
		if _, ok := f.Bindings[tag]; !ok {
			t.Errorf("embedded default.json missing key %q (field %s)", tag, tt.Field(i).Name)
		}
	}
}
