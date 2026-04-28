package ui

import (
	"fmt"
	icolor "image/color"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/karlssonsimon/lazyaz/internal/keymap"
)

const (
	notifInnerW     = 100
	notifMaxVisible = 16
	notifTimeColW   = 10
	notifLevelColW  = 11 // "✗ ERROR  " padded
)

// NotificationsOverlayState is the open/closed + scroll state of the
// notifications history overlay. The actual notification entries live
// on appshell.Notifier and are passed in at render time.
type NotificationsOverlayState struct {
	Active    bool
	CursorIdx int
}

func (s *NotificationsOverlayState) Open() {
	s.Active = true
	s.CursorIdx = 0
}

func (s *NotificationsOverlayState) Close() {
	s.Active = false
	s.CursorIdx = 0
}

// HandleKey processes key presses for the overlay. Up/Down move the
// cursor; close or cancel dismiss it.
func (s *NotificationsOverlayState) HandleKey(key string, bindings HelpKeyBindings, total int) {
	switch {
	case bindings.Close.Matches(key), bindings.Cancel != nil && bindings.Cancel.Matches(key):
		s.Close()
	case bindings.Up.Matches(key):
		if s.CursorIdx > 0 {
			s.CursorIdx--
		}
	case bindings.Down.Matches(key):
		if s.CursorIdx < total-1 {
			s.CursorIdx++
		}
	}
}

// NotificationEntry is the renderer-facing view of a logged
// notification. The parent converts appshell.Notification → this at
// render time so the ui package stays a leaf.
type NotificationEntry struct {
	Time    time.Time
	Level   ToastLevel
	Message string
}

// RenderNotificationsOverlay paints the scrollable history (newest first)
// as a tabular log: NOTIFICATIONS header pill, level counters, WHEN/LEVEL/
// EVENT columns, footer with LOG mode pill.
func RenderNotificationsOverlay(state NotificationsOverlayState, closeHint string, entries []NotificationEntry, styles Styles, _ *keymap.Keymap, width, height int, base string) string {
	innerW := notifInnerW
	boxW := innerW + 6
	if boxW > width-4 {
		boxW = width - 4
		innerW = boxW - 6
	}
	if innerW < 40 {
		innerW = 40
	}

	reversed := make([]NotificationEntry, len(entries))
	for i, e := range entries {
		reversed[len(entries)-1-i] = e
	}

	var nErr, nWarn, nOk, nInfo int
	for _, e := range reversed {
		switch e.Level {
		case ToastError:
			nErr++
		case ToastWarn:
			nWarn++
		case ToastSuccess:
			nOk++
		default:
			nInfo++
		}
	}

	ov := styles.Overlay
	rule := ov.Rule.Render(strings.Repeat("─", innerW))

	header := renderNotifHeader(closeHint, len(reversed), nErr, nWarn, nOk, nInfo, styles, innerW)
	colHeader := renderNotifColumnHeader(styles, innerW)
	bodyRows := renderNotifBody(reversed, state.CursorIdx, styles, innerW)
	footer := renderNotifFooter(styles, innerW)

	rows := []string{header, rule, colHeader}
	rows = append(rows, bodyRows...)
	rows = append(rows, rule, footer)

	content := lipgloss.JoinVertical(lipgloss.Left, rows...)
	box := ov.Box.Width(boxW).Render(content)
	return placeOverlayTop(width, height, box, base)
}

// renderNotifHeader builds the top row with the notifications badge,
// total breadcrumb, level counters, and right-aligned close hint.
func renderNotifHeader(closeHint string, total, nErr, nWarn, nOk, nInfo int, styles Styles, innerW int) string {
	ov := styles.Overlay
	dim := ov.Hint.Inline(true).Padding(0)
	chevron := dim.Render(overlayChevron)

	left := ov.HeaderBadge.Render("NOTIFICATIONS")
	if total > 0 {
		left += chevron + ov.Input.Render(formatNotifTotal(total))
	}

	if total > 0 {
		var counters []string
		if nErr > 0 {
			counters = append(counters, notifCounter(styles.Danger.GetForeground(), nErr, "error", styles))
		}
		if nWarn > 0 {
			counters = append(counters, notifCounter(styles.Warning.GetForeground(), nWarn, "warn", styles))
		}
		if nOk > 0 {
			counters = append(counters, notifCounter(styles.Ok.GetForeground(), nOk, "success", styles))
		}
		if nInfo > 0 {
			counters = append(counters, notifCounter(styles.Accent.GetForeground(), nInfo, "info", styles))
		}
		if len(counters) > 0 {
			left += dim.Render("  ") + strings.Join(counters, dim.Render(" · "))
		}
	}

	right := ""
	if closeHint != "" {
		right = dim.Render(closeHint)
	}
	return overlayJustifyRow(left, right, innerW, ov)
}

func formatNotifTotal(n int) string {
	if n == 1 {
		return "1 entry"
	}
	return formatInt(n) + " entries"
}

// notifCounter renders one "● N label" counter chip.
func notifCounter(col icolor.Color, n int, label string, styles Styles) string {
	ov := styles.Overlay
	bg := ov.Box.GetBackground()
	bullet := lipgloss.NewStyle().Foreground(col).Background(bg).Render("●")
	text := lipgloss.NewStyle().Foreground(styles.Muted.GetForeground()).Background(bg).
		Render(" " + formatInt(n) + " " + label)
	return bullet + text
}

// renderNotifColumnHeader builds the dim WHEN | LEVEL | EVENT row.
func renderNotifColumnHeader(styles Styles, innerW int) string {
	ov := styles.Overlay
	bg := ov.Normal.GetBackground()
	dim := lipgloss.NewStyle().Foreground(styles.Muted.GetForeground()).Background(bg)

	timeW := notifTimeColW
	levelW := notifLevelColW
	eventW := innerW - 2 - timeW - levelW
	if eventW < 10 {
		eventW = 10
	}

	cols := dim.Render(padRight("WHEN", timeW)) +
		dim.Render(padRight("LEVEL", levelW)) +
		dim.Render(padRight("EVENT", eventW))
	return ov.Normal.Width(innerW).Render(cols)
}

// renderNotifBody returns notifMaxVisible rows: the visible entries
// followed by empty filler so the box height stays constant.
func renderNotifBody(entries []NotificationEntry, cursor int, styles Styles, innerW int) []string {
	ov := styles.Overlay
	rows := make([]string, 0, notifMaxVisible)

	timeW := notifTimeColW
	levelW := notifLevelColW
	eventW := innerW - 2 - timeW - levelW
	if eventW < 10 {
		eventW = 10
	}

	if len(entries) == 0 {
		empty := ov.NoMatch.Render("No notifications yet.")
		rows = append(rows, ov.Normal.Width(innerW).Render(empty))
		for i := 1; i < notifMaxVisible; i++ {
			rows = append(rows, ov.Normal.Width(innerW).Render(""))
		}
		return rows
	}

	start, end := overlayScrollWindow(cursor, len(entries), notifMaxVisible)
	now := time.Now()
	for ci := start; ci < end; ci++ {
		rows = append(rows, renderNotifRow(entries[ci], ci == cursor, timeW, levelW, eventW, styles, innerW, now))
	}
	for i := len(rows); i < notifMaxVisible; i++ {
		rows = append(rows, ov.Normal.Width(innerW).Render(""))
	}
	return rows
}

// renderNotifRow renders one entry with bg-aware inline styling so the
// cursor row's selBg paints uniformly across columns.
func renderNotifRow(entry NotificationEntry, isCursor bool, timeW, levelW, eventW int, styles Styles, innerW int, now time.Time) string {
	ov := styles.Overlay

	var rowStyle lipgloss.Style
	var bg icolor.Color
	if isCursor {
		rowStyle = ov.Cursor.Width(innerW)
		bg = ov.Cursor.GetBackground()
	} else {
		rowStyle = ov.Normal.Width(innerW)
		bg = ov.Normal.GetBackground()
	}

	dimBg := lipgloss.NewStyle().Foreground(styles.Muted.GetForeground()).Background(bg)
	textBg := lipgloss.NewStyle().Foreground(rowStyle.GetForeground()).Background(bg)
	if isCursor {
		textBg = textBg.Bold(true)
	}

	when := relativeAge(now.Sub(entry.Time))
	whenCol := dimBg.Render(padRight(when, timeW))

	icon := levelIcon(entry.Level)
	word := levelLabel(entry.Level)
	levelText := icon + " " + word
	levelStyled := lipgloss.NewStyle().
		Foreground(levelColor(entry.Level, styles)).
		Background(bg).
		Bold(true).
		Render(levelText)
	levelCol := levelStyled
	if pad := levelW - lipgloss.Width(levelText); pad > 0 {
		levelCol += lipgloss.NewStyle().Background(bg).Render(strings.Repeat(" ", pad))
	}

	msg := strings.ReplaceAll(entry.Message, "\n", " ")
	msg = truncateLabel(msg, eventW)
	eventCol := textBg.Render(padRight(msg, eventW))

	return rowStyle.Render(whenCol + levelCol + eventCol)
}

// renderNotifFooter builds the bottom row with the LOG mode pill and
// the navigation hints we actually wire up.
func renderNotifFooter(styles Styles, innerW int) string {
	chrome := styles.Chrome
	ov := styles.Overlay
	dim := ov.Hint.Inline(true).Padding(0)

	mode := chrome.StatusMode.Render("LOG")
	move := chrome.StatusKey.Render("j/k") + dim.Render(" move")
	close := chrome.StatusKey.Render("esc") + dim.Render(" close")

	sep := dim.Render("  ")
	left := mode + sep + move + sep + close
	return overlayJustifyRow(left, "", innerW, ov)
}

func levelLabel(level ToastLevel) string {
	switch level {
	case ToastError:
		return "ERROR"
	case ToastWarn:
		return "WARN"
	case ToastSuccess:
		return "SUCCESS"
	default:
		return "INFO"
	}
}

// relativeAge formats a duration as a short relative time. Anything
// less than 5 seconds is "just now"; otherwise the largest unit fits.
func relativeAge(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	switch {
	case d < 5*time.Second:
		return "just now"
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}
