package keymap

// Default returns the built-in keymap with all default bindings.
// This is vim-style navigation. Users who prefer standard arrow-key
// bindings can set keymap: standard in config.yaml.
func Default() Keymap {
	return Keymap{
		// Shared.
		Quit:                New("ctrl+c", "q"),
		Cancel:              New("esc", "ctrl+c"),
		HalfPageDown:        New("ctrl+d", "pgdown"),
		HalfPageUp:          New("ctrl+u", "pgup"),
		NextFocus:           New("tab"),
		PreviousFocus:       New("shift+tab"),
		ReloadSubscriptions: New("d"),
		RefreshScope:        New("r"),
		FilterInput:         New("/"),
		OpenFocused:         New("enter"),
		OpenFocusedAlt:      New("l", "right"),
		NavigateLeft:        New("h", "left"),
		BackspaceUp:         New("backspace"),
		SubscriptionPicker:  New("S"),
		ToggleThemePicker:   New("T"),
		ToggleHelp:          New("?", "f1"),
		ToggleNotifications: New("N"),
		ToggleStreams:       New("F"),

		// Overlay navigation.
		ThemeUp:     New("up", "ctrl+k"),
		ThemeDown:   New("down", "ctrl+j"),
		ThemeApply:  New("enter"),
		ThemeCancel: New("esc", "ctrl+c"),

		// Tabs.
		NewTab:         New("ctrl+t"),
		CloseTab:       New("ctrl+w"),
		NextTab:        New("L", "ctrl+right"),
		PrevTab:        New("H", "ctrl+left"),
		CommandPalette: New("ctrl+p"),
		Jump1:          New("alt+1"),
		Jump2:          New("alt+2"),
		Jump3:          New("alt+3"),
		Jump4:          New("alt+4"),
		Jump5:          New("alt+5"),
		Jump6:          New("alt+6"),
		Jump7:          New("alt+7"),
		Jump8:          New("alt+8"),
		Jump9:          New("alt+9"),

		// Blob.
		ToggleLoadAll:     New("a", "A"),
		ToggleMark:        New(" "),
		ToggleVisualLine:  New("v", "V"),
		ExitVisualLine:    New("esc"),
		VisualSwapAnchor:  New("o"),
		DownloadSelection: New("D"),
		SortBlobs:         New("s"),
		BlobVisualMove:    New("up", "down", "j", "k", "pgup", "pgdown", "home", "end", "g", "G"),

		// Blob preview.
		PreviewBack:          New("h", "left", "esc"),
		PreviewNextFocus:     New("tab"),
		PreviewPreviousFocus: New("shift+tab"),
		PreviewDown:          New("j", "down"),
		PreviewUp:            New("k", "up"),
		PreviewBottom:        New("G", "end"),
		PreviewTopPrefix:     New("g", "home"),

		// Service Bus.
		ShowActiveQueue:     New("["),
		ShowDeadLetterQueue: New("]"),
		RequeueDLQ:          New("R"),
		DeleteDuplicate:     New("D"),
		ToggleDLQFilter:     New("f"),
		MessageBack:         New("h", "left", "backspace", "esc"),

		// Key Vault.
		YankSecret: New("y"),

		// Shared.
		Inspect: New("K"),
	}
}
