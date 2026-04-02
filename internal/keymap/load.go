package keymap

import (
	"embed"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"

	"gopkg.in/yaml.v3"
)

//go:embed keymaps/*.json
var stockKeymapsFS embed.FS

type keymapFile struct {
	Name     string              `json:"name"`
	Bindings map[string][]string `json:"bindings"`
}

// Load reads the active keymap from the config directory. Stock keymaps are
// copied to configDir/keymaps/ if missing. The active keymap name is read
// from configDir/config.yaml (field "keymap", default "default"). Any
// bindings present in the JSON override the defaults; omitted actions keep
// their default bindings.
func Load(configDir string) Keymap {
	if configDir == "" {
		return Default()
	}

	keymapsDir := filepath.Join(configDir, "keymaps")
	ensureStockKeymaps(keymapsDir)

	name := activeKeymapName(configDir)
	km := Default()

	path := filepath.Join(keymapsDir, name+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return km
	}

	var f keymapFile
	if json.Unmarshal(data, &f) != nil {
		return km
	}

	mergeBindings(&km, f.Bindings)
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

func activeKeymapName(configDir string) string {
	data, err := os.ReadFile(filepath.Join(configDir, "config.yaml"))
	if err != nil {
		return "default"
	}
	var cfg struct {
		Keymap string `yaml:"keymap"`
	}
	if yaml.Unmarshal(data, &cfg) != nil || cfg.Keymap == "" {
		return "default"
	}
	return cfg.Keymap
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
