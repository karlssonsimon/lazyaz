package ui

import (
	"strings"

	"charm.land/bubbles/v2/list"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

type MillerColumnFrame struct {
	Width     int
	Height    int
	Focused   bool
	RightRule bool
}

type MillerColumn struct {
	Title     string
	TitleMeta string // optional right-aligned meta on the title row (e.g. "16 / 154")
	SubHeader string // optional second header row (e.g. "NAME · MODIFIED · SIZE")
	Body      string
	Footer    string
	Frame     MillerColumnFrame
}

type MillerListColumn struct {
	List      *list.Model
	Title     string
	TitleMeta string // optional right-aligned meta on the title row
	SubHeader string // optional second header row rendered between title and list body
	Footer    string
	Frame     MillerColumnFrame
}

func MillerColumnContentWidth(frame MillerColumnFrame) int {
	width := frame.Width
	if frame.RightRule {
		width--
	}
	if width < 0 {
		return 0
	}
	return width
}

func MillerListBodyHeight(totalHeight int, hasFooter bool) int {
	height := totalHeight - 1
	if hasFooter {
		height--
	}
	if height < 1 {
		return 1
	}
	return height
}

func millerFooterHeight(footer string) int {
	if footer == "" {
		return 0
	}
	return strings.Count(footer, "\n") + 1
}

func millerColumnBodyHeight(totalHeight int, footer string) int {
	height := totalHeight - 1 - millerFooterHeight(footer)
	if height < 1 {
		return 1
	}
	return height
}

func RenderMillerListColumn(col MillerListColumn, styles Styles) string {
	body := ""
	if col.List != nil {
		// Body shrinks to make room for: title (1), optional sub-header
		// (N), optional footer (M). No internal horizontal rules — they
		// don't extend across parent spacer columns and produce broken
		// `│─` joints at the spacer/focused boundary.
		bodyHeight := millerColumnBodyHeight(col.Frame.Height, col.Footer)
		if col.SubHeader != "" {
			bodyHeight -= millerSubHeaderHeight(col.SubHeader)
		}
		if bodyHeight < 1 {
			bodyHeight = 1
		}
		col.List.SetSize(MillerColumnContentWidth(col.Frame), bodyHeight)
		body = col.List.View()
	}
	return RenderMillerColumn(MillerColumn{
		Title:     col.Title,
		TitleMeta: col.TitleMeta,
		SubHeader: col.SubHeader,
		Body:      body,
		Footer:    col.Footer,
		Frame:     col.Frame,
	}, styles)
}

func millerSubHeaderHeight(s string) int {
	if s == "" {
		return 0
	}
	return strings.Count(s, "\n") + 1
}

func RenderMillerColumn(col MillerColumn, styles Styles) string {
	frame := col.Frame
	if frame.Width <= 0 || frame.Height <= 0 {
		return ""
	}

	contentWidth := MillerColumnContentWidth(frame)
	bodyHeight := millerColumnBodyHeight(frame.Height, col.Footer)
	if col.SubHeader != "" {
		bodyHeight -= millerSubHeaderHeight(col.SubHeader)
	}
	if bodyHeight < 0 {
		bodyHeight = 0
	}

	rule := styles.Chrome.ColumnRule
	rows := make([]string, 0, frame.Height)

	titleStyle := styles.Chrome.ColumnTitle
	if frame.Focused {
		titleStyle = styles.Chrome.ColumnTitleFocus
	}
	titleLine := titleStyle.Render(col.Title)
	if col.TitleMeta != "" {
		metaStyle := styles.Chrome.ColumnFooter
		if frame.Focused {
			metaStyle = styles.Chrome.ColumnFooterFocus
		}
		titleLine = fitTitleLine(titleLine, metaStyle.Render(col.TitleMeta), contentWidth, titleStyle)
	} else {
		titleLine = fitMillerLine(titleLine, contentWidth, titleStyle)
	}
	rows = append(rows, titleLine)

	if col.SubHeader != "" {
		for _, line := range strings.Split(col.SubHeader, "\n") {
			rows = append(rows, fitMillerLine(line, contentWidth, lipgloss.NewStyle()))
		}
	}

	for _, line := range strings.Split(fitContent(col.Body, contentWidth, bodyHeight), "\n") {
		rows = append(rows, fitMillerLine(line, contentWidth, lipgloss.NewStyle()))
	}

	if col.Footer != "" {
		footerStyle := styles.Chrome.ColumnFooter
		if frame.Focused {
			footerStyle = styles.Chrome.ColumnFooterFocus
		}
		for _, line := range strings.Split(col.Footer, "\n") {
			rows = append(rows, fitMillerLine(footerStyle.Render(line), contentWidth, footerStyle))
		}
	}

	for len(rows) < frame.Height {
		rows = append(rows, fitMillerLine("", contentWidth, lipgloss.NewStyle()))
	}
	if len(rows) > frame.Height {
		rows = rows[:frame.Height]
	}

	if frame.RightRule {
		bar := rule.Render("│")
		for i := range rows {
			rows[i] += bar
		}
	}
	return strings.Join(rows, "\n")
}

// fitContent clips content to (innerW, innerH) cells, padding short
// lines with empty rows and truncating overlong rows so the rendered
// block fits exactly. Used by RenderMillerColumn to size the body
// area without spilling past the column.
func fitContent(content string, innerW, innerH int) string {
	if innerH <= 0 || innerW <= 0 {
		return ""
	}
	lines := strings.Split(content, "\n")
	if len(lines) > innerH {
		lines = lines[:innerH]
	}
	for i, line := range lines {
		if ansi.StringWidth(line) > innerW {
			lines[i] = ansi.Truncate(line, innerW, "")
		}
	}
	for len(lines) < innerH {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

func fitMillerLine(line string, width int, fill lipgloss.Style) string {
	if width <= 0 {
		return ""
	}
	if ansi.StringWidth(line) > width {
		return ansi.Truncate(line, width, "")
	}
	return line + fill.Render(strings.Repeat(" ", width-ansi.StringWidth(line)))
}

// RenderHorizontalRule produces a full-width thin horizontal rule using
// the column-rule style. Used between app sections (header, body,
// status) so the visual hierarchy stays continuous with the per-column
// rules. tickPositions, when non-nil, marks the x-coordinates where a
// vertical rule lives below the line — those cells get `┬` instead of
// `─` to form a clean tee. Pass nil for a plain rule.
func RenderHorizontalRule(width int, styles Styles, tickPositions []int) string {
	if width <= 0 {
		return ""
	}
	tick := make(map[int]bool, len(tickPositions))
	for _, p := range tickPositions {
		tick[p] = true
	}
	var b strings.Builder
	for i := 0; i < width; i++ {
		if tick[i] {
			b.WriteString("┬")
		} else {
			b.WriteString("─")
		}
	}
	return styles.Chrome.ColumnRule.Render(b.String())
}

// RenderHorizontalRuleBottom is the bottom-tee variant: `┴` at column
// boundaries (vertical rules end above, none below).
func RenderHorizontalRuleBottom(width int, styles Styles, tickPositions []int) string {
	if width <= 0 {
		return ""
	}
	tick := make(map[int]bool, len(tickPositions))
	for _, p := range tickPositions {
		tick[p] = true
	}
	var b strings.Builder
	for i := 0; i < width; i++ {
		if tick[i] {
			b.WriteString("┴")
		} else {
			b.WriteString("─")
		}
	}
	return styles.Chrome.ColumnRule.Render(b.String())
}

// fitTitleLine packs a left-aligned title and right-aligned meta into
// `width` cells. If the meta wouldn't fit alongside the title (less than
// one space gap), it's dropped so the title stays intact.
func fitTitleLine(left, right string, width int, fill lipgloss.Style) string {
	if width <= 0 {
		return ""
	}
	leftW := ansi.StringWidth(left)
	rightW := ansi.StringWidth(right)
	if leftW+rightW+1 > width {
		return fitMillerLine(left, width, fill)
	}
	gap := width - leftW - rightW
	return left + fill.Render(strings.Repeat(" ", gap)) + right
}
