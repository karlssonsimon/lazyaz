package ui

import "charm.land/lipgloss/v2"

const AppHeaderHeight = 1

// ChromeStyles contains the application chrome styles. Built by NewStyles from a Base16 scheme.
type ChromeStyles struct {
	Header      lipgloss.Style
	Meta        lipgloss.Style
	Pane        lipgloss.Style
	FocusedPane lipgloss.Style
	Status      lipgloss.Style
	Help        lipgloss.Style
	Error       lipgloss.Style
	FilterHint  lipgloss.Style

	HeaderBrand       lipgloss.Style
	HeaderPath        lipgloss.Style
	HeaderPathMuted   lipgloss.Style
	HeaderMeta        lipgloss.Style
	HeaderStatusOK    lipgloss.Style // ● connected indicator
	HeaderStatusBad   lipgloss.Style // ○ disconnected indicator
	ColumnTitle       lipgloss.Style
	ColumnTitleFocus  lipgloss.Style
	ColumnRule        lipgloss.Style
	ColumnFooter      lipgloss.Style
	ColumnFooterFocus lipgloss.Style
	SelectionGutter   lipgloss.Style
	RowMeta           lipgloss.Style
	StatusMode        lipgloss.Style
	StatusKey         lipgloss.Style
	Loading           lipgloss.Style
}

type HeaderConfig struct {
	Brand string
	Path  []string
	Meta  string
}

type StatusAction struct {
	Key   string
	Label string
}

type StatusLineConfig struct {
	Mode    string
	Actions []StatusAction
	Message string // transient info or error; renders on the right when set
	IsError bool
}
