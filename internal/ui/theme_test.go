package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFallbackScheme(t *testing.T) {
	s := FallbackScheme()
	if s.Name != "fallback" {
		t.Fatalf("expected name=fallback, got %s", s.Name)
	}
	if s.Base0D == "" {
		t.Fatal("expected Base0D to be non-empty")
	}
}

func TestEnsureStockThemes_WritesWhenMissing(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "themes")
	ensureStockThemes(dir)

	for _, name := range []string{"bamboo.yaml", "default-dark.yaml", "rose-pine.yaml", "dracula.yaml"} {
		path := filepath.Join(dir, name)
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected stock theme %s to be written, got err: %v", name, err)
		}
	}
}

func TestEnsureStockThemes_OverwritesStockFiles(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "themes")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	old := []byte("name: dracula\nui_colors:\n  accent: \"#old\"\n")
	if err := os.WriteFile(filepath.Join(dir, "dracula.yaml"), old, 0o644); err != nil {
		t.Fatal(err)
	}

	ensureStockThemes(dir)

	data, err := os.ReadFile(filepath.Join(dir, "dracula.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) == string(old) {
		t.Fatal("expected stock theme to be overwritten with new format")
	}
	if !strings.Contains(string(data), "base16") {
		t.Fatal("expected overwritten file to contain base16 format")
	}
}

func TestEnsureStockThemes_DoesNotTouchUserFiles(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "themes")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	custom := []byte("system: base16\nname: my-custom\npalette:\n  base0D: \"abcdef\"\n")
	if err := os.WriteFile(filepath.Join(dir, "my-custom.yaml"), custom, 0o644); err != nil {
		t.Fatal(err)
	}

	ensureStockThemes(dir)

	data, err := os.ReadFile(filepath.Join(dir, "my-custom.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(custom) {
		t.Fatal("expected user theme file to be untouched")
	}
}

func TestLoadConfig_MissingFile(t *testing.T) {
	cfg := loadConfigFromDir(t.TempDir())
	if cfg.ThemeName != "Default Dark" {
		t.Fatalf("expected theme=Default Dark, got %s", cfg.ThemeName)
	}
	if len(cfg.Schemes) < 100 {
		t.Fatalf("expected many schemes (auto-created from embedded), got %d", len(cfg.Schemes))
	}
}

func TestLoadConfig_ThemeName(t *testing.T) {
	dir := t.TempDir()
	data := []byte("theme: \"Rose Pine\"\n")
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := loadConfigFromDir(dir)
	if cfg.ThemeName != "Rose Pine" {
		t.Fatalf("expected theme=Rose Pine, got %s", cfg.ThemeName)
	}

	active := cfg.ActiveScheme()
	if active.Name != "Rose Pine" {
		t.Fatalf("expected active scheme=Rose Pine, got %s", active.Name)
	}
	if active.Base0B == "" {
		t.Fatal("expected Base0B to be non-empty")
	}
}

func TestLoadConfig_CreatesConfigFileWhenMissing(t *testing.T) {
	dir := t.TempDir()

	cfg := loadConfigFromDir(dir)
	if cfg.ThemeName != "Default Dark" {
		t.Fatalf("expected theme=Default Dark, got %s", cfg.ThemeName)
	}

	data, err := os.ReadFile(filepath.Join(dir, "config.yaml"))
	if err != nil {
		t.Fatalf("expected config.yaml to be created: %v", err)
	}
	if !strings.Contains(string(data), "Default") {
		t.Fatalf("expected config.yaml to contain Default, got %s", string(data))
	}
}

func TestActiveScheme_FallbackToFirst(t *testing.T) {
	fb := FallbackScheme()
	cfg := Config{ThemeName: "nonexistent", Schemes: []Scheme{fb}}
	active := cfg.ActiveScheme()
	if active.Name != "fallback" {
		t.Fatalf("expected fallback to first scheme, got %s", active.Name)
	}
}

func TestNewStyles_ProducesSyntax(t *testing.T) {
	s := FallbackScheme()
	styles := NewStyles(s)

	rendered := styles.Syntax.HighlightJSON(`{"key":"value"}`)
	if rendered == "" {
		t.Fatal("expected non-empty rendered output")
	}
}

func TestLoadConfig_CustomBase16Scheme(t *testing.T) {
	dir := t.TempDir()
	themesDir := filepath.Join(dir, "themes")
	if err := os.MkdirAll(themesDir, 0o755); err != nil {
		t.Fatal(err)
	}

	custom := []byte(`system: "base16"
name: "My Custom"
palette:
  base00: "111111"
  base01: "222222"
  base02: "333333"
  base03: "444444"
  base04: "555555"
  base05: "666666"
  base06: "777777"
  base07: "888888"
  base08: "990000"
  base09: "009900"
  base0A: "000099"
  base0B: "999900"
  base0C: "009999"
  base0D: "990099"
  base0E: "999999"
  base0F: "000000"
`)
	if err := os.WriteFile(filepath.Join(themesDir, "mycustom.yaml"), custom, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("theme: \"My Custom\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := loadConfigFromDir(dir)
	if len(cfg.Schemes) < 100 {
		t.Fatalf("expected many schemes (stock + 1 custom), got %d", len(cfg.Schemes))
	}

	active := cfg.ActiveScheme()
	if active.Name != "My Custom" {
		t.Fatalf("expected active scheme=My Custom, got %s", active.Name)
	}
	if active.Base08 != "990000" {
		t.Fatalf("expected base08=990000, got %s", active.Base08)
	}
}

func TestMigrateOldConfig(t *testing.T) {
	dir := t.TempDir()
	data := []byte("theme: tokyonight\n")
	if err := os.WriteFile(filepath.Join(dir, "azblob.yaml"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	migrated := migrateOldConfig(dir)
	if migrated != "tokyonight" {
		t.Fatalf("expected migrated theme=tokyonight, got %s", migrated)
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
	schemes := []Scheme{
		{Name: "alpha"},
		{Name: "beta"},
		{Name: "gamma"},
	}

	tests := []struct {
		name       string
		setup      func() ThemeOverlayState
		key        string
		schemes    []Scheme
		wantReturn bool
		check      func(t *testing.T, s ThemeOverlayState)
	}{
		{
			name: "up from middle moves cursor up",
			setup: func() ThemeOverlayState {
				s := ThemeOverlayState{Active: true, CursorIdx: 1}
				s.refilter(schemes)
				return s
			},
			key:        "up",
			schemes:    schemes,
			wantReturn: false,
			check: func(t *testing.T, s ThemeOverlayState) {
				if s.CursorIdx != 0 {
					t.Fatalf("expected CursorIdx=0, got %d", s.CursorIdx)
				}
			},
		},
		{
			name: "down at bottom stays at last",
			setup: func() ThemeOverlayState {
				s := ThemeOverlayState{Active: true, CursorIdx: 2}
				s.refilter(schemes)
				return s
			},
			key:        "down",
			schemes:    schemes,
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
				s := ThemeOverlayState{Active: true, CursorIdx: 2, ActiveThemeIdx: 0}
				s.refilter(schemes)
				return s
			},
			key:        "enter",
			schemes:    schemes,
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
				s := ThemeOverlayState{Active: true, CursorIdx: 1}
				s.refilter(schemes)
				return s
			},
			key:        "esc",
			schemes:    schemes,
			wantReturn: false,
			check: func(t *testing.T, s ThemeOverlayState) {
				if s.Active {
					t.Fatal("expected Active=false after cancel")
				}
			},
		},
		{
			name: "empty schemes does not panic",
			setup: func() ThemeOverlayState {
				return ThemeOverlayState{Active: true, CursorIdx: 0}
			},
			key:        "down",
			schemes:    nil,
			wantReturn: false,
			check: func(t *testing.T, s ThemeOverlayState) {
				if s.Active {
					t.Fatal("expected Active=false with empty schemes")
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := tc.setup()
			got := s.HandleKey(tc.key, testBindings(), tc.schemes)
			if got != tc.wantReturn {
				t.Fatalf("expected HandleKey return=%v, got %v", tc.wantReturn, got)
			}
			tc.check(t, s)
		})
	}
}
