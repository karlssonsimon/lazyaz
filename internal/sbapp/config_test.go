package sbapp

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := defaultConfig()

	if cfg.JSONColors.Key != "#C084FC" {
		t.Fatalf("expected key=#C084FC, got %s", cfg.JSONColors.Key)
	}
	if cfg.JSONColors.String != "#4ADE80" {
		t.Fatalf("expected string=#4ADE80, got %s", cfg.JSONColors.String)
	}

	if cfg.UIColors.Border != "#4B5563" {
		t.Fatalf("expected border=#4B5563, got %s", cfg.UIColors.Border)
	}
	if cfg.UIColors.Accent != "#60A5FA" {
		t.Fatalf("expected accent=#60A5FA, got %s", cfg.UIColors.Accent)
	}
	if cfg.UIColors.Danger != "#F87171" {
		t.Fatalf("expected danger=#F87171, got %s", cfg.UIColors.Danger)
	}
}

func TestLoadConfig_MissingFile(t *testing.T) {
	cfg := loadConfigFromDir(t.TempDir())
	defaults := defaultConfig()
	if cfg != defaults {
		t.Fatalf("expected defaults when file missing, got %+v", cfg)
	}
}

func TestLoadConfig_PartialYAML(t *testing.T) {
	dir := t.TempDir()

	data := []byte("json_colors:\n  key: \"#ff0000\"\nui_colors:\n  border: \"#111111\"\n")
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := loadConfigFromDir(dir)
	if cfg.JSONColors.Key != "#ff0000" {
		t.Fatalf("expected key=#ff0000, got %s", cfg.JSONColors.Key)
	}
	if cfg.UIColors.Border != "#111111" {
		t.Fatalf("expected border=#111111, got %s", cfg.UIColors.Border)
	}

	defaults := defaultConfig()
	if cfg.JSONColors.String != defaults.JSONColors.String {
		t.Fatalf("expected string default %s, got %s", defaults.JSONColors.String, cfg.JSONColors.String)
	}
	if cfg.UIColors.Accent != defaults.UIColors.Accent {
		t.Fatalf("expected accent default %s, got %s", defaults.UIColors.Accent, cfg.UIColors.Accent)
	}
}

func TestLoadConfig_FullYAML(t *testing.T) {
	dir := t.TempDir()

	data := []byte(`json_colors:
  key: "#aaaaaa"
  string: "#bbbbbb"
  number: "#cccccc"
  bool: "#dddddd"
  "null": "#eeeeee"
  punctuation: "#ffffff"
ui_colors:
  border: "#111111"
  border_focused: "#222222"
  text: "#333333"
  muted: "#444444"
  accent: "#555555"
  accent_strong: "#666666"
  danger: "#777777"
  filter_match: "#888888"
  selected_bg: "#999999"
  selected_text: "#aaaaaa"
`)
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := loadConfigFromDir(dir)
	if cfg.JSONColors.Key != "#aaaaaa" {
		t.Fatalf("expected key=#aaaaaa, got %s", cfg.JSONColors.Key)
	}
	if cfg.UIColors.Border != "#111111" {
		t.Fatalf("expected border=#111111, got %s", cfg.UIColors.Border)
	}
	if cfg.UIColors.SelectedText != "#aaaaaa" {
		t.Fatalf("expected selected_text=#aaaaaa, got %s", cfg.UIColors.SelectedText)
	}
}

func TestJSONColors_Styles(t *testing.T) {
	cfg := defaultConfig()
	s := cfg.JSONColors.styles()

	rendered := s.key.Render("test")
	if rendered == "" {
		t.Fatal("expected non-empty rendered output from key style")
	}
}
