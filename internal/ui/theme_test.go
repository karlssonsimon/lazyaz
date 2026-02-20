package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultTheme(t *testing.T) {
	theme := DefaultTheme()

	if theme.Name != "default" {
		t.Fatalf("expected name=default, got %s", theme.Name)
	}
	if theme.SyntaxColorConfig.Key != "#C084FC" {
		t.Fatalf("expected key=#C084FC, got %s", theme.SyntaxColorConfig.Key)
	}
	if theme.Colors.Border != "#4B5563" {
		t.Fatalf("expected border=#4B5563, got %s", theme.Colors.Border)
	}
}

func TestEnsureStockThemes_WritesWhenMissing(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "themes")
	ensureStockThemes(dir)

	for _, name := range []string{"bamboo.yaml", "default.yaml", "tokyonight.yaml", "rosepine.yaml"} {
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
	cfg := loadConfigFromDir(t.TempDir(), "testapp")
	if cfg.ThemeName != "default" {
		t.Fatalf("expected theme=default, got %s", cfg.ThemeName)
	}
	if cfg.AppName != "testapp" {
		t.Fatalf("expected appName=testapp, got %s", cfg.AppName)
	}
	if len(cfg.Themes) != 4 {
		t.Fatalf("expected 4 themes (auto-created from embedded), got %d", len(cfg.Themes))
	}
}

func TestLoadConfig_ThemeName(t *testing.T) {
	dir := t.TempDir()
	data := []byte("theme: rosepine\n")
	if err := os.WriteFile(filepath.Join(dir, "azsb.yaml"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := loadConfigFromDir(dir, "azsb")
	if cfg.ThemeName != "rosepine" {
		t.Fatalf("expected theme=rosepine, got %s", cfg.ThemeName)
	}

	active := cfg.ActiveTheme()
	if active.Name != "rosepine" {
		t.Fatalf("expected active theme=rosepine, got %s", active.Name)
	}
	if active.Colors.Accent != "#c4a7e7" {
		t.Fatalf("expected rosepine accent=#c4a7e7, got %s", active.Colors.Accent)
	}
}

func TestLoadConfig_UserThemeOverridesBuiltin(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "azblob.yaml"), []byte("theme: tokyonight\n"), 0o644); err != nil {
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

	cfg := loadConfigFromDir(dir, "azblob")
	active := cfg.ActiveTheme()
	if active.SyntaxColorConfig.Key != "#ff0000" {
		t.Fatalf("expected overridden key=#ff0000, got %s", active.SyntaxColorConfig.Key)
	}
	if active.Colors.Border != "#111111" {
		t.Fatalf("expected overridden border=#111111, got %s", active.Colors.Border)
	}
	if active.SyntaxColorConfig.String != DefaultTheme().SyntaxColorConfig.String {
		t.Fatalf("expected merged string=%s, got %s", DefaultTheme().SyntaxColorConfig.String, active.SyntaxColorConfig.String)
	}
}

func TestLoadConfig_MultiWordPaletteFieldsDeserialize(t *testing.T) {
	dir := t.TempDir()
	themesDir := filepath.Join(dir, "themes")
	if err := os.MkdirAll(themesDir, 0o755); err != nil {
		t.Fatal(err)
	}

	theme := []byte(`name: custom
ui_colors:
  border_focused: "#aaaaaa"
  accent_strong: "#bbbbbb"
  filter_match: "#cccccc"
  selected_bg: "#dddddd"
  selected_text: "#eeeeee"
`)
	if err := os.WriteFile(filepath.Join(themesDir, "custom.yaml"), theme, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "testapp.yaml"), []byte("theme: custom\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := loadConfigFromDir(dir, "testapp")
	active := cfg.ActiveTheme()

	checks := map[string]struct{ got, want string }{
		"BorderFocused": {active.Colors.BorderFocused, "#aaaaaa"},
		"AccentStrong":  {active.Colors.AccentStrong, "#bbbbbb"},
		"FilterMatch":   {active.Colors.FilterMatch, "#cccccc"},
		"SelectedBg":    {active.Colors.SelectedBg, "#dddddd"},
		"SelectedText":  {active.Colors.SelectedText, "#eeeeee"},
	}
	for field, c := range checks {
		if c.got != c.want {
			t.Errorf("Palette.%s: got %s, want %s", field, c.got, c.want)
		}
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

	if err := os.WriteFile(filepath.Join(dir, "testapp.yaml"), []byte("theme: mycustom\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := loadConfigFromDir(dir, "testapp")
	if len(cfg.Themes) != 5 {
		t.Fatalf("expected 5 themes (4 stock + 1 custom), got %d", len(cfg.Themes))
	}

	active := cfg.ActiveTheme()
	if active.Name != "mycustom" {
		t.Fatalf("expected active theme=mycustom, got %s", active.Name)
	}
	if active.SyntaxColorConfig.Key != "#abcdef" {
		t.Fatalf("expected key=#abcdef, got %s", active.SyntaxColorConfig.Key)
	}
	if active.Colors.Accent != "#fedcba" {
		t.Fatalf("expected accent=#fedcba, got %s", active.Colors.Accent)
	}
}

func TestLoadConfig_CreatesAppFileWhenMissing(t *testing.T) {
	dir := t.TempDir()

	cfg := loadConfigFromDir(dir, "azblob")
	if cfg.ThemeName != "default" {
		t.Fatalf("expected theme=default, got %s", cfg.ThemeName)
	}

	data, err := os.ReadFile(filepath.Join(dir, "azblob.yaml"))
	if err != nil {
		t.Fatalf("expected azblob.yaml to be created: %v", err)
	}
	if !strings.Contains(string(data), "default") {
		t.Fatalf("expected azblob.yaml to contain default, got %s", string(data))
	}
}

func TestActiveTheme_FallbackToDefault(t *testing.T) {
	cfg := Config{ThemeName: "nonexistent", Themes: []Theme{DefaultTheme()}}
	active := cfg.ActiveTheme()
	if active.Name != "default" {
		t.Fatalf("expected fallback to default, got %s", active.Name)
	}
}

func TestSyntaxColorConfig_Styles(t *testing.T) {
	theme := DefaultTheme()
	s := SyntaxStylesForTheme(theme)

	rendered := s.HighlightJSON(`{"key":"value"}`)
	if rendered == "" {
		t.Fatal("expected non-empty rendered output from key style")
	}
}

type testKey string

func (k testKey) Matches(key string) bool { return string(k) == key }

func testBindings() ThemeKeyBindings {
	return ThemeKeyBindings{
		Up:     testKey("up"),
		Down:   testKey("down"),
		Apply:  testKey("enter"),
		Cancel: testKey("esc"),
	}
}

func TestHandleKey(t *testing.T) {
	themes := []Theme{
		{Name: "alpha"},
		{Name: "beta"},
		{Name: "gamma"},
	}

	tests := []struct {
		name       string
		setup      func() ThemeOverlayState
		key        string
		themes     []Theme
		wantReturn bool
		check      func(t *testing.T, s ThemeOverlayState)
	}{
		{
			name: "up from middle moves cursor up",
			setup: func() ThemeOverlayState {
				return ThemeOverlayState{Active: true, CursorIdx: 1}
			},
			key:        "up",
			themes:     themes,
			wantReturn: false,
			check: func(t *testing.T, s ThemeOverlayState) {
				if s.CursorIdx != 0 {
					t.Fatalf("expected CursorIdx=0, got %d", s.CursorIdx)
				}
			},
		},
		{
			name: "up at top stays at zero",
			setup: func() ThemeOverlayState {
				return ThemeOverlayState{Active: true, CursorIdx: 0}
			},
			key:        "up",
			themes:     themes,
			wantReturn: false,
			check: func(t *testing.T, s ThemeOverlayState) {
				if s.CursorIdx != 0 {
					t.Fatalf("expected CursorIdx=0, got %d", s.CursorIdx)
				}
			},
		},
		{
			name: "down from middle moves cursor down",
			setup: func() ThemeOverlayState {
				return ThemeOverlayState{Active: true, CursorIdx: 1}
			},
			key:        "down",
			themes:     themes,
			wantReturn: false,
			check: func(t *testing.T, s ThemeOverlayState) {
				if s.CursorIdx != 2 {
					t.Fatalf("expected CursorIdx=2, got %d", s.CursorIdx)
				}
			},
		},
		{
			name: "down at bottom stays at last",
			setup: func() ThemeOverlayState {
				return ThemeOverlayState{Active: true, CursorIdx: 2}
			},
			key:        "down",
			themes:     themes,
			wantReturn: false,
			check: func(t *testing.T, s ThemeOverlayState) {
				if s.CursorIdx != 2 {
					t.Fatalf("expected CursorIdx=2, got %d", s.CursorIdx)
				}
			},
		},
		{
			name: "apply sets active index and returns true",
			setup: func() ThemeOverlayState {
				return ThemeOverlayState{Active: true, CursorIdx: 2, ActiveThemeIdx: 0}
			},
			key:        "enter",
			themes:     themes,
			wantReturn: true,
			check: func(t *testing.T, s ThemeOverlayState) {
				if s.ActiveThemeIdx != 2 {
					t.Fatalf("expected ActiveThemeIdx=2, got %d", s.ActiveThemeIdx)
				}
				if s.Active {
					t.Fatal("expected Active=false after apply")
				}
			},
		},
		{
			name: "cancel deactivates overlay",
			setup: func() ThemeOverlayState {
				return ThemeOverlayState{Active: true, CursorIdx: 1}
			},
			key:        "esc",
			themes:     themes,
			wantReturn: false,
			check: func(t *testing.T, s ThemeOverlayState) {
				if s.Active {
					t.Fatal("expected Active=false after cancel")
				}
			},
		},
		{
			name: "empty themes does not panic",
			setup: func() ThemeOverlayState {
				return ThemeOverlayState{Active: true, CursorIdx: 0}
			},
			key:        "down",
			themes:     nil,
			wantReturn: false,
			check: func(t *testing.T, s ThemeOverlayState) {
				if s.Active {
					t.Fatal("expected Active=false with empty themes")
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := tc.setup()
			got := s.HandleKey(tc.key, testBindings(), tc.themes)
			if got != tc.wantReturn {
				t.Fatalf("expected HandleKey return=%v, got %v", tc.wantReturn, got)
			}
			tc.check(t, s)
		})
	}
}
