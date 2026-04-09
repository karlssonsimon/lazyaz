package ui

import (
	icolor "image/color"

	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
)

// PaneLayout computes widths for a set of panes that fill the available
// width. The returned slice contains the **total block width** for each
// pane (border + padding + content area), to be passed directly as
// PaneFrame.Width / Style.Width. The widths sum to totalWidth so the
// pane row fills the screen edge to edge — borders provide visual
// separation, no margins are inserted.
//
// In v2 lipgloss, Style.Width sets the total block size including the
// border. PaneLayout therefore returns total widths, not content-area
// widths as it did in v1.
func PaneLayout(paneStyle lipgloss.Style, totalWidth, numPanes int) []int {
	if numPanes <= 0 {
		return nil
	}

	frame := paneStyle.GetHorizontalFrameSize() // border + horizontal padding
	minTotal := frame + 1                       // need at least 1 cell of content
	perPane := totalWidth / numPanes
	remainder := totalWidth % numPanes

	widths := make([]int, numPanes)
	for i := range widths {
		widths[i] = perPane
	}
	widths[numPanes-1] += remainder

	for i := range widths {
		if widths[i] < minTotal {
			widths[i] = minTotal
		}
	}

	return widths
}

// MillerColumns holds the width for each role in a miller columns layout.
type MillerColumns struct {
	Parent  int // left column (parent pane) — 0 if no parent
	Focused int // center column (focused pane) — always > 0
	Child   int // right column (child preview) — 0 if no child
}

// MillerLayout computes widths for a yazi-style miller columns layout.
// The focused pane gets ~60% of the width, the parent gets ~20% on
// the left, and the child preview gets ~20% on the right. When there's
// no parent or no child, the focused pane absorbs the extra space.
//
// hasParent: whether a parent column should be shown (focus > 0).
// hasChild: whether a child preview column should be shown.
func MillerLayout(paneStyle lipgloss.Style, totalWidth int, hasParent, hasChild bool) MillerColumns {
	frame := paneStyle.GetHorizontalFrameSize()
	minWidth := frame + 4

	var parentW, focusedW, childW int
	switch {
	case hasParent && hasChild:
		parentW = totalWidth * 20 / 100
		childW = totalWidth * 20 / 100
		focusedW = totalWidth - parentW - childW
	case hasParent && !hasChild:
		parentW = totalWidth * 20 / 100
		focusedW = totalWidth - parentW
	case !hasParent && hasChild:
		childW = totalWidth * 20 / 100
		focusedW = totalWidth - childW
	default:
		focusedW = totalWidth
	}

	if parentW > 0 && parentW < minWidth {
		parentW = minWidth
		focusedW = totalWidth - parentW - childW
	}
	if childW > 0 && childW < minWidth {
		childW = minWidth
		focusedW = totalWidth - parentW - childW
	}
	if focusedW < minWidth {
		focusedW = minWidth
	}

	return MillerColumns{Parent: parentW, Focused: focusedW, Child: childW}
}

// PaneContentWidth returns the inner content width available inside a
// pane of the given total block width (border + horizontal padding
// subtracted). Use this to size list bodies, viewports, hint bars, etc.
func PaneContentWidth(paneStyle lipgloss.Style, styleWidth int) int {
	inner := styleWidth - paneStyle.GetHorizontalFrameSize()
	if inner < 0 {
		return 0
	}
	return inner
}

// RenderCanvas takes rendered content (with ANSI codes) and draws it onto a
// cell-buffer canvas. Any cell that doesn't have an explicit background color
// gets filled with the given bg color. This solves the problem of ANSI resets
// (\e[0m) cancelling parent backgrounds — each cell is independent.
func RenderCanvas(content string, width, height int, bg icolor.Color) string {
	if width <= 0 || height <= 0 {
		return content
	}

	canvas := lipgloss.NewCanvas(width, height)
	layer := lipgloss.NewLayer(content)
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
