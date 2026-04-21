package ui

import (
	"unicode"

	"charm.land/lipgloss/v2"
)

// TextInputAction is the user's response to a text-input overlay.
type TextInputAction int

const (
	TextInputActionNone TextInputAction = iota
	TextInputActionSubmit
	TextInputActionCancel
)

// TextInputResult is HandleKey's return: the action plus the current
// value (only meaningful for Submit). When Submit is attempted but the
// validator rejects the value, Action is TextInputActionNone and the
// validator's message is stored on the state for the renderer.
type TextInputResult struct {
	Action TextInputAction
	Value  string
}

// TextInputState holds the open/closed state and the current input.
// Validate is optional; when set it's called on each Submit attempt.
// Returning a non-empty string keeps the overlay open and shows the
// error under the input.
type TextInputState struct {
	Active      bool
	Title       string
	Placeholder string
	Value       string
	Error       string
	Validate    func(value string) string
}

// Open mounts the overlay. initial is the prefilled value (use "" for
// a blank prompt). validator is optional; pass nil to accept anything.
func (s *TextInputState) Open(title, placeholder, initial string, validator func(string) string) {
	s.Active = true
	s.Title = title
	s.Placeholder = placeholder
	s.Value = initial
	s.Error = ""
	s.Validate = validator
}

// Close clears the state. Idempotent.
func (s *TextInputState) Close() {
	*s = TextInputState{}
}

// HandleKey processes one key press. Returns Submit on enter (after
// successful validation) with the final value; Cancel on esc; None
// otherwise. Printable chars append; backspace pops the last rune.
func (s *TextInputState) HandleKey(key string) TextInputResult {
	if !s.Active {
		return TextInputResult{Action: TextInputActionNone}
	}
	switch key {
	case "esc":
		val := s.Value
		s.Close()
		return TextInputResult{Action: TextInputActionCancel, Value: val}
	case "enter":
		if s.Validate != nil {
			if msg := s.Validate(s.Value); msg != "" {
				s.Error = msg
				return TextInputResult{Action: TextInputActionNone, Value: s.Value}
			}
		}
		val := s.Value
		s.Close()
		return TextInputResult{Action: TextInputActionSubmit, Value: val}
	case "backspace":
		if s.Value != "" {
			rs := []rune(s.Value)
			s.Value = string(rs[:len(rs)-1])
			s.Error = ""
		}
		return TextInputResult{Action: TextInputActionNone, Value: s.Value}
	case "space":
		s.Value += " "
		s.Error = ""
		return TextInputResult{Action: TextInputActionNone, Value: s.Value}
	}
	if isPrintableInputKey(key) {
		s.Value += key
		s.Error = ""
	}
	return TextInputResult{Action: TextInputActionNone, Value: s.Value}
}

func isPrintableInputKey(key string) bool {
	if key == "" {
		return false
	}
	runes := []rune(key)
	if len(runes) != 1 {
		return false
	}
	return unicode.IsPrint(runes[0])
}

// RenderTextInputOverlay paints a compact centered prompt on top of base.
// cursorView is the rendered cursor glyph (typically m.Cursor.View()).
func RenderTextInputOverlay(state TextInputState, cursorView string, styles Styles, width, height int, base string) string {
	innerWidth := 56
	if innerWidth > width-6 {
		innerWidth = width - 6
	}
	if innerWidth < 20 {
		innerWidth = 20
	}

	if cursorView == "" {
		cursorView = "█"
	}

	var inputLine string
	if state.Value == "" {
		// No value yet → cursor at column 0, placeholder trailing muted
		// so the caret sits at the start of the insertion point.
		inputLine = cursorView + styles.Muted.Render(state.Placeholder)
	} else {
		inputLine = styles.Overlay.Input.Render(state.Value) + cursorView
	}

	rows := []string{
		styles.Overlay.Title.Render(state.Title),
		"",
		inputLine,
	}
	if state.Error != "" {
		rows = append(rows, "", styles.Warning.Render(state.Error))
	}
	rows = append(rows, "",
		styles.Accent2.Render("enter")+styles.Overlay.Hint.Render(" confirm  ·  ")+
			styles.Accent2.Render("esc")+styles.Overlay.Hint.Render(" cancel"))

	content := lipgloss.JoinVertical(lipgloss.Left, rows...)
	box := styles.Overlay.Box.Width(innerWidth).Render(content)
	return PlaceOverlay(width, height, box, base)
}
