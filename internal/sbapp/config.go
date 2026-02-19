package sbapp

import (
	"os"
	"path/filepath"
	"reflect"

	"github.com/charmbracelet/lipgloss"
	"gopkg.in/yaml.v3"
)

type JSONColors struct {
	Key         string `yaml:"key"`
	String      string `yaml:"string"`
	Number      string `yaml:"number"`
	Bool        string `yaml:"bool"`
	Null        string `yaml:"null"`
	Punctuation string `yaml:"punctuation"`
}

type UIColors struct {
	Border        string `yaml:"border"`
	BorderFocused string `yaml:"border_focused"`
	Text          string `yaml:"text"`
	Muted         string `yaml:"muted"`
	Accent        string `yaml:"accent"`
	AccentStrong  string `yaml:"accent_strong"`
	Danger        string `yaml:"danger"`
	FilterMatch   string `yaml:"filter_match"`
	SelectedBg    string `yaml:"selected_bg"`
	SelectedText  string `yaml:"selected_text"`
}

type Config struct {
	JSONColors JSONColors `yaml:"json_colors"`
	UIColors   UIColors   `yaml:"ui_colors"`
}

func defaultConfig() Config {
	return Config{
		JSONColors: JSONColors{
			Key:         "#C084FC",
			String:      "#4ADE80",
			Number:      "#F59E0B",
			Bool:        "#38BDF8",
			Null:        "#F87171",
			Punctuation: "#94A3B8",
		},
		UIColors: UIColors{
			Border:        "#4B5563",
			BorderFocused: "#22C55E",
			Text:          "#E5E7EB",
			Muted:         "#94A3B8",
			Accent:        "#60A5FA",
			AccentStrong:  "#38BDF8",
			Danger:        "#F87171",
			FilterMatch:   "#F59E0B",
			SelectedBg:    "#334155",
			SelectedText:  "#F8FAFC",
		},
	}
}

func configDir() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "azsb")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "azsb")
}

func LoadConfig() Config {
	dir := configDir()
	if dir == "" {
		return defaultConfig()
	}
	return loadConfigFromDir(dir)
}

func loadConfigFromDir(dir string) Config {
	cfg := defaultConfig()

	data, err := os.ReadFile(filepath.Join(dir, "config.yaml"))
	if err != nil {
		return cfg
	}

	var fileCfg Config
	if err := yaml.Unmarshal(data, &fileCfg); err != nil {
		return cfg
	}

	mergeStringFields(&cfg.JSONColors, &fileCfg.JSONColors)
	mergeStringFields(&cfg.UIColors, &fileCfg.UIColors)
	return fileCfg
}

func mergeStringFields(defaults, target any) {
	dv := reflect.ValueOf(defaults).Elem()
	tv := reflect.ValueOf(target).Elem()
	for i := 0; i < tv.NumField(); i++ {
		f := tv.Field(i)
		if f.Kind() == reflect.String && f.String() == "" {
			f.SetString(dv.Field(i).String())
		}
	}
}

type jsonStyles struct {
	key    lipgloss.Style
	str    lipgloss.Style
	number lipgloss.Style
	bool   lipgloss.Style
	null   lipgloss.Style
	punct  lipgloss.Style
}

func (c JSONColors) styles() jsonStyles {
	return jsonStyles{
		key:    lipgloss.NewStyle().Foreground(lipgloss.Color(c.Key)),
		str:    lipgloss.NewStyle().Foreground(lipgloss.Color(c.String)),
		number: lipgloss.NewStyle().Foreground(lipgloss.Color(c.Number)),
		bool:   lipgloss.NewStyle().Foreground(lipgloss.Color(c.Bool)),
		null:   lipgloss.NewStyle().Foreground(lipgloss.Color(c.Null)),
		punct:  lipgloss.NewStyle().Foreground(lipgloss.Color(c.Punctuation)),
	}
}
