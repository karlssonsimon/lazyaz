package ui

import (
	icolor "image/color"

	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
)

// MillerColumns holds the width for each role in a miller columns layout.
type MillerColumns struct {
	Parent  int // left column (parent pane) — 0 if no parent
	Focused int // center column (focused pane) — always > 0
	Child   int // right column (child preview) — 0 if no child
}

// Per-column layout bounds. Mins aim for "comfortable" minimum widths
// for the kinds of names Azure resources actually have. Side columns
// (parent/child) have NO max — they grow proportionally with the
// terminal so resource names of any length can fit when they happen
// to be in a side column. Only the focused column has a max so it
// doesn't balloon to absurd widths on ultrawide displays.
const (
	millerParentMin  = 32
	millerFocusedMin = 50
	millerFocusedMax = 200
	millerChildMin   = 32
)

// MillerLayout computes widths for a yazi-style miller columns layout.
// When `hasParent` is true the layout reserves three slots in 20/60/20
// proportions; when false (focus is on the leftmost column) the parent
// slot collapses and the focused column absorbs that width — there's
// no useful parent context to show, no reason to leave a dead 20%.
// Side columns still respect their min/max caps; leftover width on
// ultrawide terminals is left as centering margin.
//
// `hasChild` is currently ignored (the layout always reserves a child
// slot for stability as the user drills), but kept in the signature
// for future use.
//
// Collapse strategy on narrow terminals: drop parent first, then child;
// dropped columns are reported as width 0.
func MillerLayout(paneStyle lipgloss.Style, totalWidth int, hasParent, hasChild bool) MillerColumns {
	_ = paneStyle
	_ = hasChild
	if totalWidth <= 0 {
		return MillerColumns{}
	}

	clamp := func(v, lo, hi int) int {
		if v < lo {
			return lo
		}
		if v > hi {
			return hi
		}
		return v
	}

	// Side columns (parent/child) get whatever's left after the focus
	// column hits its max — that way they grow with the terminal and
	// can hold long resource names when one happens to land in a side
	// slot. Only the focused column has a hard max.
	maxAtLeast := func(v, lo int) int {
		if v < lo {
			return lo
		}
		return v
	}
	switch {
	case hasParent && totalWidth >= millerParentMin+millerFocusedMin+millerChildMin:
		focusedW := clamp(totalWidth*60/100, millerFocusedMin, millerFocusedMax)
		remaining := totalWidth - focusedW
		// Split remaining 50/50 between parent and child, ensuring
		// each meets its minimum.
		parentW := maxAtLeast(remaining/2, millerParentMin)
		childW := maxAtLeast(remaining-parentW, millerChildMin)
		return MillerColumns{Parent: parentW, Focused: focusedW, Child: childW}
	case totalWidth >= millerFocusedMin+millerChildMin:
		// No parent or too narrow for three columns — focus absorbs
		// the parent slot, child gets the rest.
		focusedW := clamp(totalWidth*80/100, millerFocusedMin, millerFocusedMax)
		childW := maxAtLeast(totalWidth-focusedW, millerChildMin)
		return MillerColumns{Focused: focusedW, Child: childW}
	default:
		return MillerColumns{Focused: totalWidth}
	}
}

// MillerSideMargin reports the left margin width needed to center the
// rendered column block when the bounded layout doesn't consume the
// whole terminal width. Returns 0 when the columns already fill the
// width or the leftover is negligible.
func MillerSideMargin(cols MillerColumns, totalWidth int) int {
	used := cols.Parent + cols.Focused + cols.Child
	if used >= totalWidth {
		return 0
	}
	return (totalWidth - used) / 2
}

func MillerContentWidth(frame MillerColumnFrame) int {
	return MillerColumnContentWidth(frame)
}

func AppBodyHeight(totalHeight int) int {
	// 1 header + 1 rule + body + 1 rule + 1 status
	height := totalHeight - AppHeaderHeight - StatusBarHeight - 2
	if height < 1 {
		return 1
	}
	return height
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
