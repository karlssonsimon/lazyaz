package kvapp

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
	FilterInput         KeyBinding
	OpenFocused         KeyBinding
	OpenFocusedAlt      KeyBinding
	NavigateLeft        KeyBinding
	BackspaceUp         KeyBinding
	YankSecret          KeyBinding
	ToggleThemePicker   KeyBinding
	ToggleHelp          KeyBinding

	ThemeUp     KeyBinding
	ThemeDown   KeyBinding
	ThemeApply  KeyBinding
	ThemeCancel KeyBinding
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
		FilterInput:         NewKeyBinding("/"),
		OpenFocused:         NewKeyBinding("enter"),
		OpenFocusedAlt:      NewKeyBinding("l", "right"),
		NavigateLeft:        NewKeyBinding("h", "left"),
		BackspaceUp:         NewKeyBinding("backspace"),
		YankSecret:          NewKeyBinding("y"),
		ToggleThemePicker:   NewKeyBinding("T"),
		ToggleHelp:          NewKeyBinding("?"),

		ThemeUp:     NewKeyBinding("up", "k"),
		ThemeDown:   NewKeyBinding("down", "j"),
		ThemeApply:  NewKeyBinding("enter"),
		ThemeCancel: NewKeyBinding("esc", "q"),
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
				helpEntry(NewKeyBinding(k.OpenFocused.Label()+"/"+k.OpenFocusedAlt.Label()), "open selected item"),
				helpEntry(k.NavigateLeft, "go back"),
				helpEntry(k.BackspaceUp, "backspace navigation"),
				helpEntry(NewKeyBinding(k.HalfPageDown.Label()+"/"+k.HalfPageUp.Label()), "half-page scroll"),
			},
		},
		{
			Title: "Secrets",
			Items: []string{
				helpEntry(k.YankSecret, "yank secret value to clipboard"),
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
