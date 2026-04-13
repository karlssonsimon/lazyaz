package keymap

import (
	"embed"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
)

//go:embed keymaps/*.json
var stockKeymapsFS embed.FS

type keymapFile struct {
	Name     string              `json:"name"`
	Bindings map[string][]string `json:"bindings"`
}

// Load reads the active keymap from the config directory. Stock keymaps are
// copied to configDir/keymaps/ if missing. The active keymap name is read
// from configDir/config.json (field "keymap", default "default"). Any
// bindings present in the keymap file override the defaults; omitted actions
// keep their default bindings. Inline "bindings" in config.json are applied
// on top of the keymap file, allowing per-user overrides without a custom
// keymap file.
func Load(configDir string) Keymap {
	if configDir == "" {
		return Default()
	}

	keymapsDir := filepath.Join(configDir, "keymaps")
	ensureStockKeymaps(keymapsDir)

	name, inlineBindings := readKeymapConfig(configDir)
	km := Default()

	path := filepath.Join(keymapsDir, name+".json")
	data, err := os.ReadFile(path)
	if err == nil {
		var f keymapFile
		if json.Unmarshal(data, &f) == nil {
			mergeBindings(&km, f.Bindings)
		}
	}

	// Inline bindings from config.json override the keymap file.
	if len(inlineBindings) > 0 {
		mergeBindings(&km, inlineBindings)
	}

	return km
}

func ensureStockKeymaps(keymapsDir string) {
	if err := os.MkdirAll(keymapsDir, 0o755); err != nil {
		return
	}
	entries, err := fs.ReadDir(stockKeymapsFS, "keymaps")
	if err != nil {
		return
	}
	for _, e := range entries {
		data, err := stockKeymapsFS.ReadFile("keymaps/" + e.Name())
		if err != nil {
			continue
		}
		os.WriteFile(filepath.Join(keymapsDir, e.Name()), data, 0o644)
	}
}

// readKeymapConfig reads the keymap name and optional inline bindings from
// config.json. Returns ("default", nil) if the file is missing or invalid.
func readKeymapConfig(configDir string) (string, map[string][]string) {
	data, err := os.ReadFile(filepath.Join(configDir, "config.json"))
	if err != nil {
		return "default", nil
	}
	var cfg struct {
		Keymap   string              `json:"keymap"`
		Bindings map[string][]string `json:"bindings"`
	}
	if json.Unmarshal(data, &cfg) != nil {
		return "default", nil
	}
	name := cfg.Keymap
	if name == "" {
		name = "default"
	}
	return name, cfg.Bindings
}

// mergeBindings overlays JSON bindings onto the keymap struct using
// reflection on json tags.
func mergeBindings(km *Keymap, bindings map[string][]string) {
	v := reflect.ValueOf(km).Elem()
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		tag := t.Field(i).Tag.Get("json")
		if tag == "" || tag == "-" {
			continue
		}
		if keys, ok := bindings[tag]; ok {
			v.Field(i).Set(reflect.ValueOf(Binding{Keys: keys}))
		}
	}
}
