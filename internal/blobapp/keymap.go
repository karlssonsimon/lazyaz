package blobapp

import "slices"

import "strings"

import "azure-storage/internal/ui"

type KeyBinding struct {
	Keys []string
}

func NewKeyBinding(keys ...string) KeyBinding {
	return KeyBinding{Keys: keys}
}

func (b KeyBinding) Matches(key string) bool {
	return slices.Contains(b.Keys, key)
}

func (b KeyBinding) Label() string {
	if len(b.Keys) == 0 {
		return ""
	}
	labels := make([]string, 0, len(b.Keys))
	for _, key := range b.Keys {
		if key == " " {
			labels = append(labels, "space")
			continue
		}
		labels = append(labels, key)
	}
	return strings.Join(labels, "/")
}

type KeyMap struct {
	Quit                KeyBinding
	HalfPageDown        KeyBinding
	HalfPageUp          KeyBinding
	NextFocus           KeyBinding
	PreviousFocus       KeyBinding
	ReloadSubscriptions KeyBinding
	RefreshScope        KeyBinding
	OpenFocused         KeyBinding
	OpenFocusedAlt      KeyBinding
	NavigateLeft        KeyBinding
	BackspaceUp         KeyBinding
	ToggleLoadAll       KeyBinding
	ToggleMark          KeyBinding
	ToggleVisualLine    KeyBinding
	ExitVisualLine      KeyBinding
	DownloadSelection   KeyBinding
	FilterInput         KeyBinding
	BlobVisualMove      KeyBinding
	ToggleThemePicker   KeyBinding
	ToggleHelp          KeyBinding

	ThemeUp     KeyBinding
	ThemeDown   KeyBinding
	ThemeApply  KeyBinding
	ThemeCancel KeyBinding

	PreviewBack          KeyBinding
	PreviewNextFocus     KeyBinding
	PreviewPreviousFocus KeyBinding
	PreviewDown          KeyBinding
	PreviewUp            KeyBinding
	PreviewBottom        KeyBinding
	PreviewTopPrefix     KeyBinding
}

func DefaultKeyMap() KeyMap {
	return KeyMap{
		Quit:                NewKeyBinding("ctrl+c", "q"),
		HalfPageDown:        NewKeyBinding("ctrl+d"),
		HalfPageUp:          NewKeyBinding("ctrl+u"),
		NextFocus:           NewKeyBinding("tab"),
		PreviousFocus:       NewKeyBinding("shift+tab"),
		ReloadSubscriptions: NewKeyBinding("d"),
		RefreshScope:        NewKeyBinding("r"),
		OpenFocused:         NewKeyBinding("enter"),
		OpenFocusedAlt:      NewKeyBinding("l", "right"),
		NavigateLeft:        NewKeyBinding("h", "left"),
		BackspaceUp:         NewKeyBinding("backspace"),
		ToggleLoadAll:       NewKeyBinding("a", "A"),
		ToggleMark:          NewKeyBinding(" "),
		ToggleVisualLine:    NewKeyBinding("v", "V"),
		ExitVisualLine:      NewKeyBinding("esc"),
		DownloadSelection:   NewKeyBinding("D"),
		FilterInput:         NewKeyBinding("/"),
		BlobVisualMove:      NewKeyBinding("up", "down", "j", "k", "pgup", "pgdown", "home", "end", "g", "G"),
		ToggleThemePicker:   NewKeyBinding("T"),
		ToggleHelp:          NewKeyBinding("?"),

		ThemeUp:     NewKeyBinding("up", "ctrl+k"),
		ThemeDown:   NewKeyBinding("down", "ctrl+j"),
		ThemeApply:  NewKeyBinding("enter"),
		ThemeCancel: NewKeyBinding("esc", "ctrl+c"),

		PreviewBack:          NewKeyBinding("h", "left", "esc"),
		PreviewNextFocus:     NewKeyBinding("tab"),
		PreviewPreviousFocus: NewKeyBinding("shift+tab"),
		PreviewDown:          NewKeyBinding("j", "down"),
		PreviewUp:            NewKeyBinding("k", "up"),
		PreviewBottom:        NewKeyBinding("G"),
		PreviewTopPrefix:     NewKeyBinding("g"),
	}
}

func (k KeyMap) FooterHelpText() string {
	return k.ToggleHelp.Label() + ": help"
}

func (k KeyMap) HelpSections() []ui.HelpSection {
	return []ui.HelpSection{
		{
			Title: "Navigation",
			Items: []string{
				helpEntry(k.NextFocus, "next focus"),
				helpEntry(k.PreviousFocus, "previous focus"),
				helpEntry(k.FilterInput, "filter focused pane"),
				helpEntry(NewKeyBinding(k.OpenFocused.Label()+"/"+k.OpenFocusedAlt.Label()), "open and move right"),
				helpEntry(k.NavigateLeft, "go left/back"),
				helpEntry(k.BackspaceUp, "up one folder"),
				helpEntry(NewKeyBinding(k.HalfPageDown.Label()+"/"+k.HalfPageUp.Label()), "half-page scroll"),
			},
		},
		{
			Title: "Blob Actions",
			Items: []string{
				helpEntry(k.ToggleLoadAll, "toggle load-all blobs"),
				helpEntry(k.ToggleMark, "toggle mark on current blob"),
				helpEntry(k.ToggleVisualLine, "start/end visual-line selection"),
				helpEntry(k.ExitVisualLine, "exit visual mode"),
				helpEntry(k.DownloadSelection, "download marked/visual selection"),
			},
		},
		{
			Title: "Preview",
			Items: []string{
				helpEntry(k.PreviewNextFocus, "next preview focus"),
				helpEntry(k.PreviewPreviousFocus, "previous preview focus"),
				helpEntry(NewKeyBinding(k.PreviewDown.Label()+"/"+k.PreviewUp.Label()), "scroll preview"),
				helpEntry(NewKeyBinding(k.HalfPageDown.Label()+"/"+k.HalfPageUp.Label()), "half-page preview scroll"),
				helpEntry(k.PreviewTopPrefix, "go to top with gg"),
				helpEntry(k.PreviewBottom, "go to bottom"),
				helpEntry(k.PreviewBack, "close preview / go back"),
			},
		},
		{
			Title: "App",
			Items: []string{
				helpEntry(k.ToggleThemePicker, "open theme picker"),
				helpEntry(k.RefreshScope, "refresh current scope"),
				helpEntry(k.ReloadSubscriptions, "reload subscriptions"),
				helpEntry(k.ToggleHelp, "toggle help"),
				helpEntry(k.Quit, "quit"),
			},
		},
	}
}

func helpEntry(binding KeyBinding, description string) string {
	return strings.Join([]string{binding.Label(), description}, "  ")
}
