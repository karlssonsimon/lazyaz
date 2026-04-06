package ui

import (
	"embed"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

//go:embed themes/*.yaml
var stockThemesFS embed.FS

// ConfigDir returns the shared configuration directory.
func ConfigDir() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "lazyaz")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "lazyaz")
}

// LoadConfig loads the shared configuration and all available Base16 schemes.
func LoadConfig() Config {
	dir := ConfigDir()
	if dir == "" {
		fb := FallbackScheme()
		return Config{ThemeName: fb.Name, Schemes: []Scheme{fb}}
	}
	return loadConfigFromDir(dir)
}

func loadConfigFromDir(dir string) Config {
	cfg := Config{
		ThemeName: "Default Dark",
	}

	// Read shared config file.
	cfgFile := filepath.Join(dir, "config.yaml")
	data, err := os.ReadFile(cfgFile)
	if err == nil {
		var fileCfg struct {
			ThemeName string      `yaml:"theme"`
			Tabs      []TabConfig `yaml:"tabs"`
		}
		if yaml.Unmarshal(data, &fileCfg) == nil {
			if fileCfg.ThemeName != "" {
				cfg.ThemeName = fileCfg.ThemeName
			}
			cfg.Tabs = fileCfg.Tabs
		}
	} else {
		// Migrate from old per-app config files if config.yaml doesn't exist.
		if migrated := migrateOldConfig(dir); migrated != "" {
			cfg.ThemeName = migrated
		}
		saveThemeNameToFile(cfgFile, cfg.ThemeName)
	}

	// Ensure stock themes are present.
	themesDir := filepath.Join(dir, "themes")
	ensureStockThemes(themesDir)

	// Load all Base16 scheme files.
	cfg.Schemes = loadSchemes(themesDir)

	// Merge missing fields against the fallback.
	fb := FallbackScheme()
	for i := range cfg.Schemes {
		mergeSchemeDefaults(&fb, &cfg.Schemes[i])
	}

	if len(cfg.Schemes) == 0 {
		cfg.Schemes = []Scheme{FallbackScheme()}
	}

	// Migrate old theme names to new Base16 scheme names.
	cfg.ThemeName = migrateThemeName(cfg.ThemeName)

	return cfg
}

func ensureStockThemes(themesDir string) {
	if err := os.MkdirAll(themesDir, 0o755); err != nil {
		return
	}
	entries, err := fs.ReadDir(stockThemesFS, "themes")
	if err != nil {
		return
	}
	for _, e := range entries {
		dest := filepath.Join(themesDir, e.Name())
		stockData, err := stockThemesFS.ReadFile("themes/" + e.Name())
		if err != nil {
			continue
		}
		// Always overwrite stock themes to pick up format changes.
		// User-created themes (not matching a stock filename) are never touched.
		os.WriteFile(dest, stockData, 0o644)
	}
}

func loadSchemes(themesDir string) []Scheme {
	entries, err := filepath.Glob(filepath.Join(themesDir, "*.yaml"))
	if err != nil {
		return nil
	}

	var schemes []Scheme
	for _, path := range entries {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var sf schemeFile
		if yaml.Unmarshal(data, &sf) != nil {
			continue
		}
		s := parseSchemeFile(sf)
		if s.Name == "" {
			stem := filepath.Base(path)
			s.Name = strings.TrimSuffix(stem, filepath.Ext(stem))
		}
		schemes = append(schemes, s)
	}
	return schemes
}

// SaveThemeName persists the theme name to the shared config file.
func SaveThemeName(name string) {
	dir := ConfigDir()
	if dir == "" {
		return
	}
	saveThemeNameToFile(filepath.Join(dir, "config.yaml"), name)
}

func saveThemeNameToFile(path, name string) {
	cfg := make(map[string]any)
	if data, err := os.ReadFile(path); err == nil {
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			cfg = make(map[string]any)
		}
	}
	if cfg == nil {
		cfg = make(map[string]any)
	}
	cfg["theme"] = name

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return
	}
	os.MkdirAll(filepath.Dir(path), 0o755)
	os.WriteFile(path, data, 0o644)
}

// migrateThemeName maps old custom theme names to new Base16 scheme names.
func migrateThemeName(name string) string {
	migrations := map[string]string{
		"default":      "Default Dark",
		"Default":      "Default Dark",
		"tokyonight":   "Tokyo Night Dark",
		"Tokyo Night":  "Tokyo Night Dark",
		"rosepine":     "Rose Pine",
		"Rosé Pine":   "Rose Pine",
		"bamboo":       "Bamboo",
	}
	if newName, ok := migrations[name]; ok {
		return newName
	}
	return name
}

// migrateOldConfig checks for old per-app config files and returns
// the theme name from the first one found.
func migrateOldConfig(dir string) string {
	for _, name := range []string{"aztui", "azblob", "azsb", "azkv"} {
		path := filepath.Join(dir, name+".yaml")
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var old struct {
			ThemeName string `yaml:"theme"`
		}
		if yaml.Unmarshal(data, &old) == nil && old.ThemeName != "" {
			return old.ThemeName
		}
	}
	return ""
}
