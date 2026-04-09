package ui

import "github.com/atotto/clipboard"

// ReadClipboard returns the system clipboard content, or empty string
// on error. Used by overlay key handlers to support ctrl+v paste in
// text inputs that don't use a bubbles textinput component.
func ReadClipboard() string {
	text, err := clipboard.ReadAll()
	if err != nil {
		return ""
	}
	return text
}
