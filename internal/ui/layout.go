package ui

import (
	icolor "image/color"

	"github.com/charmbracelet/lipgloss"
	uv "github.com/charmbracelet/ultraviolet"
	lg2 "charm.land/lipgloss/v2"
)

// PaneLayout computes widths for a set of panes that fill the available width.
// It uses the Pane style to determine frame overhead (border + padding) and
// accounts for no margins — borders provide visual separation.
//
// The returned slice has one entry per pane: the Width value to set on the
// pane style (which includes padding but excludes border). The list content
// inside each pane should be sized to paneWidth - horizontalPadding.
func PaneLayout(paneStyle lipgloss.Style, totalWidth, numPanes int) []int {
	if numPanes <= 0 {
		return nil
	}

	pad := paneStyle.GetHorizontalPadding()
	border := paneStyle.GetHorizontalBorderSize()

	// Total rendered width per pane = Width (set value) + border.
	// numPanes * (W + border) = totalWidth
	// => W = (totalWidth - numPanes*border) / numPanes
	totalForPanes := totalWidth
	perPane := totalForPanes / numPanes
	remainder := totalForPanes % numPanes

	w := perPane - border

	widths := make([]int, numPanes)
	for i := range widths {
		widths[i] = w
	}
	widths[numPanes-1] += remainder

	for i := range widths {
		if widths[i]-pad < 1 {
			widths[i] = pad + 1
		}
	}

	return widths
}

// PaneContentWidth returns the list content width for a pane with the given
// style Width value. This is simply Width - horizontal padding.
func PaneContentWidth(paneStyle lipgloss.Style, styleWidth int) int {
	return styleWidth - paneStyle.GetHorizontalPadding()
}

// RenderCanvas takes rendered content (with ANSI codes) and draws it onto a
// cell-buffer canvas. Any cell that doesn't have an explicit background color
// gets filled with the given bg color. This solves the problem of ANSI resets
// (\e[0m) cancelling parent backgrounds — each cell is independent.
func RenderCanvas(content string, width, height int, bg icolor.Color) string {
	if width <= 0 || height <= 0 {
		return content
	}

	canvas := lg2.NewCanvas(width, height)
	layer := lg2.NewLayer(content)
	canvas.Compose(layer)

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			cell := canvas.CellAt(x, y)
			if cell == nil {
				canvas.SetCell(x, y, &uv.Cell{
					Content: " ",
					Width:   1,
					Style:   uv.Style{Bg: bg},
				})
			} else if cell.Style.Bg == nil {
				cell.Style.Bg = bg
			}
		}
	}

	return canvas.Render()
}
