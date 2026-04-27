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
	Breadcrumb   []string // optional context chain shown after the title pill
	Message      string
	ConfirmLabel string // verb shown in the hint row, e.g. "delete" or "overwrite"
	CancelLabel  string // verb shown in the hint row, e.g. "cancel"
	Destructive  bool   // when true, the badge + confirm key render in danger color
}

// Open mounts the modal with the given content. ConfirmLabel / CancelLabel
// are display strings used in the bottom hint row — pick verbs the user
// will recognize ("delete", "discard"), not generic "yes/no".
func (s *ConfirmModalState) Open(title, message, confirmLabel, cancelLabel string, destructive bool) {
	s.Active = true
	s.Title = title
	s.Breadcrumb = nil
	s.Message = message
	s.ConfirmLabel = confirmLabel
	s.CancelLabel = cancelLabel
	s.Destructive = destructive
}

// OpenWithBreadcrumb is Open + sets the header breadcrumb in one call.
func (s *ConfirmModalState) OpenWithBreadcrumb(title string, breadcrumb []string, message, confirmLabel, cancelLabel string, destructive bool) {
	s.Open(title, message, confirmLabel, cancelLabel, destructive)
	if !s.Active {
		return
	}
	s.Breadcrumb = breadcrumb
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

const confirmInnerWidth = 72

// RenderConfirmModal paints the prompt using the floating-screen pattern:
// BADGE + breadcrumb + esc header, message body, status-bar footer with
// the confirm/cancel keys. Destructive prompts render the badge and the
// `y` key in danger color.
func RenderConfirmModal(state ConfirmModalState, styles Styles, width, height int, base string) string {
	innerW := confirmInnerWidth
	boxW := innerW + 6
	if boxW > width-4 {
		boxW = width - 4
		innerW = boxW - 6
	}
	if innerW < 30 {
		innerW = 30
		boxW = innerW + 6
	}

	confirmLabel := state.ConfirmLabel
	if confirmLabel == "" {
		confirmLabel = "confirm"
	}
	cancelLabel := state.CancelLabel
	if cancelLabel == "" {
		cancelLabel = "cancel"
	}

	ov := styles.Overlay
	rows := []string{renderConfirmHeader(state, styles, innerW)}
	rows = append(rows, ov.Rule.Render(strings.Repeat("─", innerW)))
	if state.Message != "" {
		rows = append(rows, "")
		rows = append(rows, state.Message)
	}
	rows = append(rows, "")
	rows = append(rows, ov.Rule.Render(strings.Repeat("─", innerW)))
	rows = append(rows, renderConfirmFooter(state, confirmLabel, cancelLabel, styles, innerW))

	content := lipgloss.JoinVertical(lipgloss.Left, rows...)
	box := ov.Box.Width(boxW).Render(content)
	return PlaceOverlay(width, height, box, base)
}

func renderConfirmHeader(state ConfirmModalState, styles Styles, innerW int) string {
	ov := styles.Overlay
	chevron := ov.Hint.Inline(true).Padding(0).Render(overlayChevron)
	right := ov.Hint.Inline(true).Padding(0).Render("esc cancel")

	badge := ov.HeaderBadge.Render(strings.ToUpper(state.Title))
	if state.Destructive {
		// Mirror HeaderBadge layout (Padding 0,1 / Bold) but in danger color
		// so the title pill itself signals the destructive intent.
		badge = lipgloss.NewStyle().
			Foreground(ov.HeaderBadge.GetForeground()).
			Background(styles.Danger.GetForeground()).
			Bold(true).
			Padding(0, 1).
			Render(strings.ToUpper(state.Title))
	}

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

func renderConfirmFooter(state ConfirmModalState, confirmLabel, cancelLabel string, styles Styles, innerW int) string {
	chrome := styles.Chrome
	ov := styles.Overlay

	mode := "CONFIRM"
	modeStyle := chrome.StatusMode
	if state.Destructive {
		mode = strings.ToUpper(confirmLabel)
		modeStyle = lipgloss.NewStyle().
			Foreground(chrome.StatusMode.GetForeground()).
			Background(styles.Danger.GetForeground()).
			Bold(true).
			Padding(0, 1)
	}

	confirmKeyStyle := chrome.StatusKey
	if state.Destructive {
		confirmKeyStyle = styles.DangerBold.Background(chrome.StatusKey.GetBackground())
	}

	parts := []string{modeStyle.Render(mode)}
	parts = append(parts,
		confirmKeyStyle.Render("y")+ov.Hint.Inline(true).Padding(0).Render(" "+confirmLabel),
		chrome.StatusKey.Render("esc")+ov.Hint.Inline(true).Padding(0).Render(" "+cancelLabel),
	)
	left := strings.Join(parts, ov.Hint.Inline(true).Padding(0).Render("  "))
	return overlayJustifyRow(left, "", innerW, ov)
}
