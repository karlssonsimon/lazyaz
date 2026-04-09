package ui

import (
	"strings"
	"time"

	"charm.land/bubbles/v2/list"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// PaneTitleHeight is the vertical space the externally-rendered pane
// title row occupies inside a pane (see RenderListPane). Apps must
// subtract this from the list height they hand to SetSize so the title
// row fits inside the pane's fixed height.
const PaneTitleHeight = 1

// PaneFrame describes the outer dimensions and focus state of a pane.
// Frame.Height is the **total block height** including the pane's
// border and padding — i.e. the number of terminal rows the rendered
// pane occupies. Likewise Frame.Width is the total block width.
//
// The pane style itself (border, padding, focus color) is pulled from
// Styles.Chrome — PaneFrame only carries per-pane values that vary each
// frame.
type PaneFrame struct {
	Width   int
	Height  int
	Focused bool
}

// PaneInnerHeight returns the number of content rows available inside
// a pane of the given total block height, after subtracting the pane
// style's border and vertical padding. Use this when sizing list bodies
// or viewports that live inside a pane.
func PaneInnerHeight(paneStyle lipgloss.Style, totalHeight int) int {
	inner := totalHeight - paneStyle.GetVerticalFrameSize()
	if inner < 0 {
		return 0
	}
	return inner
}

// fitContent clips and pads content to exactly innerH rows of innerW
// columns each. We do this **before** handing content to lipgloss so
// the border is preserved and the resulting block size is deterministic.
//
// Why not just use lipgloss Style.MaxWidth/MaxHeight? Because for a
// bordered block, MaxHeight clips from the bottom of the rendered
// output — which removes the bottom border when inner content
// overflows. The visible result is a frameless pane with content
// spilling past where the border should sit. Pre-clipping the content
// keeps the border intact.
//
// Wide lines are truncated horizontally with [ansi.Truncate] so they
// don't get wrapped by lipgloss into extra rows.
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

// paneInnerSize returns the (innerWidth, innerHeight) that fits inside
// a pane of the given total block size.
func paneInnerSize(style lipgloss.Style, frame PaneFrame) (int, int) {
	w := frame.Width - style.GetHorizontalFrameSize()
	h := frame.Height - style.GetVerticalFrameSize()
	if w < 0 {
		w = 0
	}
	if h < 0 {
		h = 0
	}
	return w, h
}

// RenderPane wraps pre-composed content in the pane chrome (border,
// focus-aware color, fixed dimensions). The rendered block is exactly
// frame.Height rows tall and frame.Width columns wide regardless of
// content shape — overflow is clipped, underflow is padded. Use this
// directly for panes whose content doesn't fit the standard list+hints
// layout — for example blobapp's preview pane, which composes a title
// line over a viewport.
func RenderPane(content string, frame PaneFrame, styles Styles) string {
	style := styles.Chrome.Pane
	if frame.Focused {
		style = styles.Chrome.FocusedPane
	}
	innerW, innerH := paneInnerSize(style, frame)
	return style.Width(frame.Width).Render(fitContent(content, innerW, innerH))
}

// ListPane describes a standard list-backed pane: a title row (with an
// optional loading spinner), the bubbles list body, and a hints row
// along the bottom. Apps construct one per frame in View() and pass it
// to [RenderListPane].
//
// Header is optional; when non-empty it is rendered as the very first
// row of pane content, above the title. Use this for tab strips or
// section selectors that conceptually own the pane (e.g. sbapp's
// active/DLQ tabs). The caller is responsible for making the list
// height accommodate the header — i.e. subtract its rendered height
// from the list size in resize().
//
// Prefix is optional; when non-empty it is rendered between the title
// and the list body. blobapp uses this to slot its search input above
// the blobs list when search is active.
//
// Footer is optional; when non-empty it is rendered between the list
// body and the hints row. Apps use this to pin auxiliary content (such
// as the per-pane inspect strip) inside a pane. The caller is responsible
// for making the list height accommodate the footer — i.e. subtract
// lipgloss.Height(footer) from the list height in resize().
//
// TitleStyle and FrameStyle are optional overrides for panes that need
// different title/border styling than the scheme defaults — e.g. sbapp's
// dead-letter detail pane wants a danger-colored title and border
// regardless of focus. Leave nil to use styles.List.Title and the usual
// focus-aware Chrome.Pane/FocusedPane selection.
//
// The List field is read-only from RenderListPane's perspective: we do
// not mutate list.Model.Title anymore. Callers must disable the bubbles
// list's own title rendering via list.SetShowTitle(false) at construction
// time so our externally-rendered title is the only one on screen.
type ListPane struct {
	List     *list.Model
	Title    string
	Loading  bool
	LoadedAt time.Time
	Hints    []PaneHint
	Header   string
	Prefix   string
	Footer   string
	Frame    PaneFrame

	TitleStyle *lipgloss.Style
	FrameStyle *lipgloss.Style
}

// RenderListPane composes and renders a standard list-backed pane.
// The rendered block is exactly p.Frame.Height rows tall regardless
// of how many items the list has — content is clipped/padded as needed.
func RenderListPane(p ListPane, styles Styles) string {
	paneStyle := styles.Chrome.Pane
	if p.Frame.Focused {
		paneStyle = styles.Chrome.FocusedPane
	}
	if p.FrameStyle != nil {
		paneStyle = *p.FrameStyle
	}

	contentWidth := PaneContentWidth(styles.Chrome.Pane, p.Frame.Width)
	innerW, innerH := paneInnerSize(paneStyle, p.Frame)

	titleText := RenderPaneSpinner(p.Title, p.Loading, p.LoadedAt, styles, contentWidth)
	titleStyle := styles.List.Title
	if p.TitleStyle != nil {
		titleStyle = *p.TitleStyle
	}
	title := titleStyle.MaxWidth(innerW).Render(titleText)
	hints := RenderPaneHints(p.Hints, styles, contentWidth)

	parts := make([]string, 0, 6)
	if p.Header != "" {
		parts = append(parts, p.Header)
	}
	parts = append(parts, title)
	if p.Prefix != "" {
		parts = append(parts, p.Prefix)
	}
	parts = append(parts, p.List.View())
	if p.Footer != "" {
		parts = append(parts, p.Footer)
	}
	parts = append(parts, hints)
	content := lipgloss.JoinVertical(lipgloss.Left, parts...)

	return paneStyle.Width(p.Frame.Width).Render(fitContent(content, innerW, innerH))
}
