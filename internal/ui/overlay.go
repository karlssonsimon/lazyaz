package ui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/karlssonsimon/lazyaz/internal/fuzzy"
	"github.com/karlssonsimon/lazyaz/internal/keymap"
)

const (
	overlayInnerWidth = 60
	overlayBoxWidth   = overlayInnerWidth + 6 // padding(2+2) + border(1+1)
	overlayMaxVisible = 20
	overlayChevron    = " › "
)

// OverlayItem is a single entry in an overlay list.
type OverlayItem struct {
	Label    string
	Desc     string // optional second line below label (muted)
	Hint     string // right-aligned secondary text (shortcut, author, etc.)
	IsActive bool   // shows a marker (e.g. * for current theme)
	IsHeader bool   // renders Label as a section title; never selectable
}

// OverlayBindings tells the overlay renderer which keymap bindings the
// dialog actually responds to, so footer hints reflect the truth instead
// of guessing from a fixed (Theme*) subset of the Keymap. Each dialog
// declares its own bindings — exactly the same set it passes to its
// HandleKey method.
type OverlayBindings struct {
	MoveUp   keymap.Binding
	MoveDown keymap.Binding
	Apply    keymap.Binding
	Cancel   keymap.Binding
	Erase    keymap.Binding
}

// OverlayListConfig configures the dimensions and placement of an overlay list.
type OverlayListConfig struct {
	Title      string
	Query      string
	CursorView string // rendered cursor; falls back to static "█" if empty
	CloseHint  string // free text shown right-aligned in the header (e.g. "esc close")
	InnerWidth int    // content width; 0 = default (60)
	MaxVisible int    // max visible items; 0 = default (20)
	Center     bool   // center vertically instead of 1/5 from top
	HideSearch bool   // suppress the inline filter; treats overlay as a menu

	// Placeholder is shown in the filter slot when Query is empty (italic muted).
	Placeholder string
	// Total, when > 0, makes the header counter render as "<filtered>/<Total>"
	// instead of "<filtered> matches". Caller passes the raw source size.
	Total int
	// Breadcrumb is rendered between the badge and the filter, separated by ›.
	// Use it for context like a tenant domain or path.
	Breadcrumb []string

	// ModeBadge is the label shown in the footer's mode pill (e.g. "PICKER",
	// "MENU"). Defaults to "PICKER" when HideSearch is false, "MENU" otherwise.
	ModeBadge string
	// Actions overrides the auto-generated footer hints when non-empty.
	// Most callers leave this nil and pass Bindings instead.
	Actions []StatusAction
	// Bindings declares the actual keymap bindings the dialog answers to.
	// When set, the footer hints are derived from these so a user remapping
	// e.g. theme_apply propagates to the hint row automatically. When nil,
	// falls back to opinionated hardcoded defaults.
	Bindings *OverlayBindings
	// ActiveLabel is the right-aligned hint shown next to the active row
	// (defaults to "current"). Set to "-" to suppress.
	ActiveLabel string
	// NoActiveMarker drops the 2-col marker slot reserved for the IsActive
	// indicator. Use it for overlays that never set IsActive — items then
	// align flush with section headers instead of sitting indented under
	// them.
	NoActiveMarker bool
}

// RenderOverlayList renders an overlay using the floating-screen pattern:
// header badge + breadcrumb + inline filter + counter + close hint, body
// with active-row marker and rose-gutter cursor row, footer with mode pill
// and keymap-driven action hints.
func RenderOverlayList(cfg OverlayListConfig, items []OverlayItem, cursor int, styles Styles, termWidth, termHeight int, base string) string {
	innerW := cfg.InnerWidth
	if innerW <= 0 {
		innerW = overlayInnerWidth
	}
	boxW := innerW + 6
	if boxW > termWidth-4 {
		boxW = termWidth - 4
		innerW = boxW - 6
	}
	if innerW < 20 {
		innerW = 20
	}

	maxVis := cfg.MaxVisible
	if maxVis <= 0 {
		maxVis = overlayMaxVisible
	}

	ov := styles.Overlay

	// Both Normal and Cursor styles must produce the same rendered width.
	// Normal uses Padding(0,0,0,2) = 2 left pad; Cursor uses a 1-char
	// left border + Padding(0,0,0,1) = 2 total.
	padH := ov.Normal.GetHorizontalPadding() + ov.Normal.GetHorizontalBorderSize()
	contentW := innerW - padH

	normalStyle := ov.Normal.Width(innerW)
	cursorStyle := ov.Cursor.Width(innerW)

	var rows []string

	// --- Header ---
	rows = append(rows, renderOverlayHeader(cfg, len(items), styles, innerW))
	rows = append(rows, ov.Rule.Render(strings.Repeat("─", innerW)))

	// Pre-render a single empty row for padding.
	emptyRow := normalStyle.Render("")

	// --- Body ---
	visibleStart, visibleEnd := overlayScrollWindow(cursor, len(items), maxVis)
	bodyTargetRows := overlayBodyTargetRows(items, maxVis)

	if len(items) == 0 {
		rows = append(rows, ov.NoMatch.Render("No matches"))
		for i := 1; i < bodyTargetRows; i++ {
			rows = append(rows, emptyRow)
		}
	} else {
		bodyRows := renderOverlayBodyRows(cfg, items, cursor, visibleStart, visibleEnd, contentW, normalStyle, cursorStyle, ov, innerW)
		rows = append(rows, bodyRows...)
		for i := len(bodyRows); i < bodyTargetRows; i++ {
			rows = append(rows, emptyRow)
		}
	}

	// --- Footer ---
	rows = append(rows, ov.Rule.Render(strings.Repeat("─", innerW)))
	rows = append(rows, renderOverlayFooter(cfg, styles, innerW))

	content := lipgloss.JoinVertical(lipgloss.Left, rows...)
	box := ov.Box.Width(boxW).Render(content)

	if cfg.Center {
		return PlaceOverlay(termWidth, termHeight, box, base)
	}
	return placeOverlayTop(termWidth, termHeight, box, base)
}

// renderOverlayHeader builds the top row:
//
//	badge › crumb › crumb › <filter>  count        esc close
func renderOverlayHeader(cfg OverlayListConfig, filtered int, styles Styles, innerW int) string {
	ov := styles.Overlay
	chevron := ov.Hint.Inline(true).Padding(0).Render(overlayChevron)

	left := ov.HeaderBadge.Render(strings.ToUpper(cfg.Title))

	for _, crumb := range cfg.Breadcrumb {
		if crumb == "" {
			continue
		}
		left = left + chevron + ov.Input.Render(crumb)
	}

	if !cfg.HideSearch {
		cursorStr := cfg.CursorView
		if cursorStr == "" {
			cursorStr = "█"
		}

		var filter string
		switch {
		case cfg.Query == "" && cfg.Placeholder != "":
			filter = cursorStr + ov.Hint.Inline(true).Padding(0).Italic(true).Render(cfg.Placeholder)
		case cfg.Query == "":
			filter = cursorStr
		default:
			filter = ov.Input.Render(cfg.Query) + cursorStr
		}

		count := ""
		if cfg.Query != "" {
			switch {
			case cfg.Total > 0:
				count = "  " + ov.HeaderCount.Render(formatInt(filtered)+" / "+formatInt(cfg.Total))
			default:
				count = "  " + ov.HeaderCount.Render(formatOverlayCount(filtered))
			}
		}
		left = left + chevron + filter + count
	}

	right := ""
	if cfg.CloseHint != "" {
		right = ov.Hint.Inline(true).Padding(0).Render(cfg.CloseHint)
	}

	return overlayJustifyRow(left, right, innerW, ov)
}

// renderOverlayFooter builds the bottom row: [MODE] j/k move · ↵ apply · …
func renderOverlayFooter(cfg OverlayListConfig, styles Styles, innerW int) string {
	chrome := styles.Chrome
	ov := styles.Overlay

	mode := cfg.ModeBadge
	if mode == "" {
		if cfg.HideSearch {
			mode = "MENU"
		} else {
			mode = "PICKER"
		}
	}

	actions := cfg.Actions
	if len(actions) == 0 {
		actions = defaultOverlayActions(cfg.Bindings, cfg.HideSearch, cfg.Query != "")
	}

	parts := []string{chrome.StatusMode.Render(mode)}
	for _, a := range actions {
		if a.Key == "" {
			continue
		}
		label := a.Label
		if label != "" {
			label = " " + label
		}
		parts = append(parts, chrome.StatusKey.Render(a.Key)+ov.Hint.Inline(true).Padding(0).Render(label))
	}

	left := strings.Join(parts, ov.Hint.Inline(true).Padding(0).Render("  "))
	return overlayJustifyRow(left, "", innerW, ov)
}

// defaultOverlayActions returns the footer hints for an overlay. When the
// caller declares its actual Bindings, every hint reflects the real bound
// keys (so a user remapping their keymap JSON sees their key in the hint
// row). When nil, falls back to opinionated hardcoded defaults.
func defaultOverlayActions(b *OverlayBindings, hideSearch, hasQuery bool) []StatusAction {
	if b != nil {
		// In picker overlays the filter consumes plain single-char keys, so
		// the meaningful navigation key is the modifier-prefixed one
		// (e.g. ctrl+j over plain "down"). In menu overlays (HideSearch)
		// plain j/k are fine.
		key := func(bind keymap.Binding) string {
			if hideSearch {
				return bind.Short()
			}
			return modifierKey(bind)
		}
		moveKey := key(b.MoveDown) + "/" + key(b.MoveUp)
		if hideSearch {
			return []StatusAction{
				{Key: moveKey, Label: "move"},
				{Key: b.Apply.Short(), Label: "select"},
				{Key: b.Cancel.Short(), Label: "close"},
			}
		}
		out := []StatusAction{
			{Key: moveKey, Label: "move"},
			{Key: b.Apply.Short(), Label: "apply"},
			{Key: b.Cancel.Short(), Label: "cancel"},
		}
		if hasQuery && b.Erase.Short() != "" {
			out = append(out, StatusAction{Key: b.Erase.Short(), Label: "clear"})
		}
		return out
	}

	if hideSearch {
		return []StatusAction{
			{Key: "j/k", Label: "move"},
			{Key: "↵", Label: "select"},
			{Key: "esc", Label: "close"},
		}
	}
	out := []StatusAction{
		{Key: "ctrl+j/k", Label: "move"},
		{Key: "↵", Label: "apply"},
		{Key: "esc", Label: "cancel"},
	}
	if hasQuery {
		out = append(out, StatusAction{Key: "⌫", Label: "clear"})
	}
	return out
}

// renderOverlayBodyRows returns the rendered body rows for the visible
// window. Rows include the active marker, cursor styling, dashed divider
// after the active row, and filter-match highlighting on labels.
//
// All inline spans inside a row are rendered with the row's background so
// the row paints uniformly — child `\x1b[0m` resets won't leak through and
// punch holes in the cursor row's selBg.
func renderOverlayBodyRows(cfg OverlayListConfig, items []OverlayItem, cursor, start, end, contentW int, normal, cursorStyle lipgloss.Style, ov OverlayStyles, innerW int) []string {
	var rows []string

	activeLabel := cfg.ActiveLabel
	if activeLabel == "" {
		activeLabel = "current"
	}
	if activeLabel == "-" {
		activeLabel = ""
	}

	rowBg := normal.GetBackground()
	cursorBg := cursorStyle.GetBackground()

	for ci := start; ci < end; ci++ {
		item := items[ci]

		if item.IsHeader {
			// Section title row: bold colored label, no marker, no cursor
			// styling. The caller should keep the cursor on real items so
			// this branch only fires for non-cursor rows.
			title := ov.SectionTitle.Background(rowBg).Render(item.Label)
			rows = append(rows, normal.Render(title))
			continue
		}

		bg := rowBg
		style := normal
		if ci == cursor {
			bg = cursorBg
			style = cursorStyle
		}

		markerStyle := ov.ActiveMarker.Background(bg)
		matchStyle := ov.Match.Background(bg)
		hintStyle := ov.RowHint.Background(bg)
		baseStyle := lipgloss.NewStyle().Background(bg)

		markerW := 2
		if cfg.NoActiveMarker {
			markerW = 0
		}
		marker := strings.Repeat(" ", markerW)
		if item.IsActive && !cfg.NoActiveMarker {
			marker = markerStyle.Render("•") + " "
		}

		hint := item.Hint
		if item.IsActive && activeLabel != "" && hint == "" {
			hint = activeLabel
		}

		nameWidth := contentW - markerW
		if hint != "" {
			nameWidth = nameWidth - lipgloss.Width(hint) - 2
		}
		if nameWidth < 10 {
			nameWidth = 10
		}

		// Clamp the visible label to nameWidth so the hint column always
		// gets its slot — long names elide with `…` instead of pushing the
		// GUID off the row.
		visibleLabel := truncateLabel(item.Label, nameWidth)
		labelRendered := highlightFuzzyMatch(visibleLabel, cfg.Query, matchStyle, baseStyle)

		labelPadded := labelRendered
		if pad := nameWidth - lipgloss.Width(labelRendered); pad > 0 {
			labelPadded = labelRendered + baseStyle.Render(strings.Repeat(" ", pad))
		}

		entry := marker + labelPadded
		if hint != "" {
			entry += baseStyle.Render("  ") + hintStyle.Render(hint)
		}

		rows = append(rows, style.Render(entry))

		if item.Desc != "" {
			rows = append(rows, style.Render("  "+item.Desc))
		}

		// Dashed divider after the active row when followed by more rows.
		if item.IsActive && ci+1 < end {
			rows = append(rows, ov.DashedRule.Render(strings.Repeat("╌", innerW)))
		}
	}

	return rows
}

// overlayBodyTargetRows returns the number of body rows the overlay should
// reserve so the box height stays constant across queries.
func overlayBodyTargetRows(items []OverlayItem, maxVis int) int {
	rowsPerItem := 1
	if len(items) > 0 && items[0].Desc != "" {
		rowsPerItem = 2
	}
	return maxVis * rowsPerItem
}

// overlayScrollWindow returns [start, end) bounds for visible items based
// on cursor position and max visible rows.
func overlayScrollWindow(cursor, total, maxVis int) (int, int) {
	if total == 0 {
		return 0, 0
	}
	visible := min(maxVis, total)
	start := 0
	if cursor >= start+visible {
		start = cursor - visible + 1
	}
	if cursor < start {
		start = cursor
	}
	end := start + visible
	if end > total {
		end = total
		start = max(0, end-visible)
	}
	return start, end
}

// overlayJustifyRow places `left` at the start and `right` at the end,
// padding the gap with the overlay's hint background.
func overlayJustifyRow(left, right string, innerW int, ov OverlayStyles) string {
	leftW := lipgloss.Width(left)
	rightW := lipgloss.Width(right)
	gap := innerW - leftW - rightW
	if gap < 1 {
		gap = 1
	}
	return left + ov.HintFull.Render(strings.Repeat(" ", gap)) + right
}

// modifierKey returns the first key in the binding that uses a modifier
// (ctrl+/alt+/shift+). Falls back to Short() when no modifier-prefixed key
// is bound. Used in picker overlays so the hint reflects the key that
// doesn't compete with the filter input.
func modifierKey(b keymap.Binding) string {
	for _, k := range b.Keys {
		if strings.Contains(k, "+") {
			return k
		}
	}
	return b.Short()
}

// formatOverlayCount renders the "N / total" counter. Total is the visible
// (filtered) count; the source total is not threaded through the renderer.
func formatOverlayCount(visible int) string {
	if visible == 1 {
		return "1 match"
	}
	return formatInt(visible) + " matches"
}

// formatInt is a tiny strconv-free integer printer (avoids importing strconv
// just for this leaf function).
func formatInt(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// highlightFuzzyMatch renders `label` with fuzzy-matched chars wrapped in
// matchStyle and the rest in baseStyle. Returns plain rendered string when
// query is empty or there's no match.
func highlightFuzzyMatch(label, query string, matchStyle, baseStyle lipgloss.Style) string {
	if strings.TrimSpace(query) == "" {
		return baseStyle.Render(label)
	}
	res := fuzzy.Match(query, label)
	if res.Score == 0 || len(res.Pos) == 0 {
		return baseStyle.Render(label)
	}
	pos := make(map[int]bool, len(res.Pos))
	for _, p := range res.Pos {
		pos[p] = true
	}
	var b strings.Builder
	bytes := []byte(label)
	chunkStart := 0
	inMatch := false
	for i := 0; i <= len(bytes); i++ {
		var matched bool
		if i < len(bytes) {
			matched = pos[i]
		}
		if i == len(bytes) || matched != inMatch {
			chunk := string(bytes[chunkStart:i])
			if chunk != "" {
				if inMatch {
					b.WriteString(matchStyle.Render(chunk))
				} else {
					b.WriteString(baseStyle.Render(chunk))
				}
			}
			chunkStart = i
			inMatch = matched
		}
	}
	return b.String()
}

// padRightRendered pads `rendered` (which may contain ANSI styling) to the
// given visual width.
func padRightRendered(rendered string, width int) string {
	w := lipgloss.Width(rendered)
	if w >= width {
		return rendered
	}
	return rendered + strings.Repeat(" ", width-w)
}

// truncateLabel clamps a plain string to width with a trailing ellipsis when
// truncated. Used by overlay rows so long labels don't push the right-aligned
// hint column off the row.
func truncateLabel(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= width {
		return s
	}
	if width <= 1 {
		return "…"
	}
	runes := []rune(s)
	for i := len(runes); i > 0; i-- {
		candidate := string(runes[:i]) + "…"
		if lipgloss.Width(candidate) <= width {
			return candidate
		}
	}
	return "…"
}

// placeOverlayTop places the overlay near the top (1/5 down) and centered
// horizontally.
func placeOverlayTop(width, height int, overlay, base string) string {
	overlayLines := strings.Split(overlay, "\n")
	baseLines := strings.Split(base, "\n")

	for len(baseLines) < height {
		baseLines = append(baseLines, "")
	}

	oH := len(overlayLines)
	oW := 0
	for _, l := range overlayLines {
		if w := lipgloss.Width(l); w > oW {
			oW = w
		}
	}

	startY := height / 5
	startX := (width - oW) / 2
	if startY < 1 {
		startY = 1
	}
	if startX < 0 {
		startX = 0
	}
	if startY+oH > height {
		startY = max(0, height-oH)
	}

	for i, ol := range overlayLines {
		row := startY + i
		if row >= len(baseLines) {
			break
		}
		line := baseLines[row]
		lineW := lipgloss.Width(line)

		var out strings.Builder
		if startX > 0 {
			if lineW >= startX {
				out.WriteString(truncateAnsi(line, startX))
			} else {
				out.WriteString(line)
				out.WriteString(strings.Repeat(" ", startX-lineW))
			}
		}
		out.WriteString(ol)
		rightCol := startX + oW
		if lineW > rightCol {
			out.WriteString(skipAnsi(line, rightCol))
		}
		baseLines[row] = out.String()
	}

	return strings.Join(baseLines[:height], "\n")
}

// PlaceOverlay places the overlay centered on screen.
func PlaceOverlay(width, height int, overlay, base string) string {
	overlayLines := strings.Split(overlay, "\n")
	baseLines := strings.Split(base, "\n")

	for len(baseLines) < height {
		baseLines = append(baseLines, "")
	}

	oH := len(overlayLines)
	oW := 0
	for _, l := range overlayLines {
		if w := lipgloss.Width(l); w > oW {
			oW = w
		}
	}

	startY := (height - oH) / 2
	startX := (width - oW) / 2
	if startY < 0 {
		startY = 0
	}
	if startX < 0 {
		startX = 0
	}

	for i, ol := range overlayLines {
		row := startY + i
		if row >= len(baseLines) {
			break
		}
		line := baseLines[row]
		lineW := lipgloss.Width(line)

		var out strings.Builder
		if startX > 0 {
			if lineW >= startX {
				out.WriteString(truncateAnsi(line, startX))
			} else {
				out.WriteString(line)
				out.WriteString(strings.Repeat(" ", startX-lineW))
			}
		}
		out.WriteString(ol)
		rightCol := startX + oW
		if lineW > rightCol {
			out.WriteString(skipAnsi(line, rightCol))
		}
		baseLines[row] = out.String()
	}

	return strings.Join(baseLines[:height], "\n")
}

func skipAnsi(s string, skipWidth int) string {
	runes := []rune(s)
	for i := 0; i <= len(runes); i++ {
		prefix := string(runes[:i])
		if lipgloss.Width(prefix) >= skipWidth {
			return string(runes[i:])
		}
	}
	return ""
}

// padRight pads s with spaces to reach the given display width.
func padRight(s string, width int) string {
	w := lipgloss.Width(s)
	if w >= width {
		return s
	}
	return s + strings.Repeat(" ", width-w)
}

func truncateAnsi(s string, maxWidth int) string {
	if lipgloss.Width(s) <= maxWidth {
		return s
	}
	runes := []rune(s)
	for i := len(runes); i > 0; i-- {
		candidate := string(runes[:i])
		if lipgloss.Width(candidate) <= maxWidth {
			return candidate
		}
	}
	return ""
}
