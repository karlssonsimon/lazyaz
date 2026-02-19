package sbapp

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultTheme(t *testing.T) {
	theme := defaultTheme()

	if theme.Name != "default" {
		t.Fatalf("expected name=default, got %s", theme.Name)
	}
	if theme.JSONColors.Key != "#C084FC" {
		t.Fatalf("expected key=#C084FC, got %s", theme.JSONColors.Key)
	}
	if theme.UIColors.Border != "#4B5563" {
		t.Fatalf("expected border=#4B5563, got %s", theme.UIColors.Border)
	}
}

func TestEnsureStockThemes_WritesWhenMissing(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "themes")
	ensureStockThemes(dir)

	for _, name := range []string{"default.yaml", "tokyonight.yaml", "rosepine.yaml"} {
		path := filepath.Join(dir, name)
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected stock theme %s to be written, got err: %v", name, err)
		}
	}
}

func TestEnsureStockThemes_DoesNotOverwrite(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "themes")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	custom := []byte("name: tokyonight\nui_colors:\n  accent: \"#custom\"\n")
	if err := os.WriteFile(filepath.Join(dir, "tokyonight.yaml"), custom, 0o644); err != nil {
		t.Fatal(err)
	}

	ensureStockThemes(dir)

	data, err := os.ReadFile(filepath.Join(dir, "tokyonight.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(custom) {
		t.Fatalf("expected stock theme not to overwrite existing file")
	}
}

func TestLoadConfig_MissingFile(t *testing.T) {
	cfg := loadConfigFromDir(t.TempDir())
	if cfg.ThemeName != "default" {
		t.Fatalf("expected theme=default, got %s", cfg.ThemeName)
	}
	if len(cfg.Themes) != 3 {
		t.Fatalf("expected 3 themes (auto-created from embedded), got %d", len(cfg.Themes))
	}
}

func TestLoadConfig_ThemeName(t *testing.T) {
	dir := t.TempDir()
	data := []byte("theme: rosepine\n")
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := loadConfigFromDir(dir)
	if cfg.ThemeName != "rosepine" {
		t.Fatalf("expected theme=rosepine, got %s", cfg.ThemeName)
	}

	active := cfg.ActiveTheme()
	if active.Name != "rosepine" {
		t.Fatalf("expected active theme=rosepine, got %s", active.Name)
	}
	if active.UIColors.Accent != "#c4a7e7" {
		t.Fatalf("expected rosepine accent=#c4a7e7, got %s", active.UIColors.Accent)
	}
}

func TestLoadConfig_UserThemeOverridesBuiltin(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("theme: tokyonight\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	themesDir := filepath.Join(dir, "themes")
	if err := os.MkdirAll(themesDir, 0o755); err != nil {
		t.Fatal(err)
	}

	customTheme := []byte("name: tokyonight\njson_colors:\n  key: \"#ff0000\"\nui_colors:\n  border: \"#111111\"\n")
	if err := os.WriteFile(filepath.Join(themesDir, "tokyonight.yaml"), customTheme, 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := loadConfigFromDir(dir)
	active := cfg.ActiveTheme()
	if active.JSONColors.Key != "#ff0000" {
		t.Fatalf("expected overridden key=#ff0000, got %s", active.JSONColors.Key)
	}
	if active.UIColors.Border != "#111111" {
		t.Fatalf("expected overridden border=#111111, got %s", active.UIColors.Border)
	}
	if active.JSONColors.String != defaultTheme().JSONColors.String {
		t.Fatalf("expected merged string=%s, got %s", defaultTheme().JSONColors.String, active.JSONColors.String)
	}
}

func TestLoadConfig_UserThemeFromFilename(t *testing.T) {
	dir := t.TempDir()
	themesDir := filepath.Join(dir, "themes")
	if err := os.MkdirAll(themesDir, 0o755); err != nil {
		t.Fatal(err)
	}

	customTheme := []byte("json_colors:\n  key: \"#abcdef\"\nui_colors:\n  accent: \"#fedcba\"\n")
	if err := os.WriteFile(filepath.Join(themesDir, "mycustom.yaml"), customTheme, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("theme: mycustom\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := loadConfigFromDir(dir)
	if len(cfg.Themes) != 4 {
		t.Fatalf("expected 4 themes (3 stock + 1 custom), got %d", len(cfg.Themes))
	}

	active := cfg.ActiveTheme()
	if active.Name != "mycustom" {
		t.Fatalf("expected active theme=mycustom, got %s", active.Name)
	}
	if active.JSONColors.Key != "#abcdef" {
		t.Fatalf("expected key=#abcdef, got %s", active.JSONColors.Key)
	}
	if active.UIColors.Accent != "#fedcba" {
		t.Fatalf("expected accent=#fedcba, got %s", active.UIColors.Accent)
	}
}

func TestActiveTheme_FallbackToDefault(t *testing.T) {
	cfg := Config{ThemeName: "nonexistent", Themes: []Theme{defaultTheme()}}
	active := cfg.ActiveTheme()
	if active.Name != "default" {
		t.Fatalf("expected fallback to default, got %s", active.Name)
	}
}

func TestJSONColors_Styles(t *testing.T) {
	theme := defaultTheme()
	s := syntaxStylesForTheme(theme)

	rendered := s.HighlightJSON(`{"key":"value"}`)
	if rendered == "" {
		t.Fatal("expected non-empty rendered output from key style")
	}
}
