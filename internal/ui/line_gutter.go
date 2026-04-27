package ui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/viewport"
)

// LineGutterWidth reports the cells a line-number gutter occupies for
// a viewport whose deepest line number is `totalLines`. minDigits
// reserves a minimum number-column width so the gutter doesn't reflow
// as you scroll into wider line-number ranges. The total covers the
// digits plus a trailing " │ " separator.
func LineGutterWidth(totalLines, minDigits int) int {
	w := digits(totalLines)
	if w < minDigits {
		w = minDigits
	}
	return w + 3 // space + │ + space
}

// RenderLineGutter returns a multi-line gutter string sized to the
// viewport's visible region. JoinHorizontal this with vp.View() to
// place line numbers to the left of the content. The viewport's own
// LeftGutterFunc must be unset (NoGutter) or the numbers will double up.
//
// The gutter is intentionally rendered outside the viewport so text
// selection (which operates on vp.View()) doesn't include numbers in
// the copied text. Mouse-region X for selection should be offset by
// LineGutterWidth(totalLines, minDigits).
func RenderLineGutter(vp viewport.Model, styles Styles, minDigits int) string {
	if vp.Height() <= 0 {
		return ""
	}
	if minDigits < 1 {
		minDigits = 1
	}

	totalLines := vp.TotalLineCount()
	width := digits(totalLines)
	if width < minDigits {
		width = minDigits
	}

	ruleStyle := styles.Chrome.ColumnRule
	numStyle := styles.Chrome.ColumnFooter
	separator := ruleStyle.Render("│ ")
	blank := strings.Repeat(" ", width) + " " + separator

	startLine := vp.YOffset() + 1
	rows := make([]string, 0, vp.Height())
	for i := 0; i < vp.Height(); i++ {
		num := startLine + i
		if num <= totalLines {
			rows = append(rows, numStyle.Render(fmt.Sprintf("%*d", width, num))+" "+separator)
		} else {
			// Past content end — render blank gutter cell so the
			// column edge stays aligned even on padding rows.
			rows = append(rows, blank)
		}
	}
	return strings.Join(rows, "\n")
}

func digits(n int) int {
	if n <= 0 {
		return 1
	}
	d := 0
	for n > 0 {
		d++
		n /= 10
	}
	return d
}
