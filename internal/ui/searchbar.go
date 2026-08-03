package ui

import (
	"charm.land/bubbles/v2/cursor"
	"github.com/karlssonsimon/lazyaz/internal/keymap"
)

// SearchDirection is which way a search runs. `n` repeats in this
// direction and `N` in the opposite one.
type SearchDirection int

const (
	SearchForward SearchDirection = iota
	SearchBackward
)

// Prompt is the glyph the search bar shows for this direction.
func (d SearchDirection) Prompt() string {
	if d == SearchBackward {
		return "?"
	}
	return "/"
}

// Opposite is the direction `N` repeats in.
func (d SearchDirection) Opposite() SearchDirection {
	if d == SearchBackward {
		return SearchForward
	}
	return SearchBackward
}

// SearchBar holds the / prompt and the query it produced.
//
// It is deliberately buffer-agnostic: the blob preview drives a streamed
// scan from it and the Service Bus message body drives an in-memory
// search, but neither difference reaches this type.
type SearchBar struct {
	// InputOpen is true while the user is typing a query.
	InputOpen bool
	Input     TextInput
	// Direction of the search being typed, and of the last one executed.
	Direction SearchDirection
	// Pattern is the last successfully executed query. Invalid until a
	// search has been accepted.
	Pattern SearchPattern
}

// Open starts a new query in the given direction.
func (s *SearchBar) Open(dir SearchDirection) {
	s.InputOpen = true
	s.Direction = dir
	s.Input.Reset()
}

// Close abandons the prompt, leaving any previously executed pattern in
// place so `n` still repeats the last search.
func (s *SearchBar) Close() {
	s.InputOpen = false
	s.Input.Reset()
}

// Accept compiles what has been typed and closes the prompt. A failure
// leaves the previous pattern untouched so a typo does not throw away a
// working search.
func (s *SearchBar) Accept() error {
	pattern, err := CompileSearchPattern(s.Input.Value)
	if err != nil {
		return err
	}
	s.Pattern = pattern
	s.InputOpen = false
	s.Input.Reset()
	return nil
}

// Active reports whether a pattern has been executed and can be
// repeated with n / N.
func (s SearchBar) Active() bool {
	return s.Pattern.Valid()
}

// Clear drops the executed pattern, so highlighting stops and n has
// nothing to repeat.
func (s *SearchBar) Clear() {
	s.Pattern = SearchPattern{}
	s.Close()
}

// HandleKey routes a keystroke to the open prompt. It reports whether
// the key was consumed and, separately, whether the query was submitted
// so the caller can run the search.
func (s *SearchBar) HandleKey(key string, km keymap.Keymap) (consumed, submitted bool) {
	if !s.InputOpen {
		return false, false
	}
	switch {
	case km.Cancel.Matches(key):
		s.Close()
		return true, false
	case km.OpenFocused.Matches(key) || key == "enter":
		return true, true
	}
	return s.Input.HandleKey(key), false
}

// Render draws the prompt line for the pane footer, matching the column
// filter's look but with the direction's glyph.
func (s SearchBar) Render(cur cursor.Model, styles Styles, width int) string {
	if s.InputOpen {
		before, cv, after := PrepareCursor(s.Input.Value, s.Input.Cursor, cur)
		return RenderPromptLine(s.Direction.Prompt(), before, cv, after, s.Input.Value, styles, width, true)
	}
	if s.Pattern.Valid() {
		return RenderPromptLine(s.Direction.Prompt(), "", "", "", s.Pattern.Query, styles, width, false)
	}
	return ""
}
