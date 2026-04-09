package ui

import (
	"fmt"
	"strings"
	"time"

)

// StreamOverlayState is the open/closed + scroll state of the stream
// management overlay. Stream data is passed at render time.
type StreamOverlayState struct {
	Active    bool
	CursorIdx int
}

func (s *StreamOverlayState) Open() {
	s.Active = true
	s.CursorIdx = 0
}

func (s *StreamOverlayState) Close() {
	s.Active = false
	s.CursorIdx = 0
}

// StreamKeyBindings holds the bindings for the stream overlay.
// Cancel is the overlay dismiss key; CancelStream is the action key
// that cancels the currently selected fetch.
type StreamKeyBindings struct {
	Up, Down, Close, Cancel, CancelStream KeyMatcher
}

// HandleKey processes key presses for the overlay. Returns true if the
// user pressed the cancel-stream key on an active stream (the caller
// must perform the actual cancellation since the UI layer has no access
// to the broker).
func (s *StreamOverlayState) HandleKey(key string, bindings StreamKeyBindings, total int) (cancelIdx int, didCancel bool) {
	switch {
	case bindings.Close.Matches(key), bindings.Cancel != nil && bindings.Cancel.Matches(key):
		s.Close()
	case bindings.Up.Matches(key), key == "k":
		if s.CursorIdx > 0 {
			s.CursorIdx--
		}
	case bindings.Down.Matches(key), key == "j":
		if s.CursorIdx < total-1 {
			s.CursorIdx++
		}
	case bindings.CancelStream != nil && bindings.CancelStream.Matches(key):
		return s.CursorIdx, true
	}
	return 0, false
}

// StreamEntry is the renderer-facing view of a broker stream. The
// parent converts cache.StreamInfo → this at render time.
type StreamEntry struct {
	Key       string
	Status    string // "active", "done", "cancelled", "errored"
	Items     int
	Subs      int
	StartedAt time.Time
	EndedAt   time.Time
	Err       error
}

// RenderStreamOverlay paints the scrollable stream list on top of the
// given base view.
func RenderStreamOverlay(state StreamOverlayState, closeHint string, entries []StreamEntry, styles Styles, width, height int, base string) string {
	// Sort: active first, then most recent.
	active := make([]StreamEntry, 0, len(entries))
	finished := make([]StreamEntry, 0, len(entries))
	for _, e := range entries {
		if e.Status == "active" {
			active = append(active, e)
		} else {
			finished = append(finished, e)
		}
	}
	sorted := append(active, finished...)

	items := make([]OverlayItem, len(sorted))
	for i, e := range sorted {
		status := padRight(statusLabel(e.Status), 6)
		dur := formatDuration(e)
		countStr := fmt.Sprintf("%d items", e.Items)
		subsStr := fmt.Sprintf("%d subs", e.Subs)

		label := status + "  " + padRight(countStr, 12) + padRight(subsStr, 8) + padRight(dur, 10) + shortenKey(e.Key)
		desc := ""
		if e.Err != nil {
			desc = strings.ReplaceAll(e.Err.Error(), "\n", " ")
		}

		items[i] = OverlayItem{
			Label: label,
			Desc:  desc,
		}
	}

	activeCount := len(active)
	totalCount := len(sorted)
	title := fmt.Sprintf("Streams (%d active, %d total)", activeCount, totalCount)

	cfg := OverlayListConfig{
		Title:      title,
		CloseHint:  closeHint + "  x: cancel",
		InnerWidth: 100,
		MaxVisible: 20,
		Center:     true,
		HideSearch: true,
	}

	if len(items) == 0 {
		items = []OverlayItem{{Label: "No streams."}}
	}

	return RenderOverlayList(cfg, items, state.CursorIdx, styles.Overlay, width, height, base)
}

func statusLabel(status string) string {
	switch status {
	case "active":
		return "ACTIVE"
	case "done":
		return "DONE"
	case "cancelled":
		return "CANCEL"
	case "errored":
		return "ERROR"
	default:
		return status
	}
}

func formatDuration(e StreamEntry) string {
	end := e.EndedAt
	if end.IsZero() {
		end = time.Now()
	}
	d := end.Sub(e.StartedAt)
	switch {
	case d < time.Second:
		return fmt.Sprintf("%dms", d.Milliseconds())
	case d < time.Minute:
		return fmt.Sprintf("%.1fs", d.Seconds())
	default:
		totalSecs := int(d.Seconds())
		mins := totalSecs / 60
		secs := totalSecs % 60
		return fmt.Sprintf("%dm%ds", mins, secs)
	}
}

// shortenKey makes broker keys human-readable. Keys use null-byte
// separators; we replace them with " > " for display and trim long
// strings.
func shortenKey(key string) string {
	parts := strings.Split(key, "\x00")
	display := strings.Join(parts, " > ")
	if len(display) > 60 {
		display = display[:57] + "..."
	}
	return display
}
