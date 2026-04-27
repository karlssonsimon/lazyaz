package ui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/karlssonsimon/lazyaz/internal/keymap"
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

// RenderNotificationsOverlay paints the scrollable history list
// (newest first) on top of the given base view. Use the passed
// styles to color level pills consistent with toasts.
func RenderNotificationsOverlay(state NotificationsOverlayState, closeHint string, entries []NotificationEntry, styles Styles, km *keymap.Keymap, width, height int, base string) string {
	// Reverse: newest first.
	reversed := make([]NotificationEntry, len(entries))
	for i, e := range entries {
		reversed[len(entries)-1-i] = e
	}

	// Pre-compute the longest level pill width so columns align.
	maxLevelW := 0
	for _, e := range reversed {
		w := lipgloss.Width(levelLabel(e.Level))
		if w > maxLevelW {
			maxLevelW = w
		}
	}
	// Time column is HH:MM:SS = 8.
	timeW := 8

	items := make([]OverlayItem, len(reversed))
	for i, e := range reversed {
		ts := e.Time.Format("15:04:05")
		pill := levelPillStyle(e.Level, styles).Render(padRight(levelLabel(e.Level), maxLevelW))
		// Squash newlines into spaces so each entry is a single line in
		// the overlay — full multi-line content can still be searched
		// later if we add a detail view.
		msg := strings.ReplaceAll(e.Message, "\n", " ")
		items[i] = OverlayItem{
			Label: padRight(ts, timeW) + "  " + pill + "  " + msg,
		}
	}

	cfg := OverlayListConfig{
		Title:      fmt.Sprintf("Notifications (%d)", len(entries)),
		CloseHint:  closeHint,
		InnerWidth: 100,
		MaxVisible: 20,
		Center:     true,
		HideSearch: true,
		Keymap:     km,
	}

	if len(items) == 0 {
		items = []OverlayItem{{Label: "No notifications yet."}}
	}

	return RenderOverlayList(cfg, items, state.CursorIdx, styles, width, height, base)
}

func levelLabel(level ToastLevel) string {
	switch level {
	case ToastError:
		return "ERROR"
	case ToastWarn:
		return "WARN"
	case ToastSuccess:
		return "OK"
	default:
		return "INFO"
	}
}

func levelPillStyle(level ToastLevel, styles Styles) lipgloss.Style {
	switch level {
	case ToastError:
		return styles.Danger.Bold(true)
	case ToastWarn:
		return styles.Warning.Bold(true)
	case ToastSuccess:
		return styles.FocusBorder.Bold(true)
	default:
		return styles.Accent
	}
}
