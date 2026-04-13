package ui

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
)

// homeDir is split out so tests could swap it; in normal use it's just
// os.UserHomeDir.
var homeDir = os.UserHomeDir

// schemeFile is the YAML structure of a Base16 scheme file.
// The palette field contains the 16 base colors as hex strings (no #).
type schemeFile struct {
	System  string            `yaml:"system"`
	Name    string            `yaml:"name"`
	Author  string            `yaml:"author"`
	Variant string            `yaml:"variant"`
	Palette map[string]string `yaml:"palette"`
}

// Config holds the loaded configuration and available schemes.
type Config struct {
	ThemeName   string      `json:"theme"`
	DownloadDir string      `json:"download_dir"`
	Tabs        []TabConfig `json:"tabs"`
	Schemes     []Scheme    `json:"-"`
}

// ResolvedDownloadDir returns the directory under which marked blobs
// should be saved. Resolution order:
//
//  1. If DownloadDir is configured, expand `~`/`~/...` against $HOME
//     and return it as-is (absolute or relative — caller's choice).
//  2. Otherwise return the OS-conventional user Downloads folder
//     (`$HOME/Downloads` on Unix and macOS, `%USERPROFILE%\Downloads`
//     on Windows — both resolved via os.UserHomeDir).
//
// Returns an empty string only if neither the configured nor the
// default path can be resolved (no $HOME and no explicit config).
// Callers MUST treat that as an error and surface it to the user
// instead of inventing a fallback path — silent fallbacks hide the
// problem and put files somewhere unexpected.
func (c Config) ResolvedDownloadDir() string {
	if c.DownloadDir != "" {
		if c.DownloadDir == "~" {
			home, err := homeDir()
			if err != nil {
				return ""
			}
			return home
		}
		if strings.HasPrefix(c.DownloadDir, "~/") {
			home, err := homeDir()
			if err != nil {
				return ""
			}
			return home + c.DownloadDir[1:]
		}
		return c.DownloadDir
	}
	home, err := homeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, "Downloads")
}

// TabConfig is one startup-tab entry from the user config. Kind names
// the tab type (matched case-insensitively against "blob",
// "servicebus", "keyvault"). Subscription is an optional Azure
// subscription ID to auto-select once subscriptions are loaded.
type TabConfig struct {
	Kind         string `json:"kind"`
	Subscription string `json:"subscription,omitempty"`
}

// ActiveScheme returns the scheme matching ThemeName, or the first available.
func (c Config) ActiveScheme() Scheme {
	for _, s := range c.Schemes {
		if s.Name == c.ThemeName {
			return s
		}
	}
	if len(c.Schemes) > 0 {
		return c.Schemes[0]
	}
	return FallbackScheme()
}

// ActiveSchemeIndex returns the index of the active scheme.
func ActiveSchemeIndex(cfg Config) int {
	active := cfg.ActiveScheme()
	for i, s := range cfg.Schemes {
		if s.Name == active.Name {
			return i
		}
	}
	return 0
}

// parseSchemeFile converts a parsed YAML scheme file into a Scheme.
func parseSchemeFile(sf schemeFile) Scheme {
	return Scheme{
		Name:   sf.Name,
		Author: sf.Author,
		Base00: sf.Palette["base00"],
		Base01: sf.Palette["base01"],
		Base02: sf.Palette["base02"],
		Base03: sf.Palette["base03"],
		Base04: sf.Palette["base04"],
		Base05: sf.Palette["base05"],
		Base06: sf.Palette["base06"],
		Base07: sf.Palette["base07"],
		Base08: sf.Palette["base08"],
		Base09: sf.Palette["base09"],
		Base0A: sf.Palette["base0A"],
		Base0B: sf.Palette["base0B"],
		Base0C: sf.Palette["base0C"],
		Base0D: sf.Palette["base0D"],
		Base0E: sf.Palette["base0E"],
		Base0F: sf.Palette["base0F"],
	}
}

// fallbackScheme returns a minimal embedded scheme used only when no
// theme files exist at all (should never happen in practice since
// stock themes are auto-copied).
func FallbackScheme() Scheme {
	return Scheme{
		Name:   "fallback",
		Base00: "1e293b",
		Base01: "4B5563",
		Base02: "334155",
		Base03: "94A3B8",
		Base04: "94A3B8",
		Base05: "E5E7EB",
		Base06: "F8FAFC",
		Base07: "F8FAFC",
		Base08: "F87171",
		Base09: "F59E0B",
		Base0A: "F59E0B",
		Base0B: "22C55E",
		Base0C: "38BDF8",
		Base0D: "60A5FA",
		Base0E: "C084FC",
		Base0F: "94A3B8",
	}
}

// mergeSchemeDefaults fills any empty Base16 slot in target with the
// corresponding value from defaults.
func mergeSchemeDefaults(defaults, target *Scheme) {
	dv := reflect.ValueOf(defaults).Elem()
	tv := reflect.ValueOf(target).Elem()
	for i := 0; i < tv.NumField(); i++ {
		f := tv.Field(i)
		if f.Kind() == reflect.String && f.String() == "" {
			f.SetString(dv.Field(i).String())
		}
	}
}
