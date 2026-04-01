package ui

import (
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/lipgloss"
)

type Palette struct {
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

func NewDefaultDelegate(p Palette) list.DefaultDelegate {
	delegate := list.NewDefaultDelegate()
	delegate.SetSpacing(0)
	delegate.Styles.NormalTitle = delegate.Styles.NormalTitle.Foreground(lipgloss.Color(p.Text))
	delegate.Styles.NormalDesc = delegate.Styles.NormalDesc.Foreground(lipgloss.Color(p.Muted))
	delegate.Styles.SelectedTitle = delegate.Styles.SelectedTitle.
		Foreground(lipgloss.Color(p.SelectedText)).
		Background(lipgloss.Color(p.SelectedBg)).
		Bold(true)
	delegate.Styles.SelectedDesc = delegate.Styles.SelectedDesc.
		Foreground(lipgloss.Color(p.SelectedText)).
		Background(lipgloss.Color(p.SelectedBg))
	delegate.Styles.FilterMatch = delegate.Styles.FilterMatch.Foreground(lipgloss.Color(p.FilterMatch)).Underline(true)
	return delegate
}

func StyleList(l *list.Model, p Palette) {
	// Build all styles from scratch so no default colors bleed through.
	l.Styles.TitleBar = lipgloss.NewStyle().
		Foreground(lipgloss.Color(p.Muted)).
		Padding(0, 1)
	l.Styles.Title = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(p.Accent)).
		Padding(0, 1)
	l.Styles.Spinner = lipgloss.NewStyle().
		Foreground(lipgloss.Color(p.AccentStrong))
	l.Styles.FilterPrompt = lipgloss.NewStyle().
		Foreground(lipgloss.Color(p.Accent))
	l.Styles.FilterCursor = lipgloss.NewStyle().
		Foreground(lipgloss.Color(p.AccentStrong))
	l.Styles.DefaultFilterCharacterMatch = lipgloss.NewStyle().
		Foreground(lipgloss.Color(p.FilterMatch)).
		Underline(true)
	l.Styles.StatusBar = lipgloss.NewStyle().
		Foreground(lipgloss.Color(p.Muted)).
		Padding(0, 0, 1, 2)
	l.Styles.StatusBarActiveFilter = lipgloss.NewStyle().
		Foreground(lipgloss.Color(p.Accent)).
		Bold(true)
	l.Styles.StatusBarFilterCount = lipgloss.NewStyle().
		Foreground(lipgloss.Color(p.AccentStrong)).
		Bold(true)
	l.Styles.NoItems = lipgloss.NewStyle().
		Foreground(lipgloss.Color(p.Muted))
	l.Styles.PaginationStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(p.Muted))
	l.Styles.HelpStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(p.Muted))
}

func ApplyThemeToLists(theme Theme, lists []*list.Model, spin *spinner.Model) (Palette, SyntaxStyles) {
	palette := theme.Colors
	styles := SyntaxStylesForTheme(theme)
	delegate := NewDefaultDelegate(palette)
	for _, l := range lists {
		l.SetDelegate(delegate)
		StyleList(l, palette)
	}
	spin.Style = lipgloss.NewStyle().Foreground(lipgloss.Color(palette.AccentStrong))
	return palette, styles
}

func ActiveThemeIndex(cfg Config) int {
	active := cfg.ActiveTheme()
	for i, t := range cfg.Themes {
		if t.Name == active.Name {
			return i
		}
	}
	return 0
}
