// Package keymap provides a unified, JSON-configurable keybinding system.
// Stock keymaps are embedded; users can override by placing custom JSON
// files in the config directory.
package keymap

import (
	"slices"
	"strings"

	"charm.land/bubbles/v2/key"
)

// Binding represents one or more key aliases for an action.
type Binding struct {
	Keys []string
}

// New creates a Binding from one or more key strings.
func New(keys ...string) Binding {
	return Binding{Keys: keys}
}

// Matches returns true if key matches any of the binding's keys.
func (b Binding) Matches(key string) bool {
	return slices.Contains(b.Keys, key)
}

// Label returns a human-readable representation of the binding
// (e.g. "ctrl+c/q"). The space key is displayed as "space".
func (b Binding) Label() string {
	if len(b.Keys) == 0 {
		return ""
	}
	seen := make(map[string]bool, len(b.Keys))
	labels := make([]string, 0, len(b.Keys))
	for _, key := range b.Keys {
		display := key
		if key == " " {
			display = "space"
		}
		if seen[display] {
			continue
		}
		seen[display] = true
		labels = append(labels, display)
	}
	return strings.Join(labels, "/")
}

// Short returns the first key only (for compact hint display).
func (b Binding) Short() string {
	if len(b.Keys) == 0 {
		return ""
	}
	if b.Keys[0] == " " {
		return "space"
	}
	return b.Keys[0]
}

// HelpEntry formats a binding and description as a help line.
func HelpEntry(b Binding, description string) string {
	return b.Label() + "  " + description
}

// AsBubbleKey returns a bubbles key.Binding bound to the same keys as
// this Binding. Used to override bubbles list/textinput keymaps so
// they follow the user-configured bindings.
func (b Binding) AsBubbleKey() key.Binding {
	return key.NewBinding(key.WithKeys(b.Keys...))
}
