package ui

import (
	"fmt"
	icolor "image/color"
	"sort"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
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
// activity ID. The cursor lives as an ID (not an index) so it stays stable
// across sort shuffles and arrival of new activities.
type ActivityOverlayState struct {
	Active    bool
	View      ActivityPane
	FocusedID string
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
// describing what the caller should do next. rows is the current
// activity snapshot from the registry — passed in so navigation walks
// the same sorted order the renderer uses (state can't cache it: View
// runs on a value receiver and mutations there don't survive).
func (s *ActivityOverlayState) HandleKey(key string, rows []ActivityRow) ActivityResult {
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
		s.moveCursor(rows, 1)
	case "k", "up":
		s.moveCursor(rows, -1)
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

func (s *ActivityOverlayState) moveCursor(rows []ActivityRow, delta int) {
	sorted := sortActivityRows(rows)
	if len(sorted) == 0 {
		s.FocusedID = ""
		return
	}
	idx := -1
	for i, r := range sorted {
		if r.ID == s.FocusedID {
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
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	s.FocusedID = sorted[idx].ID
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
	Tick      int // render-frame counter, used to rotate the fetch spinner
	CloseHint string
	Bindings  *OverlayBindings
}

const (
	activityInnerWidth = 100
	activityMaxBody    = 28
	activityBarWidth   = 10
)

// RenderActivityOverlay paints the activity overlay on top of base.
// state.FocusedID is anchored to the first visible row when missing so
// the user always has something to drive — note that View() runs on a
// value receiver so any mutation to state here is local to this call.
// Persistent navigation state lives on FocusedID, updated by HandleKey.
func RenderActivityOverlay(state *ActivityOverlayState, rows []ActivityRow, cfg ActivityOverlayConfig, styles Styles, width, height int, base string) string {
	sorted := sortActivityRows(rows)

	if state.FocusedID == "" && len(sorted) > 0 {
		state.FocusedID = sorted[0].ID
	}

	if state.View == ActivityDetailPane {
		return renderActivityDetail(state, sorted, cfg, styles, width, height, base)
	}
	return renderActivityList(state, sorted, cfg, styles, width, height, base)
}

func renderActivityList(state *ActivityOverlayState, rows []ActivityRow, cfg ActivityOverlayConfig, styles Styles, termWidth, termHeight int, base string) string {
	innerW := activityInnerWidth
	boxW := innerW + 6
	if boxW > termWidth-4 {
		boxW = termWidth - 4
		innerW = boxW - 6
	}
	if innerW < 50 {
		innerW = 50
		boxW = innerW + 6
	}

	ov := styles.Overlay
	bodyRows := []string{}

	if len(rows) == 0 {
		bodyRows = renderActivityEmptyBody(styles, innerW)
	} else {
		bodyRows = renderActivitySections(state, rows, cfg, styles, innerW)
	}

	// Pad body to a consistent height so the box doesn't shrink as the
	// activity list drains.
	for len(bodyRows) < activityMaxBody {
		bodyRows = append(bodyRows, "")
	}
	if len(bodyRows) > activityMaxBody {
		bodyRows = bodyRows[:activityMaxBody]
	}

	out := []string{renderActivityHeader(rows, styles, innerW)}
	out = append(out, ov.Rule.Render(strings.Repeat("─", innerW)))
	out = append(out, bodyRows...)
	out = append(out, ov.Rule.Render(strings.Repeat("─", innerW)))
	out = append(out, renderActivityFooter(styles, innerW))

	content := lipgloss.JoinVertical(lipgloss.Left, out...)
	box := ov.Box.Width(boxW).Render(content)
	return PlaceOverlay(termWidth, termHeight, box, base)
}

// renderActivityHeader builds: ACTIVITY › range  ● N live · ● N attn · ● N done    esc close.
// Range is omitted (we don't track a window) but the chevron + counters give
// the same visual rhythm as the mockup.
func renderActivityHeader(rows []ActivityRow, styles Styles, innerW int) string {
	ov := styles.Overlay

	live, attn, done := 0, 0, 0
	for _, r := range rows {
		switch r.Status {
		case activity.StatusRunning:
			live++
		case activity.StatusWaitingInput, activity.StatusErrored:
			attn++
		case activity.StatusDone, activity.StatusCancelled:
			done++
		}
	}

	chevron := ov.Hint.Inline(true).Padding(0).Render(overlayChevron)
	right := ov.Hint.Inline(true).Padding(0).Render("esc close")
	badge := ov.HeaderBadge.Render("ACTIVITY")
	muted := ov.Hint.Inline(true).Padding(0)

	dot := func(style lipgloss.Style, label string) string {
		return style.Render("●") + muted.Render(" "+label)
	}
	parts := []string{
		dot(styles.Warning, fmt.Sprintf("%d live", live)),
	}
	if attn > 0 {
		parts = append(parts, dot(styles.DangerBold, fmt.Sprintf("%d attention", attn)))
	}
	parts = append(parts, dot(styles.Accent, fmt.Sprintf("%d recent", done)))

	left := badge + chevron + strings.Join(parts, muted.Render(" · "))
	return overlayJustifyRow(left, right, innerW, ov)
}

// renderActivityEmptyBody paints a centered cyan ring + helper text when
// the activity list is empty.
func renderActivityEmptyBody(styles Styles, innerW int) []string {
	ov := styles.Overlay
	rows := []string{}

	pad := activityMaxBody / 2
	for i := 0; i < pad-2; i++ {
		rows = append(rows, "")
	}
	rows = append(rows, centerLine(styles.Accent.Render("○"), innerW))
	rows = append(rows, "")
	rows = append(rows, centerLine(ov.Input.Render("No active work."), innerW))
	rows = append(rows, "")
	rows = append(rows, centerLine(ov.Hint.Inline(true).Padding(0).Render("uploads will appear here automatically"), innerW))
	return rows
}

// renderActivitySections returns body rows grouped LIVE / ATTENTION / RECENT.
// Empty groups are skipped so a single-running activity doesn't draw two
// blank section headers.
func renderActivitySections(state *ActivityOverlayState, rows []ActivityRow, cfg ActivityOverlayConfig, styles Styles, innerW int) []string {
	live, attn, recent := groupActivityRows(rows)

	var out []string
	first := true
	for _, group := range []struct {
		title   string
		context string
		rows    []ActivityRow
	}{
		{"LIVE", fmt.Sprintf("%d in flight", len(live)), live},
		{"ATTENTION", fmt.Sprintf("%d need a look", len(attn)), attn},
		{"RECENT", fmt.Sprintf("%d done", len(recent)), recent},
	} {
		if len(group.rows) == 0 {
			continue
		}
		if !first {
			out = append(out, "")
		}
		first = false
		out = append(out, renderActivitySectionHeader(group.title, group.context, styles, innerW))
		for _, r := range group.rows {
			out = append(out, renderActivityRow(r, r.ID == state.FocusedID, cfg.Tick, styles, innerW))
		}
	}
	return out
}

func groupActivityRows(rows []ActivityRow) (live, attn, recent []ActivityRow) {
	for _, r := range rows {
		switch r.Status {
		case activity.StatusRunning:
			live = append(live, r)
		case activity.StatusWaitingInput, activity.StatusErrored:
			attn = append(attn, r)
		case activity.StatusDone, activity.StatusCancelled:
			recent = append(recent, r)
		}
	}
	return
}

func renderActivitySectionHeader(title, context string, styles Styles, innerW int) string {
	ov := styles.Overlay
	left := ov.HeaderCount.Render(title)
	right := ov.HeaderCount.Render(context)
	return overlayJustifyRow(left, right, innerW, ov)
}

// Activity row column widths. Fixed so bars and right-aligned stats line
// up across every row; title absorbs the remainder.
const (
	activityKindColWidth   = 8
	activityStatsColWidth  = 16
	activityRightColWidth  = 12
	activityRowFixedColumn = 2 + 2 + 1 + activityKindColWidth + 1 + activityBarWidth + 1 +
		activityStatsColWidth + 2 + activityRightColWidth // gutter + icon + space + kind + space + bar + space + stats + gap + right
)

// renderActivityRow paints one item: gutter + status icon + KIND + title +
// bar + stats + right-aligned status text. Column widths are fixed so the
// bar and right columns line up across every row regardless of content.
// Cursor row gets selBg highlight + rose gutter.
func renderActivityRow(r ActivityRow, focused bool, tick int, styles Styles, innerW int) string {
	ov := styles.Overlay

	bg := ov.Normal.GetBackground()
	if focused {
		bg = ov.Cursor.GetBackground()
	}
	baseStyle := lipgloss.NewStyle().Background(bg)
	muted := ov.RowHint.Background(bg)

	gutter := "  "
	if focused {
		gutter = styles.Warning.Background(ov.Normal.GetBackground()).Render("▍") + " "
	}

	icon := activityStatusIcon(r, tick, styles).Background(bg).Render(activityStatusGlyph(r, tick))
	kind := muted.Render(padRight(activityKindLabel(r.Kind), activityKindColWidth))

	bar := activityProgressBar(r, styles, bg)
	stats := padLeftRendered(muted.Render(activityRowStats(r)), activityStatsColWidth)
	rightCol := padLeftRendered(muted.Render(activityRowRightText(r)), activityRightColWidth)

	titleW := innerW - activityRowFixedColumn
	if titleW < 12 {
		titleW = 12
	}
	titleText := truncateLabel(r.Title, titleW)
	titleStyle := baseStyle
	if focused {
		titleStyle = lipgloss.NewStyle().Background(bg).Foreground(ov.Cursor.GetForeground()).Bold(true)
	}
	titlePadded := titleStyle.Render(titleText)
	if pad := titleW - lipgloss.Width(titleText); pad > 0 {
		titlePadded += baseStyle.Render(strings.Repeat(" ", pad))
	}

	sp := baseStyle.Render(" ")
	gap := baseStyle.Render("  ")
	return gutter + icon + sp + kind + sp + titlePadded + sp + bar + sp + stats + gap + rightCol
}

// padLeftRendered right-aligns rendered text inside a fixed visual width.
func padLeftRendered(rendered string, width int) string {
	w := lipgloss.Width(rendered)
	if w >= width {
		return rendered
	}
	return strings.Repeat(" ", width-w) + rendered
}

// activityStatusIcon returns a styled icon style; activityStatusGlyph
// returns the literal char (split so the caller can apply the bg).
func activityStatusIcon(r ActivityRow, tick int, styles Styles) lipgloss.Style {
	switch r.Status {
	case activity.StatusErrored:
		return styles.DangerBold
	case activity.StatusWaitingInput:
		return styles.Warning
	case activity.StatusDone:
		return styles.Accent
	case activity.StatusCancelled:
		return styles.Muted
	default: // Running
		return styles.Warning
	}
}

func activityStatusGlyph(r ActivityRow, tick int) string {
	switch r.Status {
	case activity.StatusErrored:
		return "✗"
	case activity.StatusWaitingInput:
		return "⏸"
	case activity.StatusDone:
		return "✓"
	case activity.StatusCancelled:
		return "⊘"
	}
	frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	return frames[tick%len(frames)]
}

func activityKindLabel(k activity.Kind) string {
	switch k {
	case activity.KindUpload:
		return "UPLOAD"
	case activity.KindDownload:
		return "DOWNLOAD"
	case activity.KindFetch:
		return "FETCH"
	}
	return "TASK"
}

// activityProgressBar renders a 10-cell column. Running uploads show a
// proportional yellow magnitude bar; everything else (queued, errored,
// done, cancelled) shows a muted dotted placeholder so the column stays
// aligned without painting useless solid bars on completed rows.
func activityProgressBar(r ActivityRow, styles Styles, bg icolor.Color) string {
	muted := styles.Muted.Background(bg)
	if r.Status != activity.StatusRunning || r.Snapshot.TotalBytes == 0 {
		return muted.Render(strings.Repeat("·", activityBarWidth))
	}

	pct := activityRowPct(r)
	filled := int(pct * float64(activityBarWidth))
	if filled < 0 {
		filled = 0
	}
	if filled > activityBarWidth {
		filled = activityBarWidth
	}

	fill := styles.Warning.Background(bg).Render(strings.Repeat("█", filled))
	rest := muted.Render(strings.Repeat("░", activityBarWidth-filled))
	return fill + rest
}

func activityRowPct(r ActivityRow) float64 {
	if r.Status == activity.StatusDone {
		return 1
	}
	if r.Snapshot.TotalBytes > 0 {
		p := float64(r.Snapshot.DoneBytes) / float64(r.Snapshot.TotalBytes)
		if p > 1 {
			return 1
		}
		return p
	}
	return 0
}

func activityRowStats(r ActivityRow) string {
	s := r.Snapshot
	if s.TotalBytes > 0 {
		stats := fmt.Sprintf("%s/%s",
			activity.FormatDecimalBytes(s.DoneBytes),
			activity.FormatDecimalBytes(s.TotalBytes))
		if s.BytesPerSec > 0 {
			stats += " · " + activity.FormatDecimalRate(s.BytesPerSec)
		}
		return stats
	}
	if s.Items > 0 {
		return fmt.Sprintf("%d items", s.Items)
	}
	return ""
}

// activityRowRightText is the per-row right-aligned hint: ETA for running
// uploads, error code/message for errored, elapsed for done, blank otherwise.
func activityRowRightText(r ActivityRow) string {
	s := r.Snapshot
	switch r.Status {
	case activity.StatusErrored:
		if s.Err != nil {
			return s.Err.Error()
		}
		return "error"
	case activity.StatusCancelled:
		return "cancelled"
	case activity.StatusDone:
		elapsed := time.Since(s.StartedAt)
		if !s.FinishedAt.IsZero() {
			elapsed = s.FinishedAt.Sub(s.StartedAt)
		}
		return formatShortElapsed(elapsed)
	case activity.StatusWaitingInput:
		return "waiting"
	}
	if s.BytesPerSec > 0 && s.TotalBytes > s.DoneBytes {
		remain := float64(s.TotalBytes-s.DoneBytes) / s.BytesPerSec
		return "eta " + formatShortDurationSeconds(time.Duration(remain*float64(time.Second)))
	}
	if !s.StartedAt.IsZero() {
		return "running"
	}
	return ""
}

func renderActivityFooter(styles Styles, innerW int) string {
	chrome := styles.Chrome
	ov := styles.Overlay
	parts := []string{chrome.StatusMode.Render("ACTIVITY")}
	for _, a := range []StatusAction{
		{Key: "j/k", Label: "move"},
		{Key: "↵", Label: "open"},
		{Key: "x", Label: "cancel"},
		{Key: "esc", Label: "close"},
	} {
		parts = append(parts, chrome.StatusKey.Render(a.Key)+ov.Hint.Inline(true).Padding(0).Render(" "+a.Label))
	}
	left := strings.Join(parts, ov.Hint.Inline(true).Padding(0).Render("  "))
	return overlayJustifyRow(left, "", innerW, ov)
}

// centerLine pads s with leading/trailing spaces to center it in width.
func centerLine(s string, width int) string {
	w := lipgloss.Width(s)
	if w >= width {
		return s
	}
	left := (width - w) / 2
	right := width - w - left
	return strings.Repeat(" ", left) + s + strings.Repeat(" ", right)
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
		Title:      activityKindLabel(focused.Kind) + " · " + focused.Title,
		CloseHint:  "h/esc back · x cancel",
		InnerWidth: 80,
		MaxVisible: 16,
		HideSearch: true,
		Center:     true,
		Bindings:   cfg.Bindings,
	}
	return RenderOverlayList(listCfg, items, 0, styles, width, height, base)
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
