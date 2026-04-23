package ui

import (
	"charm.land/lipgloss/v2"
)

// FormAction is the user's response to a form overlay.
type FormAction int

const (
	FormActionNone FormAction = iota
	FormActionSubmit
	FormActionCancel
)

// FormResult is HandleKey's return. Values carries one entry per field,
// in the same order as FormOverlayState.Fields; it is only meaningful
// when Action == FormActionSubmit.
type FormResult struct {
	Action FormAction
	Values []string
}

// FormField describes one row in a FormOverlayState. Validate is
// optional; when set it is called on Submit and a non-empty return
// keeps the overlay open with the message stored on the field's Error.
type FormField struct {
	Label       string
	Placeholder string
	Value       string
	Error       string
	Validate    func(value string) string
}

// FormOverlayState holds a labeled multi-field prompt. Focus moves
// between fields with tab / shift-tab; printable keys and backspace
// edit the focused field. Submit validates every field together: if
// any validator returns non-empty the overlay stays open with each
// failing field showing its message.
type FormOverlayState struct {
	Active bool
	Title  string
	Fields []FormField
	Focus  int
}

// Open mounts the overlay with the given title and fields. Focus
// starts on the first field. Passing zero fields leaves the overlay
// inactive (a form with nothing to fill in is never meaningful).
func (s *FormOverlayState) Open(title string, fields []FormField) {
	if len(fields) == 0 {
		return
	}
	s.Active = true
	s.Title = title
	s.Fields = fields
	s.Focus = 0
}

// Close clears the state. Idempotent.
func (s *FormOverlayState) Close() {
	*s = FormOverlayState{}
}

// FocusedField returns a pointer to the field currently receiving
// input, or nil if the overlay is inactive / focus is out of range.
// Callers needing to append paste text to the focused field use this
// (HandleKey only accepts single-char keypresses).
func (s *FormOverlayState) FocusedField() *FormField {
	if !s.Active || s.Focus < 0 || s.Focus >= len(s.Fields) {
		return nil
	}
	return &s.Fields[s.Focus]
}

// HandleKey processes one key press. tab / shift-tab cycle focus.
// Enter validates all fields; if any fail, per-field errors are set
// and the action is None (overlay stays open). Esc cancels. Printable
// keys and backspace edit the focused field and clear its error.
func (s *FormOverlayState) HandleKey(key string) FormResult {
	if !s.Active {
		return FormResult{Action: FormActionNone}
	}
	switch key {
	case "esc":
		s.Close()
		return FormResult{Action: FormActionCancel}
	case "tab":
		s.Focus = (s.Focus + 1) % len(s.Fields)
		return FormResult{Action: FormActionNone}
	case "shift+tab":
		s.Focus = (s.Focus - 1 + len(s.Fields)) % len(s.Fields)
		return FormResult{Action: FormActionNone}
	case "enter":
		anyError := false
		for i := range s.Fields {
			f := &s.Fields[i]
			f.Error = ""
			if f.Validate == nil {
				continue
			}
			if msg := f.Validate(f.Value); msg != "" {
				f.Error = msg
				anyError = true
			}
		}
		if anyError {
			// Park focus on the first failing field so the user sees
			// the cursor where they need to fix something.
			for i := range s.Fields {
				if s.Fields[i].Error != "" {
					s.Focus = i
					break
				}
			}
			return FormResult{Action: FormActionNone}
		}
		values := make([]string, len(s.Fields))
		for i := range s.Fields {
			values[i] = s.Fields[i].Value
		}
		s.Close()
		return FormResult{Action: FormActionSubmit, Values: values}
	case "backspace":
		f := &s.Fields[s.Focus]
		if f.Value != "" {
			rs := []rune(f.Value)
			f.Value = string(rs[:len(rs)-1])
			f.Error = ""
		}
		return FormResult{Action: FormActionNone}
	case "space":
		f := &s.Fields[s.Focus]
		f.Value += " "
		f.Error = ""
		return FormResult{Action: FormActionNone}
	}
	if isPrintableInputKey(key) {
		f := &s.Fields[s.Focus]
		f.Value += key
		f.Error = ""
	}
	return FormResult{Action: FormActionNone}
}

// RenderFormOverlay paints a labeled multi-field prompt on top of base.
// cursorView is the rendered cursor glyph (typically m.Cursor.View()),
// shown on the focused field. Each field is a label line followed by an
// input line; per-field errors appear directly under the input.
func RenderFormOverlay(state FormOverlayState, cursorView string, styles Styles, width, height int, base string) string {
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

	rows := []string{styles.Overlay.Title.Render(state.Title), ""}

	for i, f := range state.Fields {
		rows = append(rows, styles.Overlay.SectionTitle.Render(f.Label))

		var inputLine string
		switch {
		case i == state.Focus && f.Value == "":
			inputLine = cursorView + styles.Muted.Render(f.Placeholder)
		case i == state.Focus:
			inputLine = styles.Overlay.Input.Render(f.Value) + cursorView
		case f.Value == "":
			inputLine = styles.Muted.Render(f.Placeholder)
		default:
			inputLine = styles.Overlay.Input.Render(f.Value)
		}
		rows = append(rows, inputLine)

		if f.Error != "" {
			rows = append(rows, styles.Warning.Render(f.Error))
		}

		// Blank row between fields, except after the last one.
		if i < len(state.Fields)-1 {
			rows = append(rows, "")
		}
	}

	rows = append(rows, "",
		styles.Accent2.Render("tab")+styles.Overlay.Hint.Render(" next field  ·  ")+
			styles.Accent2.Render("enter")+styles.Overlay.Hint.Render(" confirm  ·  ")+
			styles.Accent2.Render("esc")+styles.Overlay.Hint.Render(" cancel"))

	content := lipgloss.JoinVertical(lipgloss.Left, rows...)
	box := styles.Overlay.Box.Width(innerWidth).Render(content)
	return PlaceOverlay(width, height, box, base)
}
