package kvapp

import (
	"charm.land/bubbles/v2/list"
)

// secretMarkKey extracts the mark/visual lookup key for ui.MarkDelegate
// from a secrets-list item.
func secretMarkKey(item list.Item) (string, bool) {
	s, ok := item.(secretItem)
	if !ok {
		return "", false
	}
	return s.secret.Name, true
}
