package ui

import (
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/lipgloss"
)

// PaneTitleHeight is the vertical space the externally-rendered pane
// title row occupies inside a pane (see RenderListPane). Apps must
// subtract this from the list height they hand to SetSize so the title
// row fits inside the pane's fixed height.
const PaneTitleHeight = 1

// PaneFrame describes the outer dimensions and focus state of a pane.
// The pane style itself (border, padding, focus color) is pulled from
// Styles.Chrome — PaneFrame only carries per-pane values that vary each
// frame.
type PaneFrame struct {
	Width   int
	Height  int
	Focused bool
}

// RenderPane wraps pre-composed content in the pane chrome (border,
// focus-aware color, fixed dimensions). Use this directly for panes
// whose content doesn't fit the standard list+hints layout — for
// example blobapp's preview pane, which composes a title line over a
// viewport.
func RenderPane(content string, frame PaneFrame, styles Styles) string {
	style := styles.Chrome.Pane
	if frame.Focused {
		style = styles.Chrome.FocusedPane
	}
	return style.Copy().Width(frame.Width).Height(frame.Height).Render(content)
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
func RenderListPane(p ListPane, styles Styles) string {
	contentWidth := PaneContentWidth(styles.Chrome.Pane, p.Frame.Width)

	titleText := RenderPaneSpinner(p.Title, p.Loading, p.LoadedAt, styles, contentWidth)
	titleStyle := styles.List.Title
	if p.TitleStyle != nil {
		titleStyle = *p.TitleStyle
	}
	title := titleStyle.Render(titleText)

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

	paneStyle := styles.Chrome.Pane
	if p.Frame.Focused {
		paneStyle = styles.Chrome.FocusedPane
	}
	if p.FrameStyle != nil {
		paneStyle = *p.FrameStyle
	}
	return paneStyle.Copy().Width(p.Frame.Width).Height(p.Frame.Height).Render(content)
}
