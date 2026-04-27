package ui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// ConfirmAction is the user's response to a confirmation prompt.
type ConfirmAction int

const (
	ConfirmActionNone ConfirmAction = iota
	ConfirmActionConfirm
	ConfirmActionCancel
)

// ConfirmModalState holds open/closed state and the prompt's content.
// Zero value is closed; reopen by calling Open.
type ConfirmModalState struct {
	Active       bool
	Title        string
	Message      string
	ConfirmLabel string // verb shown in the hint row, e.g. "delete" or "overwrite"
	CancelLabel  string // verb shown in the hint row, e.g. "cancel"
	Destructive  bool   // when true, the confirm key is rendered in danger color
}

// Open mounts the modal with the given content. ConfirmLabel / CancelLabel
// are display strings used in the bottom hint row — pick verbs the user
// will recognize ("delete", "discard"), not generic "yes/no".
func (s *ConfirmModalState) Open(title, message, confirmLabel, cancelLabel string, destructive bool) {
	s.Active = true
	s.Title = title
	s.Message = message
	s.ConfirmLabel = confirmLabel
	s.CancelLabel = cancelLabel
	s.Destructive = destructive
}

// Close clears the state. Idempotent.
func (s *ConfirmModalState) Close() {
	*s = ConfirmModalState{}
}

// HandleKey processes one key press. Returns ConfirmActionConfirm on
// `y` / `enter`, ConfirmActionCancel on `n` / `esc`, ConfirmActionNone
// otherwise. The modal closes automatically on Confirm or Cancel.
func (s *ConfirmModalState) HandleKey(key string) ConfirmAction {
	if !s.Active {
		return ConfirmActionNone
	}
	switch key {
	case "y", "Y", "enter":
		s.Close()
		return ConfirmActionConfirm
	case "n", "N", "esc":
		s.Close()
		return ConfirmActionCancel
	}
	return ConfirmActionNone
}

// RenderConfirmModal paints a compact centered prompt on top of base.
// Box sizes to content — no empty padding rows. Width is fixed-ish
// (fits typical messages) and clamps to terminal width.
func RenderConfirmModal(state ConfirmModalState, styles Styles, width, height int, base string) string {
	innerWidth := 56
	if innerWidth > width-6 {
		innerWidth = width - 6
	}
	if innerWidth < 20 {
		innerWidth = 20
	}

	confirmLabel := state.ConfirmLabel
	if confirmLabel == "" {
		confirmLabel = "confirm"
	}
	cancelLabel := state.CancelLabel
	if cancelLabel == "" {
		cancelLabel = "cancel"
	}

	confirmKeyStyle := styles.Accent2
	if state.Destructive {
		confirmKeyStyle = styles.DangerBold
	}

	hint := confirmKeyStyle.Render("(y) "+confirmLabel) +
		styles.Overlay.Hint.Render("  ·  ") +
		styles.Accent2.Render("(esc) "+cancelLabel)

	title := styles.Overlay.Title.Render(state.Title)
	rule := styles.Overlay.Rule.Render(strings.Repeat("─", innerWidth))

	rows := []string{title, rule}
	if state.Message != "" {
		// Render message as plain text — styles.Overlay.Normal has
		// a left-padding on it (sized for item-row alignment with the
		// cursor marker) that lands on only the first wrapped line
		// if the box wraps the text, creating inconsistent indent.
		rows = append(rows, "", state.Message)
	}
	rows = append(rows, "", hint)

	content := lipgloss.JoinVertical(lipgloss.Left, rows...)
	box := styles.Overlay.Box.Width(innerWidth).Render(content)
	return PlaceOverlay(width, height, box, base)
}
