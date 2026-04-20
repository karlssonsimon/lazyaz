package ui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/karlssonsimon/lazyaz/internal/activity"
)

// ActivityPane enumerates which pane the activity overlay is displaying.
type ActivityPane int

const (
	ActivityListPane ActivityPane = iota
	ActivityDetailPane
)

// ActivityAction is returned by HandleKey so the caller can perform
// side-effects the UI package can't do alone (cancel an activity,
// close the overlay).
type ActivityAction int

const (
	ActivityActionNone ActivityAction = iota
	ActivityActionClose
	ActivityActionBack
	ActivityActionDrill
	ActivityActionCancel
)

// ActivityResult is the handler's result. Action is always set; TargetID is
// populated for Cancel/Drill so the caller knows which activity.
type ActivityResult struct {
	Action   ActivityAction
	TargetID string
}

// ActivityOverlayState holds the open/closed state and the currently-focused
// activity ID. VisibleIDs is set by the render caller each frame so the
// state machine can move the cursor through the sorted list. The cursor
// lives as an ID (not an index) so it stays stable across sort shuffles.
type ActivityOverlayState struct {
	Active     bool
	View       ActivityPane
	FocusedID  string
	VisibleIDs []string
}

func (s *ActivityOverlayState) Open() {
	s.Active = true
	s.View = ActivityListPane
}

func (s *ActivityOverlayState) OpenDetail(id string) {
	s.Active = true
	s.View = ActivityDetailPane
	s.FocusedID = id
}

func (s *ActivityOverlayState) Close() {
	s.Active = false
	s.View = ActivityListPane
	s.FocusedID = ""
}

// HandleKey mutates the state based on key and returns an action
// describing what the caller should do next.
func (s *ActivityOverlayState) HandleKey(key string) ActivityResult {
	if s.View == ActivityDetailPane {
		switch key {
		case "esc", "h", "left":
			s.View = ActivityListPane
			return ActivityResult{Action: ActivityActionBack}
		case "x":
			return ActivityResult{Action: ActivityActionCancel, TargetID: s.FocusedID}
		}
		return ActivityResult{Action: ActivityActionNone}
	}
	switch key {
	case "esc":
		s.Close()
		return ActivityResult{Action: ActivityActionClose}
	case "j", "down":
		s.moveCursor(1)
	case "k", "up":
		s.moveCursor(-1)
	case "enter", "l", "right":
		if s.FocusedID != "" {
			s.View = ActivityDetailPane
			return ActivityResult{Action: ActivityActionDrill, TargetID: s.FocusedID}
		}
	case "x":
		if s.FocusedID != "" {
			return ActivityResult{Action: ActivityActionCancel, TargetID: s.FocusedID}
		}
	}
	return ActivityResult{Action: ActivityActionNone}
}

func (s *ActivityOverlayState) moveCursor(delta int) {
	if len(s.VisibleIDs) == 0 {
		s.FocusedID = ""
		return
	}
	idx := -1
	for i, id := range s.VisibleIDs {
		if id == s.FocusedID {
			idx = i
			break
		}
	}
	if idx < 0 {
		idx = 0
	}
	idx += delta
	if idx < 0 {
		idx = 0
	}
	if idx >= len(s.VisibleIDs) {
		idx = len(s.VisibleIDs) - 1
	}
	s.FocusedID = s.VisibleIDs[idx]
}

// ActivityRow is the renderer-facing view of an activity. The caller (app)
// converts activity.ActivityView → ActivityRow at render time (so the UI
// package doesn't need to know about the activity types' internals).
type ActivityRow struct {
	ID       string
	Kind     activity.Kind
	Title    string
	Status   activity.Status
	Snapshot activity.Snapshot
}

// ActivityOverlayConfig bundles the overlay's render-time settings.
type ActivityOverlayConfig struct {
	Tick      int    // render-frame counter, used to rotate the fetch spinner
	CloseHint string
}

// RenderActivityOverlay paints the activity overlay overlay on top of base.
// state is mutated to populate VisibleIDs so the caller's next HandleKey
// call moves through the currently-rendered sort order.
func RenderActivityOverlay(state *ActivityOverlayState, rows []ActivityRow, cfg ActivityOverlayConfig, styles Styles, width, height int, base string) string {
	sorted := sortActivityRows(rows)

	state.VisibleIDs = state.VisibleIDs[:0]
	for _, r := range sorted {
		state.VisibleIDs = append(state.VisibleIDs, r.ID)
	}
	if state.FocusedID != "" {
		present := false
		for _, id := range state.VisibleIDs {
			if id == state.FocusedID {
				present = true
				break
			}
		}
		if !present {
			if len(state.VisibleIDs) > 0 {
				state.FocusedID = state.VisibleIDs[0]
			} else {
				state.FocusedID = ""
			}
		}
	} else if len(state.VisibleIDs) > 0 {
		state.FocusedID = state.VisibleIDs[0]
	}

	if state.View == ActivityDetailPane {
		return renderActivityDetail(state, sorted, cfg, styles, width, height, base)
	}
	return renderActivityList(state, sorted, cfg, styles, width, height, base)
}

func renderActivityList(state *ActivityOverlayState, rows []ActivityRow, cfg ActivityOverlayConfig, styles Styles, width, height int, base string) string {
	items := make([]OverlayItem, 0, len(rows))
	cursor := 0
	for i, r := range rows {
		if r.ID == state.FocusedID {
			cursor = i
		}
		line1 := fmt.Sprintf("%s %s%s", activityIcon(r, cfg.Tick), r.Title, activityStatusSuffix(r.Status))
		line2 := activityProgressLine(r.Snapshot)
		items = append(items, OverlayItem{
			Label: line1,
			Desc:  line2,
		})
	}
	if len(items) == 0 {
		items = []OverlayItem{{Label: "Nothing running."}}
	}

	listCfg := OverlayListConfig{
		Title:      "Activity",
		CloseHint:  cfg.CloseHint,
		InnerWidth: 80,
		MaxVisible: 18,
		HideSearch: true,
		Center:     true,
	}
	return RenderOverlayList(listCfg, items, cursor, styles.Overlay, width, height, base)
}

func renderActivityDetail(state *ActivityOverlayState, rows []ActivityRow, cfg ActivityOverlayConfig, styles Styles, width, height int, base string) string {
	var focused *ActivityRow
	for i := range rows {
		if rows[i].ID == state.FocusedID {
			focused = &rows[i]
			break
		}
	}
	if focused == nil {
		state.View = ActivityListPane
		return renderActivityList(state, rows, cfg, styles, width, height, base)
	}

	items := buildDetailItems(*focused)
	listCfg := OverlayListConfig{
		Title:      activityIcon(*focused, cfg.Tick) + " " + focused.Title + activityStatusSuffix(focused.Status),
		CloseHint:  "h/esc back · x cancel",
		InnerWidth: 80,
		MaxVisible: 16,
		HideSearch: true,
		Center:     true,
	}
	return RenderOverlayList(listCfg, items, 0, styles.Overlay, width, height, base)
}

func buildDetailItems(r ActivityRow) []OverlayItem {
	s := r.Snapshot
	var rows []OverlayItem

	switch r.Kind {
	case activity.KindUpload:
		pct := 0.0
		if s.TotalBytes > 0 {
			pct = float64(s.DoneBytes) / float64(s.TotalBytes)
			if pct > 1 {
				pct = 1
			}
		}
		rows = append(rows, OverlayItem{
			Label: activityMiniBar(pct, 40) + fmt.Sprintf("  %.0f%%", pct*100),
		})
		rate := ""
		if s.BytesPerSec > 0 {
			rate = " · " + activity.FormatDecimalRate(s.BytesPerSec)
		}
		eta := ""
		if s.BytesPerSec > 0 && s.TotalBytes > s.DoneBytes && s.Status == activity.StatusRunning {
			remain := float64(s.TotalBytes-s.DoneBytes) / s.BytesPerSec
			eta = " · ETA " + formatShortDurationSeconds(time.Duration(remain*float64(time.Second)))
		}
		elapsed := time.Since(s.StartedAt)
		if !s.FinishedAt.IsZero() {
			elapsed = s.FinishedAt.Sub(s.StartedAt)
		}
		elapsedStr := " · " + formatShortElapsed(elapsed)
		rows = append(rows, OverlayItem{
			Label: fmt.Sprintf("%s / %s%s%s%s",
				activity.FormatDecimalBytes(s.DoneBytes), activity.FormatDecimalBytes(s.TotalBytes),
				rate, eta, elapsedStr),
		})
		if file := truncateFilename(s.Detail, 60); file != "" {
			rows = append(rows, OverlayItem{Label: file})
		}
		if s.Skipped > 0 {
			rows = append(rows, OverlayItem{Label: fmt.Sprintf("Skipped: %d", s.Skipped)})
		}
		if s.Err != nil {
			rows = append(rows, OverlayItem{Label: "error: " + s.Err.Error()})
		}

	case activity.KindFetch:
		elapsed := time.Since(s.StartedAt)
		if !s.FinishedAt.IsZero() {
			elapsed = s.FinishedAt.Sub(s.StartedAt)
		}
		rows = append(rows, OverlayItem{
			Label: fmt.Sprintf("Elapsed: %s", formatShortElapsed(elapsed)),
		})
		rows = append(rows, OverlayItem{
			Label: fmt.Sprintf("Items: %d", s.Items),
		})
		if s.Detail != "" {
			rows = append(rows, OverlayItem{Label: s.Detail})
		}
		if s.Err != nil {
			rows = append(rows, OverlayItem{Label: "error: " + s.Err.Error()})
		}
	}
	return rows
}

// sortActivityRows orders rows: Running → WaitingInput → Errored → Cancelled
// → Done, then by StartedAt desc within each bucket.
func sortActivityRows(rows []ActivityRow) []ActivityRow {
	out := append([]ActivityRow(nil), rows...)
	sort.SliceStable(out, func(i, j int) bool {
		ri, rj := out[i], out[j]
		bi, bj := statusBucket(ri.Status), statusBucket(rj.Status)
		if bi != bj {
			return bi < bj
		}
		return ri.Snapshot.StartedAt.After(rj.Snapshot.StartedAt)
	})
	return out
}

func statusBucket(s activity.Status) int {
	switch s {
	case activity.StatusRunning:
		return 0
	case activity.StatusWaitingInput:
		return 1
	case activity.StatusErrored:
		return 2
	case activity.StatusCancelled:
		return 3
	case activity.StatusDone:
		return 4
	default:
		return 5
	}
}

func activityIcon(r ActivityRow, tick int) string {
	switch r.Kind {
	case activity.KindUpload:
		return "↑"
	case activity.KindDownload:
		return "↓"
	case activity.KindFetch:
		frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
		return frames[tick%len(frames)]
	}
	return "•"
}

func activityStatusSuffix(s activity.Status) string {
	switch s {
	case activity.StatusWaitingInput:
		return " · waiting"
	case activity.StatusErrored:
		return " · error"
	case activity.StatusDone:
		return " · done"
	case activity.StatusCancelled:
		return " · cancelled"
	}
	return ""
}

func activityProgressLine(s activity.Snapshot) string {
	elapsed := time.Since(s.StartedAt)
	if !s.FinishedAt.IsZero() {
		elapsed = s.FinishedAt.Sub(s.StartedAt)
	}
	if s.TotalBytes > 0 {
		pct := float64(s.DoneBytes) / float64(s.TotalBytes)
		if pct > 1 {
			pct = 1
		}
		bar := activityMiniBar(pct, 10)
		rate := ""
		if s.BytesPerSec > 0 {
			rate = fmt.Sprintf(" · %s", activity.FormatDecimalRate(s.BytesPerSec))
		}
		eta := ""
		if s.BytesPerSec > 0 && s.TotalBytes > s.DoneBytes {
			remain := float64(s.TotalBytes-s.DoneBytes) / s.BytesPerSec
			eta = fmt.Sprintf(" · ETA %s", formatShortDurationSeconds(time.Duration(remain*float64(time.Second))))
		}
		return fmt.Sprintf("%s %.0f%%  %s/%s%s%s",
			bar, pct*100,
			activity.FormatDecimalBytes(s.DoneBytes), activity.FormatDecimalBytes(s.TotalBytes),
			rate, eta)
	}
	if s.Items > 0 {
		return fmt.Sprintf("%d items · %s", s.Items, formatShortElapsed(elapsed))
	}
	return fmt.Sprintf("starting… · %s", formatShortElapsed(elapsed))
}

func activityMiniBar(pct float64, width int) string {
	filled := int(pct * float64(width))
	if filled < 0 {
		filled = 0
	}
	if filled > width {
		filled = width
	}
	return "[" + strings.Repeat("▓", filled) + strings.Repeat("░", width-filled) + "]"
}

func formatShortDurationSeconds(d time.Duration) string {
	if d < time.Second {
		return "1s"
	}
	sec := int(d.Seconds())
	if sec < 60 {
		return fmt.Sprintf("%ds", sec)
	}
	if sec < 3600 {
		return fmt.Sprintf("%dm %ds", sec/60, sec%60)
	}
	return fmt.Sprintf("%dh %dm", sec/3600, (sec%3600)/60)
}

func formatShortElapsed(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	return formatShortDurationSeconds(d)
}

func truncateFilename(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen < 4 {
		return s[:maxLen]
	}
	return "…" + s[len(s)-(maxLen-1):]
}
