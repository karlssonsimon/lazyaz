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

func ConfigDir() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "aztools")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "aztools")
}

func LoadConfig(appName string) Config {
	dir := ConfigDir()
	if dir == "" {
		return Config{AppName: appName, ThemeName: "default", Themes: []Theme{DefaultTheme()}}
	}
	return loadConfigFromDir(dir, appName)
}

func loadConfigFromDir(dir, appName string) Config {
	cfg := Config{
		AppName:   appName,
		ThemeName: "default",
	}

	appFile := filepath.Join(dir, appName+".yaml")
	data, err := os.ReadFile(appFile)
	if err == nil {
		var fileCfg struct {
			ThemeName string `yaml:"theme"`
		}
		if yaml.Unmarshal(data, &fileCfg) == nil && fileCfg.ThemeName != "" {
			cfg.ThemeName = fileCfg.ThemeName
		}
	} else {
		// Auto-create the config file so the user can discover and edit it.
		saveThemeNameToDir(dir, appName, cfg.ThemeName)
	}

	themesDir := filepath.Join(dir, "themes")
	ensureStockThemes(themesDir)

	themes := loadUserThemes(themesDir)
	def := DefaultTheme()
	for i := range themes {
		mergeStringFields(&def.SyntaxColorConfig, &themes[i].SyntaxColorConfig)
		mergeStringFields(&def.Colors, &themes[i].Colors)
	}
	cfg.Themes = themes

	if len(cfg.Themes) == 0 {
		cfg.Themes = []Theme{DefaultTheme()}
	}

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
		if _, err := os.Stat(dest); err == nil {
			continue
		}
		data, err := stockThemesFS.ReadFile("themes/" + e.Name())
		if err != nil {
			continue
		}
		os.WriteFile(dest, data, 0o644)
	}
}

func loadUserThemes(themesDir string) []Theme {
	entries, err := filepath.Glob(filepath.Join(themesDir, "*.yaml"))
	if err != nil {
		return nil
	}

	var themes []Theme
	for _, path := range entries {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var t Theme
		if yaml.Unmarshal(data, &t) != nil {
			continue
		}
		if t.Name == "" {
			stem := filepath.Base(path)
			t.Name = strings.TrimSuffix(stem, filepath.Ext(stem))
		}
		themes = append(themes, t)
	}
	return themes
}

func SaveThemeName(appName, name string) {
	dir := ConfigDir()
	if dir == "" {
		return
	}
	saveThemeNameToDir(dir, appName, name)
}

func saveThemeNameToDir(dir, appName, name string) {
	path := filepath.Join(dir, appName+".yaml")

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
	os.MkdirAll(dir, 0o755)
	os.WriteFile(path, data, 0o644)
}
