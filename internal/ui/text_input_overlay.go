package ui

import (
	"strings"
	"unicode"

	"charm.land/lipgloss/v2"
	"github.com/karlssonsimon/lazyaz/internal/keymap"
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
	Active     bool
	Title      string
	Breadcrumb []string // optional ›-separated context after the title pill
	// Placeholder shows in the value slot when Value is empty (italic muted).
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
	s.Breadcrumb = nil
	s.Placeholder = placeholder
	s.Value = initial
	s.Error = ""
	s.Validate = validator
}

// OpenWithBreadcrumb is Open + sets the header breadcrumb in one call.
func (s *TextInputState) OpenWithBreadcrumb(title string, breadcrumb []string, placeholder, initial string, validator func(string) string) {
	s.Open(title, placeholder, initial, validator)
	if !s.Active {
		return
	}
	s.Breadcrumb = breadcrumb
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

const textInputInnerWidth = 72

// RenderTextInputOverlay paints a single-field prompt using the
// floating-screen pattern: BADGE + breadcrumb + esc cancel header,
// input row with rose gutter, status-bar footer with INPUT pill.
//
// km may be nil — when set, footer hints reflect actual bindings.
func RenderTextInputOverlay(state TextInputState, cursorView string, styles Styles, km *keymap.Keymap, width, height int, base string) string {
	innerW := textInputInnerWidth
	boxW := innerW + 6
	if boxW > width-4 {
		boxW = width - 4
		innerW = boxW - 6
	}
	if innerW < 24 {
		innerW = 24
		boxW = innerW + 6
	}

	if cursorView == "" {
		cursorView = "█"
	}

	ov := styles.Overlay

	rows := []string{renderTextInputHeader(state, styles, innerW)}
	rows = append(rows, ov.Rule.Render(strings.Repeat("─", innerW)))
	rows = append(rows, "")
	rows = append(rows, renderTextInputBody(state, cursorView, styles, innerW))
	if state.Error != "" {
		rows = append(rows, padRowToWidth("  "+styles.Warning.Render(state.Error), innerW, lipgloss.NewStyle().Background(ov.Normal.GetBackground())))
	}
	rows = append(rows, "")
	rows = append(rows, ov.Rule.Render(strings.Repeat("─", innerW)))
	rows = append(rows, renderTextInputFooter(styles, km, innerW))

	content := lipgloss.JoinVertical(lipgloss.Left, rows...)
	box := ov.Box.Width(boxW).Render(content)
	return PlaceOverlay(width, height, box, base)
}

func renderTextInputHeader(state TextInputState, styles Styles, innerW int) string {
	ov := styles.Overlay
	chevron := ov.Hint.Inline(true).Padding(0).Render(overlayChevron)
	right := ov.Hint.Inline(true).Padding(0).Render("esc cancel")

	badge := ov.HeaderBadge.Render(strings.ToUpper(state.Title))

	budget := innerW - lipgloss.Width(badge) - lipgloss.Width(right) - 1
	for _, c := range state.Breadcrumb {
		if c != "" {
			budget -= lipgloss.Width(chevron)
		}
	}
	left := badge
	for _, crumb := range state.Breadcrumb {
		if crumb == "" {
			continue
		}
		display := truncateLabel(crumb, max(8, budget))
		budget -= lipgloss.Width(display)
		left = left + chevron + ov.Input.Render(display)
	}

	return overlayJustifyRow(left, right, innerW, ov)
}

// renderTextInputBody renders the cursor row with selBg highlight + rose
// gutter, matching the focused-field pattern from the form overlay.
func renderTextInputBody(state TextInputState, cursorView string, styles Styles, innerW int) string {
	ov := styles.Overlay
	bg := ov.Cursor.GetBackground()
	baseStyle := lipgloss.NewStyle().Background(bg)
	muted := ov.RowHint.Background(bg)
	inputStyle := ov.Input.Background(bg)

	gutter := styles.Warning.Background(ov.Normal.GetBackground()).Render("▍") + " "

	var value string
	if state.Value == "" {
		value = cursorView + muted.Italic(true).Render(state.Placeholder)
	} else {
		value = inputStyle.Render(state.Value) + cursorView
	}

	row := gutter + value
	pad := innerW - lipgloss.Width(row)
	if pad > 0 {
		row += baseStyle.Render(strings.Repeat(" ", pad))
	}
	return row
}

func renderTextInputFooter(styles Styles, km *keymap.Keymap, innerW int) string {
	chrome := styles.Chrome
	ov := styles.Overlay
	parts := []string{chrome.StatusMode.Render("INPUT")}
	actions := []StatusAction{
		{Key: "↵", Label: "confirm"},
		{Key: "esc", Label: "cancel"},
	}
	for _, a := range actions {
		label := a.Label
		if label != "" {
			label = " " + label
		}
		parts = append(parts, chrome.StatusKey.Render(a.Key)+ov.Hint.Inline(true).Padding(0).Render(label))
	}
	left := strings.Join(parts, ov.Hint.Inline(true).Padding(0).Render("  "))
	_ = km // reserved for future keymap-driven hints
	return overlayJustifyRow(left, "", innerW, ov)
}
