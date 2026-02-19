package ui

import (
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/lipgloss"
)

type Palette struct {
	Border        string
	BorderFocused string
	Text          string
	Muted         string
	Accent        string
	AccentStrong  string
	Danger        string
	FilterMatch   string
	SelectedBg    string
	SelectedText  string
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
	l.Styles.TitleBar = l.Styles.TitleBar.
		Foreground(lipgloss.Color(p.Muted)).
		Padding(0, 1)
	l.Styles.Title = l.Styles.Title.
		Bold(true).
		Foreground(lipgloss.Color(p.Accent))
	l.Styles.Spinner = l.Styles.Spinner.Foreground(lipgloss.Color(p.AccentStrong))
	l.Styles.FilterPrompt = l.Styles.FilterPrompt.Foreground(lipgloss.Color(p.Accent))
	l.Styles.FilterCursor = l.Styles.FilterCursor.Foreground(lipgloss.Color(p.AccentStrong))
	l.Styles.DefaultFilterCharacterMatch = l.Styles.DefaultFilterCharacterMatch.Foreground(lipgloss.Color(p.FilterMatch)).Underline(true)
	l.Styles.StatusBar = l.Styles.StatusBar.
		Foreground(lipgloss.Color(p.Muted))
	l.Styles.StatusBarActiveFilter = l.Styles.StatusBarActiveFilter.Foreground(lipgloss.Color(p.Accent)).Bold(true)
	l.Styles.StatusBarFilterCount = l.Styles.StatusBarFilterCount.Foreground(lipgloss.Color(p.AccentStrong)).Bold(true)
	l.Styles.NoItems = l.Styles.NoItems.Foreground(lipgloss.Color(p.Muted))
	l.Styles.PaginationStyle = l.Styles.PaginationStyle.Foreground(lipgloss.Color(p.Muted))
	l.Styles.HelpStyle = l.Styles.HelpStyle.Foreground(lipgloss.Color(p.Muted))
}
