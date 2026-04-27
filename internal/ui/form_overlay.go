package ui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/karlssonsimon/lazyaz/internal/keymap"
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

	// Section, when non-empty, opens a new labeled section starting at
	// this field. Use it to group fields visually (e.g. "REQUIRED").
	Section string
	// Help is rendered muted directly under the value (e.g.
	// "lowercase, digits, hyphens · 1–127 chars").
	Help string
	// Hint is rendered muted right-aligned on the field's row (e.g.
	// "ctrl+r reveal"). When MaxChars is set and Hint is empty, a
	// "<n> chars" counter is rendered automatically.
	Hint string
	// MaxChars triggers the auto char counter when Hint is empty.
	MaxChars int
	// Mask hides the value behind bullet characters until the form's
	// Reveal flag is toggled (ctrl+r). Used for secret values.
	Mask bool
}

// FormOverlayState holds a labeled multi-field prompt. Focus moves
// between fields with tab / shift-tab; printable keys and backspace
// edit the focused field. Submit validates every field together: if
// any validator returns non-empty the overlay stays open with each
// failing field showing its message.
type FormOverlayState struct {
	Active bool
	Title  string
	// Breadcrumb is rendered as ›-separated segments after the title
	// pill (e.g. ["sitsgroup", "kv-htg-stage-keyvault"]).
	Breadcrumb []string
	Fields     []FormField
	Focus      int
	// Reveal, when true, shows the raw value of Mask=true fields.
	// Toggled by ctrl+r on the form.
	Reveal bool
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
	s.Reveal = false
}

// OpenWithBreadcrumb is Open + sets the header breadcrumb in one call.
func (s *FormOverlayState) OpenWithBreadcrumb(title string, breadcrumb []string, fields []FormField) {
	s.Open(title, fields)
	if !s.Active {
		return
	}
	s.Breadcrumb = breadcrumb
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
// ctrl+r toggles the Reveal flag for any Mask=true fields.
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
	case "ctrl+r":
		s.Reveal = !s.Reveal
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

// formInnerWidth is the form's content width — wide enough for label +
// helper text + char counter without crowding.
const formInnerWidth = 96

// RenderFormOverlay paints a labeled multi-field prompt using the
// floating-screen pattern: header badge + breadcrumb + esc, sectioned
// fields with rose gutter on the focused row, helper text below each
// value, and a status-bar footer with INPUT mode pill + key hints.
//
// km may be nil — when set, footer hints reflect actual bindings.
func RenderFormOverlay(state FormOverlayState, cursorView string, styles Styles, km *keymap.Keymap, width, height int, base string) string {
	innerW := formInnerWidth
	boxW := innerW + 6 // padding(2+2) + border(1+1)
	if boxW > width-4 {
		boxW = width - 4
		innerW = boxW - 6
	}
	if innerW < 30 {
		innerW = 30
		boxW = innerW + 6
	}
	if cursorView == "" {
		cursorView = "█"
	}

	ov := styles.Overlay

	// --- Header ---
	rows := []string{renderFormHeader(state, styles, innerW)}
	rows = append(rows, ov.Rule.Render(strings.Repeat("─", innerW)))
	rows = append(rows, "")

	// --- Body: sections + fields ---
	labelW := formLabelWidth(state.Fields)

	for i, f := range state.Fields {
		if f.Section != "" {
			if i > 0 {
				rows = append(rows, "")
			}
			rows = append(rows, ov.HeaderCount.Render(strings.ToUpper(f.Section)))
			rows = append(rows, "")
		}

		focused := i == state.Focus
		fieldRows := renderFormField(f, focused, state.Reveal, cursorView, styles, innerW, labelW)
		rows = append(rows, fieldRows...)

		if i < len(state.Fields)-1 {
			rows = append(rows, "")
		}
	}

	// --- Footer ---
	rows = append(rows, "")
	rows = append(rows, ov.Rule.Render(strings.Repeat("─", innerW)))
	rows = append(rows, renderFormFooter(state, styles, km, innerW))

	content := lipgloss.JoinVertical(lipgloss.Left, rows...)
	box := ov.Box.Width(boxW).Render(content)
	return PlaceOverlay(width, height, box, base)
}

// renderFormHeader builds the row: BADGE › crumb › crumb        esc cancel.
// Long crumbs are truncated with `…` so the right-aligned close hint never
// gets pushed onto a wrap line.
func renderFormHeader(state FormOverlayState, styles Styles, innerW int) string {
	ov := styles.Overlay
	chevron := ov.Hint.Inline(true).Padding(0).Render(overlayChevron)
	right := ov.Hint.Inline(true).Padding(0).Render("esc cancel")

	badge := ov.HeaderBadge.Render(strings.ToUpper(state.Title))

	// Budget for crumbs: total width minus badge, right hint, gap, and one
	// chevron per crumb segment.
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

// renderFormFooter builds the row: [INPUT] tab next  ↑tab prev  ↵ confirm …
func renderFormFooter(state FormOverlayState, styles Styles, km *keymap.Keymap, innerW int) string {
	chrome := styles.Chrome
	ov := styles.Overlay

	parts := []string{chrome.StatusMode.Render("INPUT")}

	actions := []StatusAction{
		{Key: "tab", Label: "next"},
		{Key: "↑tab", Label: "prev"},
		{Key: "↵", Label: "confirm"},
		{Key: "esc", Label: "cancel"},
	}
	if formHasMaskedField(state) {
		actions = append(actions, StatusAction{Key: "ctrl+r", Label: revealLabel(state.Reveal)})
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

// renderFormField returns the rendered rows for a single field.
func renderFormField(f FormField, focused, reveal bool, cursorView string, styles Styles, innerW, labelW int) []string {
	ov := styles.Overlay

	// Row backgrounds — focused row uses selBg + rose gutter (matches
	// the overlay cursor row pattern). All inline spans render with the
	// row's background so child resets don't punch holes.
	bg := ov.Normal.GetBackground()
	if focused {
		bg = ov.Cursor.GetBackground()
	}
	baseStyle := lipgloss.NewStyle().Background(bg)
	muted := ov.RowHint.Background(bg)
	labelStyle := lipgloss.NewStyle().Background(bg).Bold(true)
	if !focused {
		labelStyle = labelStyle.Bold(false)
	}
	inputStyle := ov.Input.Background(bg)

	// Build the row.
	gutter := "  "
	if focused {
		gutter = styles.Warning.Background(ov.Normal.GetBackground()).Render("▍") + " "
	}

	labelText := labelStyle.Render(f.Label)
	labelPadded := labelText + baseStyle.Render(strings.Repeat(" ", max(0, labelW-lipgloss.Width(labelText))))

	displayValue := f.Value
	if f.Mask && !reveal {
		displayValue = strings.Repeat("•", len([]rune(f.Value)))
	}

	var valueRendered string
	switch {
	case focused && f.Value == "":
		valueRendered = cursorView + muted.Italic(true).Render(f.Placeholder)
	case focused:
		valueRendered = inputStyle.Render(displayValue) + cursorView
	case f.Value == "":
		valueRendered = muted.Italic(true).Render(f.Placeholder)
	default:
		valueRendered = inputStyle.Render(displayValue)
	}

	hint := f.Hint
	if hint == "" {
		hint = formAutoHint(f)
	}
	hintWidth := 0
	if hint != "" {
		hintWidth = lipgloss.Width(hint) + 2
	}

	// Value column width.
	valueCol := innerW - 2 /* gutter */ - labelW - hintWidth - 2 /* row padding */
	if valueCol < 10 {
		valueCol = 10
	}

	valuePad := valueCol - lipgloss.Width(valueRendered)
	if valuePad < 0 {
		valuePad = 0
	}
	valueLine := valueRendered + baseStyle.Render(strings.Repeat(" ", valuePad))

	row := gutter + labelPadded + baseStyle.Render("  ") + valueLine
	if hint != "" {
		row += baseStyle.Render("  ") + muted.Render(hint)
	}

	rows := []string{padRowToWidth(row, innerW, baseStyle)}

	// Helper / error / Blank under the value, indented to the value column.
	indent := "  " + strings.Repeat(" ", labelW+2)
	if f.Error != "" {
		rows = append(rows, padRowToWidth(indent+styles.Warning.Render(f.Error), innerW, baseStyle))
	} else if f.Help != "" {
		rows = append(rows, padRowToWidth(indent+muted.Render(f.Help), innerW, baseStyle))
	}

	return rows
}

// formAutoHint renders the auto char counter when MaxChars is set and
// no explicit hint was provided.
func formAutoHint(f FormField) string {
	if f.MaxChars <= 0 {
		return ""
	}
	chars := len([]rune(f.Value))
	return fmt.Sprintf("%d chars", chars)
}

// formLabelWidth picks a column width that fits every field's label.
// Min 8 so short labels still sit on a clean column.
func formLabelWidth(fields []FormField) int {
	w := 8
	for _, f := range fields {
		if lw := lipgloss.Width(f.Label); lw > w {
			w = lw
		}
	}
	return w
}

// formHasMaskedField returns true when at least one field uses Mask=true.
// Used to decide whether the footer's reveal action is shown.
func formHasMaskedField(state FormOverlayState) bool {
	for _, f := range state.Fields {
		if f.Mask {
			return true
		}
	}
	return false
}

func revealLabel(revealed bool) string {
	if revealed {
		return "hide"
	}
	return "reveal"
}

// padRowToWidth pads a styled row to the given width using baseStyle for
// the trailing spaces, so the row's background extends fully.
func padRowToWidth(row string, width int, baseStyle lipgloss.Style) string {
	pad := width - lipgloss.Width(row)
	if pad <= 0 {
		return row
	}
	return row + baseStyle.Render(strings.Repeat(" ", pad))
}
