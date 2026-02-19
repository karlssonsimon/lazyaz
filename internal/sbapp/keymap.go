package sbapp

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
	FilterInput         KeyBinding
	OpenFocused         KeyBinding
	OpenFocusedAlt      KeyBinding
	NavigateLeft        KeyBinding
	BackspaceUp         KeyBinding
	ToggleMark          KeyBinding
	ShowActiveQueue     KeyBinding
	ShowDeadLetterQueue KeyBinding
	RequeueDLQ          KeyBinding
	DeleteDuplicate     KeyBinding
	ToggleThemePicker   KeyBinding

	ThemeUp     KeyBinding
	ThemeDown   KeyBinding
	ThemeApply  KeyBinding
	ThemeCancel KeyBinding

	MessageBack KeyBinding
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
		ToggleMark:          NewKeyBinding(" "),
		ShowActiveQueue:     NewKeyBinding("["),
		ShowDeadLetterQueue: NewKeyBinding("]"),
		RequeueDLQ:          NewKeyBinding("R"),
		DeleteDuplicate:     NewKeyBinding("D"),
		ToggleThemePicker:   NewKeyBinding("T"),

		ThemeUp:     NewKeyBinding("up", "k"),
		ThemeDown:   NewKeyBinding("down", "j"),
		ThemeApply:  NewKeyBinding("enter"),
		ThemeCancel: NewKeyBinding("esc", "q"),

		MessageBack: NewKeyBinding("h", "left", "backspace", "esc"),
	}
}

func (k KeyMap) HelpText() string {
	return strings.Join([]string{
		"keys:",
		k.NextFocus.Label() + " focus",
		k.FilterInput.Label() + " filter",
		k.OpenFocused.Label() + "/" + k.OpenFocusedAlt.Label() + " open",
		k.NavigateLeft.Label() + " back",
		k.BackspaceUp.Label() + " up",
		k.ShowActiveQueue.Label() + "/" + k.ShowDeadLetterQueue.Label() + " active/dlq",
		k.ToggleMark.Label() + " mark",
		k.RequeueDLQ.Label() + " requeue(dlq)",
		k.DeleteDuplicate.Label() + " delete(dup)",
		k.HalfPageDown.Label() + "/" + k.HalfPageUp.Label() + " half-page",
		k.ToggleThemePicker.Label() + " theme",
		k.RefreshScope.Label() + " refresh",
		k.ReloadSubscriptions.Label() + " reload",
		k.Quit.Label() + " quit",
	}, " | ")
}
