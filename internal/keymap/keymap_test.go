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
		t.Fatal("expected next_focus to remain tab")
	}
}

func TestJumpIndex(t *testing.T) {
	km := Default()
	if idx, ok := km.JumpIndex("alt+3"); !ok || idx != 2 {
		t.Fatalf("expected alt+3 → index 2, got %d, %v", idx, ok)
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
	os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("keymap: custom\n"), 0o644)

	km := Load(dir)
	if !km.Quit.Matches("ctrl+q") {
		t.Fatal("expected custom quit binding")
	}
	if km.Quit.Matches("q") {
		t.Fatal("expected old q binding replaced")
	}
	// Non-overridden binding should use default.
	if !km.NextFocus.Matches("tab") {
		t.Fatal("expected default next_focus")
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
