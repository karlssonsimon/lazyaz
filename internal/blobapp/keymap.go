package blobapp

import "slices"

import "strings"

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

		ThemeUp:     NewKeyBinding("up", "k"),
		ThemeDown:   NewKeyBinding("down", "j"),
		ThemeApply:  NewKeyBinding("enter"),
		ThemeCancel: NewKeyBinding("esc", "q"),

		PreviewBack:          NewKeyBinding("h", "left", "esc"),
		PreviewNextFocus:     NewKeyBinding("tab"),
		PreviewPreviousFocus: NewKeyBinding("shift+tab"),
		PreviewDown:          NewKeyBinding("j", "down"),
		PreviewUp:            NewKeyBinding("k", "up"),
		PreviewBottom:        NewKeyBinding("G"),
		PreviewTopPrefix:     NewKeyBinding("g"),
	}
}

func (k KeyMap) HelpText() string {
	return strings.Join([]string{
		"keys:",
		k.NextFocus.Label() + " focus",
		k.FilterInput.Label() + " filter pane",
		k.OpenFocused.Label() + "/" + k.OpenFocusedAlt.Label() + " open->focus right",
		k.NavigateLeft.Label() + " left/up",
		k.ToggleLoadAll.Label() + " toggle load-all blobs",
		k.ToggleMark.Label() + " toggle mark",
		k.ToggleVisualLine.Label() + " visual-line range",
		k.DownloadSelection.Label() + " download selection",
		"preview: " + k.PreviewUp.Label() + " " + k.HalfPageDown.Label() + "/" + k.HalfPageUp.Label() + " gg " + k.PreviewBottom.Label() + " " + k.PreviewBack.Label(),
		k.BackspaceUp.Label() + " up folder",
		k.ToggleThemePicker.Label() + " theme",
		k.RefreshScope.Label() + " refresh scope",
		k.ReloadSubscriptions.Label() + " reload subscriptions",
		k.Quit.Label() + " quit",
	}, " | ")
}
