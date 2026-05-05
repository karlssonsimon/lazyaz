package ui

// Icons is the resolved icon set used for tab badges and other named
// glyph slots. Two pre-built sets exist: terminal-safe Unicode (the
// default) and Nerd Fonts. Selection is config-driven, not theme-
// driven, so icons live alongside Styles but aren't rebuilt on theme
// change.
//
// Nerd Fonts codepoints are taken from the Font Awesome (nf-fa) block,
// which has been stable across Nerd Fonts releases. Material Design
// (nf-md) glyphs are sometimes prettier but their codepoints have
// shifted between major versions, which would silently break for users
// on older patches.
type Icons struct {
	TabBlob       string
	TabServiceBus string
	TabKeyVault   string
	TabDashboard  string
}

// terminalIcons returns the default, terminal-safe glyph set. These
// render correctly in any monospace font; no patched font required.
func terminalIcons() Icons {
	return Icons{
		TabBlob:       "▲",
		TabServiceBus: "⇄",
		TabKeyVault:   "⌬",
		TabDashboard:  "▦",
	}
}

// nerdfontIcons returns the Nerd Fonts glyph set. Renders only when the
// terminal is configured with a patched Nerd Font.
func nerdfontIcons() Icons {
	return Icons{
		TabBlob:       "", // nf-fa-database
		TabServiceBus: "", // nf-fa-inbox
		TabKeyVault:   "", // nf-fa-key
		TabDashboard:  "", // nf-fa-dashboard
	}
}

// NewIcons returns the icon set selected by config.
func NewIcons(nerdfonts bool) Icons {
	if nerdfonts {
		return nerdfontIcons()
	}
	return terminalIcons()
}
