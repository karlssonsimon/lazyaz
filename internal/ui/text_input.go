package ui

import (
	"unicode"

	"charm.land/bubbles/v2/cursor"
)

// TextInput is a minimal cursor-aware text buffer used by every input
// surface in the app (form fields, search overlays, file browser filter).
// It owns its Value and Cursor and exposes a single HandleKey entry
// point covering the editing keys users expect — arrows, home/end,
// backspace, delete, ctrl+w, plus printable insertion.
//
// Cursor is a rune index (not a byte index) into Value, in the range
// [0, RuneCount(Value)]. Render-side code should split with
// SplitAtCursor and draw the cursor glyph between the two halves.
type TextInput struct {
	Value  string
	Cursor int
}

// HandleKey processes one keypress. Returns true if the key was
// consumed (movement, edit, printable insert). Returns false for keys
// the caller is responsible for (enter, tab, esc, custom shortcuts) so
// the caller can keep its own switch concise.
func (t *TextInput) HandleKey(key string) bool {
	switch key {
	case "left":
		if t.Cursor > 0 {
			t.Cursor--
		}
		return true
	case "right":
		if t.Cursor < t.runeLen() {
			t.Cursor++
		}
		return true
	case "home", "ctrl+a":
		t.Cursor = 0
		return true
	case "end", "ctrl+e":
		t.Cursor = t.runeLen()
		return true
	case "backspace":
		t.deleteBefore()
		return true
	case "delete":
		t.deleteAt()
		return true
	case "ctrl+w":
		t.deleteWordBefore()
		return true
	case "space":
		t.Insert(" ")
		return true
	}
	if isPrintableInputKey(key) {
		t.Insert(key)
		return true
	}
	return false
}

// Insert adds text at the current cursor and advances the cursor past
// it. Used for both single-char typing and clipboard paste.
func (t *TextInput) Insert(s string) {
	if s == "" {
		return
	}
	before, after := t.SplitAtCursor()
	t.Value = before + s + after
	t.Cursor += runeLen(s)
}

// Reset clears the buffer and cursor.
func (t *TextInput) Reset() {
	t.Value = ""
	t.Cursor = 0
}

// SetValue replaces the buffer and parks the cursor at the end. Useful
// when restoring state from outside (e.g. opening an edit form with a
// pre-filled value).
func (t *TextInput) SetValue(s string) {
	t.Value = s
	t.Cursor = runeLen(s)
}

// SplitAtCursor returns (before, after) so renderers can draw the
// cursor glyph between them. Clamps Cursor to the legal range — code
// that mutates Value directly (legacy callers) doesn't need to also
// fix Cursor before rendering.
func (t *TextInput) SplitAtCursor() (string, string) {
	rs := []rune(t.Value)
	c := t.Cursor
	if c < 0 {
		c = 0
	}
	if c > len(rs) {
		c = len(rs)
	}
	return string(rs[:c]), string(rs[c:])
}

// SplitWithCursor returns (before, at, after) where `at` is the single
// rune sitting under the cursor (or " " if the cursor is at the end)
// and `after` is the text strictly past that rune. Together they
// reconstruct the original value: before + at + after == Value.
//
// Use this when the cursor should *overlay* the character it points
// at (so blink-off shows the real character) rather than being drawn
// between characters. The standard pattern is:
//
//	before, at, after := ti.SplitWithCursor()
//	cur.SetChar(at)
//	out := before + cur.View() + after
func (t *TextInput) SplitWithCursor() (before, at, after string) {
	rs := []rune(t.Value)
	c := t.Cursor
	if c < 0 {
		c = 0
	}
	if c > len(rs) {
		c = len(rs)
	}
	before = string(rs[:c])
	if c >= len(rs) {
		return before, " ", ""
	}
	return before, string(rs[c]), string(rs[c+1:])
}

// PrepareCursor splits value at cursor and configures a copy of cur so
// its blink-off state shows the rune under the caret. Returns the
// three pieces ready to assemble: before + cursorView + after. cur is
// taken by value — the caller's cursor model is not mutated.
func PrepareCursor(value string, cursor int, cur cursor.Model) (before, cursorView, after string) {
	ti := TextInput{Value: value, Cursor: cursor}
	b, at, a := ti.SplitWithCursor()
	cur.SetChar(at)
	return b, cur.View(), a
}

func (t *TextInput) runeLen() int { return runeLen(t.Value) }

func (t *TextInput) deleteBefore() {
	if t.Cursor == 0 {
		return
	}
	rs := []rune(t.Value)
	t.Value = string(rs[:t.Cursor-1]) + string(rs[t.Cursor:])
	t.Cursor--
}

func (t *TextInput) deleteAt() {
	rs := []rune(t.Value)
	if t.Cursor >= len(rs) {
		return
	}
	t.Value = string(rs[:t.Cursor]) + string(rs[t.Cursor+1:])
}

// deleteWordBefore removes from the cursor backward through one run of
// whitespace + one run of non-whitespace, matching standard ctrl+w
// behavior in shells and editors.
func (t *TextInput) deleteWordBefore() {
	if t.Cursor == 0 {
		return
	}
	rs := []rune(t.Value)
	end := t.Cursor
	i := end
	for i > 0 && unicode.IsSpace(rs[i-1]) {
		i--
	}
	for i > 0 && !unicode.IsSpace(rs[i-1]) {
		i--
	}
	t.Value = string(rs[:i]) + string(rs[end:])
	t.Cursor = i
}

func runeLen(s string) int { return len([]rune(s)) }
