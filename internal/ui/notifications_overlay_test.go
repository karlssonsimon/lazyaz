package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
)

type notifKey string

func (k notifKey) Matches(key string) bool { return string(k) == key }

func notifBindings() NotifKeyBindings {
	return NotifKeyBindings{
		Up: notifKey("k"), Down: notifKey("j"),
		Close: notifKey("N"), Cancel: notifKey("esc"),
		Yank: notifKey("y"),
	}
}

const longAzureError = "Failed to peek messages from orders: amqp: link detached, reason: *Error{Condition: amqp:not-allowed, Description: It is not possible for an entity that requires sessions to create a non-sessionful message receiver. TrackingId:0123456789abcdef_G12_B34, SystemTracker:sb-htg-prod:Queue:orders, Timestamp:2026-09-25T08:14:33, Info: map[]}"

// The list row truncates, but the detail pane carries the whole
// message, wrapped, so nothing about a long error is lost on screen.
func TestNotificationsOverlayShowsSelectedEntryInFull(t *testing.T) {
	entries := []NotificationEntry{
		{Time: time.Now(), Level: ToastInfo, Message: "older entry"},
		{Time: time.Now(), Level: ToastError, Message: longAzureError},
	}
	state := NotificationsOverlayState{Active: true}
	out := ansi.Strip(RenderNotificationsOverlay(state, "N close", "y", entries, NewStyles(FallbackScheme()), 120, 40, ""))

	// Every word of the error appears somewhere in the rendered box,
	// which the one-line list row alone cannot manage.
	for _, word := range strings.Fields(longAzureError) {
		if !strings.Contains(out, word) {
			t.Fatalf("detail pane lost %q from the selected error", word)
		}
	}
	if !strings.Contains(out, "y copy") {
		t.Error("footer does not advertise the copy key")
	}

	// Moving the cursor swaps the detail pane to the older entry.
	state.HandleKey("j", notifBindings(), len(entries))
	out = ansi.Strip(RenderNotificationsOverlay(state, "N close", "y", entries, NewStyles(FallbackScheme()), 120, 40, ""))
	if strings.Contains(out, "TrackingId") {
		t.Error("detail pane still shows the previous entry after moving the cursor")
	}
}

// A very long message is clamped to the pane and marked as clipped.
func TestNotificationsOverlayDetailClamps(t *testing.T) {
	msg := strings.Repeat("word ", 400)
	entries := []NotificationEntry{{Time: time.Now(), Level: ToastError, Message: msg}}
	rows := renderNotifDetail(entries, 0, NewStyles(FallbackScheme()), notifInnerW)
	if len(rows) != notifDetailLines {
		t.Fatalf("detail pane is %d rows, want %d", len(rows), notifDetailLines)
	}
	if !strings.Contains(ansi.Strip(rows[len(rows)-1]), "…") {
		t.Error("clipped message is not marked with an ellipsis")
	}
}

// The footer omits the copy hint when the keymap has no yank key.
func TestNotificationsOverlayNoYankHint(t *testing.T) {
	out := ansi.Strip(RenderNotificationsOverlay(NotificationsOverlayState{Active: true}, "N close", "", nil, NewStyles(FallbackScheme()), 120, 40, ""))
	if strings.Contains(out, "copy") {
		t.Error("copy hint shown although the keymap has no yank key")
	}
}

// y reports a yank only when there is something under the cursor.
func TestNotificationsOverlayYankKey(t *testing.T) {
	state := NotificationsOverlayState{Active: true}
	if state.HandleKey("y", notifBindings(), 0) {
		t.Error("yank reported with no entries")
	}
	if !state.HandleKey("y", notifBindings(), 3) {
		t.Error("yank not reported with entries present")
	}
	if !state.Active {
		t.Error("yank closed the overlay")
	}
	if state.HandleKey("esc", notifBindings(), 3) || state.Active {
		t.Error("esc should close without yanking")
	}
}
