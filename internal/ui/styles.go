package ui

import (
	icolor "image/color"

	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/spinner"
	"charm.land/lipgloss/v2"
)

// Scheme represents a Base16 color scheme. The 16 colors follow the
// Tinted Theming specification: base00–base07 are shades (background
// through foreground), base08–base0F are accent hues.
type Scheme struct {
	Name   string `yaml:"name"`
	Author string `yaml:"author"`
	// Shades
	Base00 string `yaml:"base00"` // Default Background
	Base01 string `yaml:"base01"` // Lighter Background (status bars, pane borders)
	Base02 string `yaml:"base02"` // Selection Background
	Base03 string `yaml:"base03"` // Comments, Muted text
	Base04 string `yaml:"base04"` // Dark Foreground (status bars)
	Base05 string `yaml:"base05"` // Default Foreground
	Base06 string `yaml:"base06"` // Light Foreground (selected text)
	Base07 string `yaml:"base07"` // Bright Foreground (headers)
	// Accents
	Base08 string `yaml:"base08"` // Red — danger, errors
	Base09 string `yaml:"base09"` // Orange — numbers, constants
	Base0A string `yaml:"base0A"` // Yellow — filter match, warnings
	Base0B string `yaml:"base0B"` // Green — focused border, strings
	Base0C string `yaml:"base0C"` // Cyan — accent strong
	Base0D string `yaml:"base0D"` // Blue — accent, functions
	Base0E string `yaml:"base0E"` // Purple — keywords
	Base0F string `yaml:"base0F"` // Brown — punctuation
}

// Styles is the single resolved style collection built from a Scheme.
// Every render site uses fields from this struct instead of raw colors.
type Styles struct {
	Chrome         ChromeStyles
	Delegate       list.DefaultDelegate
	DelegateTwoRow list.DefaultDelegate // two-line variant (title + description)
	List           ListStyles
	Spinner        lipgloss.Style
	Syntax         SyntaxStyles
	Overlay        OverlayStyles
	TabBar         TabBarStyles

	// Bg is the Base00 background color for RenderCanvas.
	Bg icolor.Color

	// Semantic convenience styles for ad-hoc usage in views.
	Accent             lipgloss.Style // Bold accent (base0D)
	Accent2            lipgloss.Style // Bold secondary accent (base09 orange)
	Muted              lipgloss.Style // Muted text (base03)
	Danger             lipgloss.Style // Danger text (base08)
	DangerBold         lipgloss.Style // Bold danger (base08)
	Warning            lipgloss.Style // Warning/filter match (base0A)
	FocusBorder        lipgloss.Style // Focused border color (base0B)
	SelectionHighlight lipgloss.Style // Mouse text selection highlight (base02 bg + base06 fg)

	// StatusBar is the bottom status bar.
	StatusBar StatusBarStyles
}

// ListStyles contains all list.Model.Styles fields.
type ListStyles struct {
	TitleBar                    lipgloss.Style
	Title                       lipgloss.Style
	Spinner                     lipgloss.Style
	DefaultFilterCharacterMatch lipgloss.Style
	StatusBar                   lipgloss.Style
	StatusEmpty                 lipgloss.Style
	StatusBarActiveFilter       lipgloss.Style
	StatusBarFilterCount        lipgloss.Style
	NoItems                     lipgloss.Style
	PaginationStyle             lipgloss.Style
	HelpStyle                   lipgloss.Style
	ActivePaginationDot         lipgloss.Style
	InactivePaginationDot       lipgloss.Style
	ArabicPagination            lipgloss.Style
	DividerDot                  lipgloss.Style
}

// OverlayStyles covers theme picker, help overlay, command palette, tab picker.
type OverlayStyles struct {
	Title        lipgloss.Style
	SectionTitle lipgloss.Style
	Rule         lipgloss.Style // thin separator above/below header and footer rows
	DashedRule   lipgloss.Style // dashed divider after the active row
	HeaderBadge  lipgloss.Style // lavender pill: "THEMES", "COMMANDS", etc.
	HeaderCount  lipgloss.Style // muted "5 / 312" counter
	Prompt       lipgloss.Style
	Input        lipgloss.Style
	Normal       lipgloss.Style
	NormalFull   lipgloss.Style // normal with full width (for help body)
	Cursor       lipgloss.Style
	NoMatch      lipgloss.Style
	Hint         lipgloss.Style
	HintFull     lipgloss.Style // hint with full width
	RowHint      lipgloss.Style // muted inline hint for row content (no bg, so cursor row bg shows through)
	ActiveMarker lipgloss.Style // cyan • for the active/current row
	Match        lipgloss.Style // matched query chars in item labels
	BoxBg        icolor.Color   // background color for custom box construction
	Box          lipgloss.Style
}

// TabBarStyles covers tab bar rendering.
type TabBarStyles struct {
	Active   lipgloss.Style
	Inactive lipgloss.Style
	Sep      lipgloss.Style
	Bar      lipgloss.Style // full-width bar wrapper
}

// color converts a Base16 hex string to a color.Color, prepending "#".
func color(hex string) icolor.Color {
	if hex == "" {
		return lipgloss.Color("")
	}
	if hex[0] != '#' {
		return lipgloss.Color("#" + hex)
	}
	return lipgloss.Color(hex)
}

// colorRGB converts a Base16 hex string to a color.Color for Canvas rendering.
func colorRGB(hex string) icolor.Color {
	if hex == "" {
		return nil
	}
	if hex[0] != '#' {
		return lipgloss.Color("#" + hex)
	}
	return lipgloss.Color(hex)
}

// NewStyles resolves a Scheme into a complete Styles struct.
func NewStyles(s Scheme) Styles {
	bg := color(s.Base00)      // main background
	surface := color(s.Base01) // chrome/surface background + border foreground
	selBg := color(s.Base02)
	muted := color(s.Base03)
	statusFg := color(s.Base04)
	text := color(s.Base05)
	selText := color(s.Base06)
	danger := color(s.Base08)
	orange := color(s.Base09)
	warning := color(s.Base0A)
	green := color(s.Base0B)
	cyan := color(s.Base0C)
	blue := color(s.Base0D)
	purple := color(s.Base0E)
	brown := color(s.Base0F)

	// --- Chrome ---
	chrome := ChromeStyles{
		Header: lipgloss.NewStyle().
			Foreground(blue).
			Background(surface).
			Bold(true).
			Padding(0, 1),
		Meta: lipgloss.NewStyle().
			Foreground(muted).
			Background(surface).
			Padding(0, 1),
		Pane: lipgloss.NewStyle().
			Foreground(text).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(surface).
			Padding(0, 1),
		FocusedPane: lipgloss.NewStyle().
			Foreground(text).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(green).
			Padding(0, 1),
		Status: lipgloss.NewStyle().
			Foreground(text).
			Background(surface).
			Padding(0, 1),
		Help: lipgloss.NewStyle().
			Foreground(muted).
			Background(surface).
			Padding(0, 1),
		Error: lipgloss.NewStyle().
			Foreground(danger).
			Background(surface).
			Padding(0, 1),
		FilterHint: lipgloss.NewStyle().
			Foreground(blue).
			Background(surface).
			Padding(0, 1),
		HeaderBrand: lipgloss.NewStyle().
			Foreground(purple).
			Background(bg).
			Bold(true),
		HeaderPath: lipgloss.NewStyle().
			Foreground(text).
			Background(bg),
		HeaderPathMuted: lipgloss.NewStyle().
			Foreground(muted).
			Background(bg),
		HeaderMeta: lipgloss.NewStyle().
			Foreground(muted).
			Background(bg),
		HeaderStatusOK: lipgloss.NewStyle().
			Foreground(green).
			Background(bg).
			Bold(true),
		HeaderStatusBad: lipgloss.NewStyle().
			Foreground(danger).
			Background(bg).
			Bold(true),
		ColumnTitle: lipgloss.NewStyle().
			Foreground(purple).
			Background(bg).
			Bold(true),
		ColumnTitleFocus: lipgloss.NewStyle().
			Foreground(selText).
			Background(bg).
			Bold(true),
		ColumnRule: lipgloss.NewStyle().
			// Base03 (muted) reads as a real mid-gray on every base16
			// theme; Base01 (surface) is intentionally near-bg in dark
			// schemes, which made the rule glyphs nearly invisible.
			Foreground(muted).
			Background(bg),
		ColumnFooter: lipgloss.NewStyle().
			Foreground(muted).
			Background(bg),
		ColumnFooterFocus: lipgloss.NewStyle().
			Foreground(text).
			Background(bg),
		SelectionGutter: lipgloss.NewStyle().
			Foreground(warning).
			Background(bg).
			Bold(true),
		RowMeta: lipgloss.NewStyle().
			Foreground(muted).
			Background(bg),
		StatusMode: lipgloss.NewStyle().
			Foreground(bg).
			Background(warning).
			Bold(true).
			Padding(0, 1),
		StatusKey: lipgloss.NewStyle().
			Foreground(warning).
			Background(surface).
			Bold(true),
		Loading: lipgloss.NewStyle().
			Foreground(cyan).
			Background(bg),
	}

	// --- List Delegate ---
	// Delegate styles are transparent — RenderCanvas fills backgrounds
	// at the cell level after rendering.
	delegate := list.NewDefaultDelegate()
	delegate.SetHeight(1)
	delegate.SetSpacing(0)
	delegate.ShowDescription = false
	delegate.Styles.NormalTitle = lipgloss.NewStyle().
		Foreground(text).
		Padding(0, 0, 0, 2)
	delegate.Styles.NormalDesc = lipgloss.NewStyle().
		Foreground(muted).
		Padding(0, 0, 0, 2)
	delegate.Styles.SelectedTitle = lipgloss.NewStyle().
		Foreground(selText).
		Background(selBg).
		Bold(true).
		Border(lipgloss.ThickBorder(), false, false, false, true).
		BorderForeground(warning).
		BorderBackground(bg).
		Padding(0, 0, 0, 1)
	delegate.Styles.SelectedDesc = lipgloss.NewStyle().
		Foreground(selText).
		Background(selBg).
		Border(lipgloss.ThickBorder(), false, false, false, true).
		BorderForeground(warning).
		BorderBackground(bg).
		Padding(0, 0, 0, 1)
	delegate.Styles.DimmedTitle = lipgloss.NewStyle().
		Foreground(muted).
		Padding(0, 0, 0, 2)
	delegate.Styles.DimmedDesc = lipgloss.NewStyle().
		Foreground(muted).
		Padding(0, 0, 0, 2)
	delegate.Styles.FilterMatch = lipgloss.NewStyle().
		Foreground(warning).
		Underline(true)

	// Two-row delegate: inherits all styles but shows descriptions.
	delegateTwoRow := delegate
	delegateTwoRow.SetHeight(2)
	delegateTwoRow.SetSpacing(0)
	delegateTwoRow.ShowDescription = true

	// --- List Component Styles ---
	ls := ListStyles{
		TitleBar: lipgloss.NewStyle().
			Foreground(muted).
			Padding(0, 1),
		Title: lipgloss.NewStyle().
			Bold(true).
			Foreground(blue).
			Padding(0, 1),
		Spinner: lipgloss.NewStyle().
			Foreground(cyan),
		DefaultFilterCharacterMatch: lipgloss.NewStyle().
			Foreground(warning).
			Underline(true),
		StatusBar: lipgloss.NewStyle().
			Foreground(statusFg).
			Padding(0, 0, 1, 2),
		StatusEmpty: lipgloss.NewStyle().
			Foreground(muted),
		StatusBarActiveFilter: lipgloss.NewStyle().
			Foreground(blue).
			Bold(true),
		StatusBarFilterCount: lipgloss.NewStyle().
			Foreground(cyan).
			Bold(true),
		NoItems: lipgloss.NewStyle().
			Foreground(muted),
		PaginationStyle: lipgloss.NewStyle().
			Foreground(muted).
			PaddingLeft(2),
		HelpStyle: lipgloss.NewStyle().
			Foreground(muted).
			Padding(1, 0, 0, 2),
		ActivePaginationDot: lipgloss.NewStyle().
			Foreground(cyan).
			SetString("•"),
		InactivePaginationDot: lipgloss.NewStyle().
			Foreground(muted).
			SetString("•"),
		ArabicPagination: lipgloss.NewStyle().
			Foreground(muted),
		DividerDot: lipgloss.NewStyle().
			Foreground(muted).
			SetString(" • "),
	}

	// --- Spinner ---
	spinStyle := lipgloss.NewStyle().Foreground(cyan)

	// --- Syntax Highlighting ---
	syntax := NewSyntaxStyles(SyntaxPalette{
		Key:         "#" + s.Base0E,
		String:      "#" + s.Base0B,
		Number:      "#" + s.Base09,
		Bool:        "#" + s.Base0C,
		Punctuation: "#" + s.Base0F,
		XMLTag:      "#" + s.Base0D,
		XMLAttr:     "#" + s.Base0A,
	})

	// --- Overlay ---
	// Shares the redesign language: thin muted border (matches column
	// rules), purple-bold title (matches column titles), thick-left
	// gutter for selected rows (matches list selection delegate). No
	// rounded corners or accent-colored borders.
	overlay := OverlayStyles{
		Title: lipgloss.NewStyle().
			Bold(true).
			Foreground(purple).
			Background(bg),
		SectionTitle: lipgloss.NewStyle().
			Bold(true).
			Foreground(purple).
			Background(bg),
		Rule: lipgloss.NewStyle().
			Foreground(muted).
			Background(bg),
		DashedRule: lipgloss.NewStyle().
			Foreground(muted).
			Background(bg),
		HeaderBadge: lipgloss.NewStyle().
			Foreground(bg).
			Background(blue).
			Bold(true).
			Padding(0, 1),
		HeaderCount: lipgloss.NewStyle().
			Foreground(muted).
			Background(bg),
		Prompt: lipgloss.NewStyle().
			Foreground(muted).
			Background(bg),
		Input: lipgloss.NewStyle().
			Foreground(text).
			Background(bg),
		Normal: lipgloss.NewStyle().
			Foreground(text).
			Background(bg).
			Padding(0, 0, 0, 2),
		NormalFull: lipgloss.NewStyle().
			Foreground(text).
			Background(bg),
		Cursor: lipgloss.NewStyle().
			Foreground(selText).
			Background(selBg).
			Bold(true).
			Border(lipgloss.ThickBorder(), false, false, false, true).
			BorderForeground(warning).
			BorderBackground(bg).
			Padding(0, 0, 0, 1),
		NoMatch: lipgloss.NewStyle().
			Foreground(muted).
			Background(bg).
			Italic(true),
		Hint: lipgloss.NewStyle().
			Foreground(muted).
			Background(bg).
			Padding(0, 1),
		HintFull: lipgloss.NewStyle().
			Foreground(muted).
			Background(bg),
		RowHint: lipgloss.NewStyle().
			Foreground(muted),
		ActiveMarker: lipgloss.NewStyle().
			Foreground(cyan),
		Match: lipgloss.NewStyle().
			Foreground(warning).
			Bold(true),
		BoxBg: color(s.Base00),
		Box: lipgloss.NewStyle().
			Background(bg).
			Border(lipgloss.NormalBorder()).
			BorderForeground(muted).
			BorderBackground(bg).
			Padding(1, 2),
	}

	// --- Tab Bar ---
	// Active uses the accent color as a button background so it pops
	// clearly against the dimmer inactive tabs. Inactive tabs use muted
	// text on the surface background to recede.
	tabBar := TabBarStyles{
		Active: lipgloss.NewStyle().
			Bold(true).
			Foreground(bg).
			Background(blue).
			Padding(0, 1),
		Inactive: lipgloss.NewStyle().
			Foreground(muted).
			Background(surface).
			Padding(0, 1),
		Sep: lipgloss.NewStyle().
			Foreground(brown).
			Background(surface),
		Bar: lipgloss.NewStyle().
			Background(surface),
	}

	return Styles{
		Chrome:         chrome,
		Delegate:       delegate,
		DelegateTwoRow: delegateTwoRow,
		List:           ls,
		Spinner:        spinStyle,
		Syntax:         syntax,
		Overlay:        overlay,
		TabBar:         tabBar,

		Bg:                 colorRGB(s.Base00),
		Accent:             lipgloss.NewStyle().Bold(true).Foreground(blue),
		Accent2:            lipgloss.NewStyle().Bold(true).Foreground(orange),
		Muted:              lipgloss.NewStyle().Foreground(muted),
		Danger:             lipgloss.NewStyle().Foreground(danger),
		DangerBold:         lipgloss.NewStyle().Foreground(danger).Bold(true),
		Warning:            lipgloss.NewStyle().Foreground(warning),
		FocusBorder:        lipgloss.NewStyle().BorderForeground(green),
		SelectionHighlight: lipgloss.NewStyle().Foreground(selText).Background(selBg).Reverse(true),

		StatusBar: StatusBarStyles{
			Box: lipgloss.NewStyle().
				Background(surface).
				Padding(1, 1),
			Label: lipgloss.NewStyle().
				Foreground(text).
				Background(surface).
				Bold(true),
			Value: lipgloss.NewStyle().
				Foreground(muted).
				Background(surface),
			Error: lipgloss.NewStyle().
				Foreground(danger).
				Background(surface).
				Bold(true),
			Gap: lipgloss.NewStyle().
				Background(surface),
		},
	}
}

// ApplyToList applies the resolved styles to a single list.Model.
func (st Styles) ApplyToList(l *list.Model) {
	l.SetDelegate(st.Delegate)
	l.Styles.TitleBar = st.List.TitleBar
	l.Styles.Title = st.List.Title
	l.Styles.Spinner = st.List.Spinner
	l.Styles.DefaultFilterCharacterMatch = st.List.DefaultFilterCharacterMatch
	l.Styles.StatusBar = st.List.StatusBar
	l.Styles.StatusEmpty = st.List.StatusEmpty
	l.Styles.StatusBarActiveFilter = st.List.StatusBarActiveFilter
	l.Styles.StatusBarFilterCount = st.List.StatusBarFilterCount
	l.Styles.NoItems = st.List.NoItems
	l.Styles.PaginationStyle = st.List.PaginationStyle
	l.Styles.HelpStyle = st.List.HelpStyle
	l.Styles.ActivePaginationDot = st.List.ActivePaginationDot
	l.Styles.InactivePaginationDot = st.List.InactivePaginationDot
	l.Styles.ArabicPagination = st.List.ArabicPagination
	l.Styles.DividerDot = st.List.DividerDot
}

// ApplyToLists applies styles to multiple lists and a spinner.
func (st Styles) ApplyToLists(lists []*list.Model, spin *spinner.Model) {
	for _, l := range lists {
		st.ApplyToList(l)
	}
	spin.Style = st.Spinner
}
