package ui

import "reflect"

type SyntaxColorConfig struct {
	Key         string `yaml:"key"`
	String      string `yaml:"string"`
	Number      string `yaml:"number"`
	Bool        string `yaml:"bool"`
	Null        string `yaml:"null"`
	Punctuation string `yaml:"punctuation"`
}

type Theme struct {
	Name         string       `yaml:"name"`
	SyntaxColorConfig SyntaxColorConfig `yaml:"json_colors"`
	Colors       Palette      `yaml:"ui_colors"`
}

type Config struct {
	AppName   string  `yaml:"-"`
	ThemeName string  `yaml:"theme"`
	Themes    []Theme `yaml:"-"`
}

func DefaultTheme() Theme {
	return Theme{
		Name: "default",
		SyntaxColorConfig: SyntaxColorConfig{
			Key:         "#C084FC",
			String:      "#4ADE80",
			Number:      "#F59E0B",
			Bool:        "#38BDF8",
			Null:        "#F87171",
			Punctuation: "#94A3B8",
		},
		Colors: Palette{
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

func (c Config) ActiveTheme() Theme {
	for _, t := range c.Themes {
		if t.Name == c.ThemeName {
			return t
		}
	}
	return DefaultTheme()
}

func SyntaxStylesForTheme(theme Theme) SyntaxStyles {
	return NewSyntaxStyles(SyntaxPalette{
		Key:         theme.SyntaxColorConfig.Key,
		String:      theme.SyntaxColorConfig.String,
		Number:      theme.SyntaxColorConfig.Number,
		Bool:        theme.SyntaxColorConfig.Bool,
		Null:        theme.SyntaxColorConfig.Null,
		Punctuation: theme.SyntaxColorConfig.Punctuation,
		XMLTag:      theme.Colors.Accent,
		XMLAttr:     theme.Colors.FilterMatch,
		CSVCellA:    theme.Colors.Text,
		CSVCellB:    theme.Colors.AccentStrong,
	})
}

func mergeStringFields(defaults, target any) {
	dv := reflect.ValueOf(defaults).Elem()
	tv := reflect.ValueOf(target).Elem()
	if dv.Type() != tv.Type() {
		return
	}
	for i := 0; i < tv.NumField(); i++ {
		f := tv.Field(i)
		if f.Kind() == reflect.String && f.String() == "" {
			f.SetString(dv.Field(i).String())
		}
	}
}
