package app

import (
	"slices"
	"strings"

	"azure-storage/internal/ui"
)

type keyBinding struct {
	Keys []string
}

func newKeyBinding(keys ...string) keyBinding {
	return keyBinding{Keys: keys}
}

func (b keyBinding) Matches(key string) bool {
	return slices.Contains(b.Keys, key)
}

func (b keyBinding) Label() string {
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

type tabKeyMap struct {
	NewTab    keyBinding
	CloseTab  keyBinding
	NextTab   keyBinding
	PrevTab   keyBinding
	Jump1     keyBinding
	Jump2     keyBinding
	Jump3     keyBinding
	Jump4     keyBinding
	Jump5     keyBinding
	Jump6     keyBinding
	Jump7     keyBinding
	Jump8     keyBinding
	Jump9     keyBinding
	Quit      keyBinding
	ThemePick keyBinding
	ToggleHelp keyBinding

	ThemeUp     keyBinding
	ThemeDown   keyBinding
	ThemeApply  keyBinding
	ThemeCancel keyBinding
}

func defaultTabKeyMap() tabKeyMap {
	return tabKeyMap{
		NewTab:    newKeyBinding("ctrl+t"),
		CloseTab:  newKeyBinding("ctrl+w"),
		NextTab:   newKeyBinding("L"),
		PrevTab:   newKeyBinding("H"),
		Jump1:     newKeyBinding("alt+1"),
		Jump2:     newKeyBinding("alt+2"),
		Jump3:     newKeyBinding("alt+3"),
		Jump4:     newKeyBinding("alt+4"),
		Jump5:     newKeyBinding("alt+5"),
		Jump6:     newKeyBinding("alt+6"),
		Jump7:     newKeyBinding("alt+7"),
		Jump8:     newKeyBinding("alt+8"),
		Jump9:     newKeyBinding("alt+9"),
		Quit:      newKeyBinding("ctrl+c", "q"),
		ThemePick: newKeyBinding("T"),
		ToggleHelp: newKeyBinding("?"),

		ThemeUp:     newKeyBinding("up", "k"),
		ThemeDown:   newKeyBinding("down", "j"),
		ThemeApply:  newKeyBinding("enter"),
		ThemeCancel: newKeyBinding("esc", "q"),
	}
}

func (k tabKeyMap) jumpIndex(key string) (int, bool) {
	jumps := []keyBinding{k.Jump1, k.Jump2, k.Jump3, k.Jump4, k.Jump5, k.Jump6, k.Jump7, k.Jump8, k.Jump9}
	for i, b := range jumps {
		if b.Matches(key) {
			return i, true
		}
	}
	return 0, false
}

func (k tabKeyMap) helpSections(childSections []ui.HelpSection) []ui.HelpSection {
	tabSection := ui.HelpSection{
		Title: "Tabs",
		Items: []string{
			k.NewTab.Label() + "  new tab",
			k.CloseTab.Label() + "  close tab",
			k.PrevTab.Label() + "  prev tab",
			k.NextTab.Label() + "  next tab",
			"alt+1..9  jump to tab",
			k.ThemePick.Label() + "  theme picker",
			k.ToggleHelp.Label() + "  help",
			k.Quit.Label() + "  quit",
		},
	}
	return append([]ui.HelpSection{tabSection}, childSections...)
}
