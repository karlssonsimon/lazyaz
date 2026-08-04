package vim

import "unicode"

// TextBuffer is the read-only view a buffer surface gives the motion
// engine: raw text as lines, no ANSI, no knowledge of where the bytes
// came from. The blob preview implements it over its loaded window; the
// message body can implement it over one string.
type TextBuffer interface {
	LineCount() int
	Line(i int) string
}

// Cursor is a buffer position. Col is a rune index within the line —
// display columns exist only at render time. Want is vim's goal column:
// j through a short line and back restores it, and WantEnd makes $
// sticky so j after $ rides line ends.
type Cursor struct {
	Line int
	Col  int
	Want int
}

// WantEnd is the sticky end-of-line sentinel for Want, set by $.
const WantEnd = -1

// --- character classes: vim's three, plus empty lines as word stops ---

const (
	clsWS = iota
	clsWord
	clsPunct
	clsEmpty
)

func classOf(r rune) int {
	switch {
	case unicode.IsSpace(r):
		return clsWS
	case r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r):
		return clsWord
	default:
		return clsPunct
	}
}

// --- position walking over the buffer ---

type pos struct{ line, col int }

func lineRunes(b TextBuffer, line int) []rune {
	return []rune(b.Line(line))
}

func lastCol(b TextBuffer, line int) int {
	n := len(lineRunes(b, line)) - 1
	if n < 0 {
		return 0
	}
	return n
}

func classAt(b TextBuffer, p pos) int {
	rs := lineRunes(b, p.line)
	if len(rs) == 0 {
		return clsEmpty
	}
	return classOf(rs[p.col])
}

func nextPos(b TextBuffer, p pos) (pos, bool) {
	if p.col+1 < len(lineRunes(b, p.line)) {
		return pos{p.line, p.col + 1}, true
	}
	if p.line+1 >= b.LineCount() {
		return p, false
	}
	return pos{p.line + 1, 0}, true
}

func prevPos(b TextBuffer, p pos) (pos, bool) {
	if p.col > 0 {
		return pos{p.line, p.col - 1}, true
	}
	if p.line == 0 {
		return p, false
	}
	return pos{p.line - 1, lastCol(b, p.line-1)}, true
}

// normalize clamps a cursor into the buffer so every motion tolerates
// out-of-range input.
func normalize(b TextBuffer, c Cursor) Cursor {
	if b.LineCount() == 0 {
		return Cursor{Want: c.Want}
	}
	c.Line = clampInt(c.Line, 0, b.LineCount()-1)
	c.Col = clampInt(c.Col, 0, lastCol(b, c.Line))
	return c
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// resolveWant places the column on a line according to the goal column.
func resolveWant(b TextBuffer, line, want int) int {
	if want == WantEnd {
		return lastCol(b, line)
	}
	return clampInt(want, 0, lastCol(b, line))
}

// --- motions: pure functions (TextBuffer, Cursor, count) → Cursor ---

// MoveLeft is h. It does not wrap lines, as in vim.
func MoveLeft(b TextBuffer, c Cursor, n int) Cursor {
	c = normalize(b, c)
	c.Col = clampInt(c.Col-n, 0, lastCol(b, c.Line))
	c.Want = c.Col
	return c
}

// MoveRight is l, clamped to the last character (normal-mode rule).
func MoveRight(b TextBuffer, c Cursor, n int) Cursor {
	c = normalize(b, c)
	c.Col = clampInt(c.Col+n, 0, lastCol(b, c.Line))
	c.Want = c.Col
	return c
}

// MoveDown is j: the goal column carries across lines.
func MoveDown(b TextBuffer, c Cursor, n int) Cursor {
	c = normalize(b, c)
	c.Line = clampInt(c.Line+n, 0, b.LineCount()-1)
	c.Col = resolveWant(b, c.Line, c.Want)
	return c
}

// MoveUp is k.
func MoveUp(b TextBuffer, c Cursor, n int) Cursor {
	return MoveDown(b, c, -n)
}

// LineStart is 0.
func LineStart(c Cursor) Cursor {
	c.Col = 0
	c.Want = 0
	return c
}

// LineEnd is $. A count goes count-1 lines down first, and Want becomes
// sticky so j keeps riding line ends.
func LineEnd(b TextBuffer, c Cursor, n int) Cursor {
	c = normalize(b, c)
	c.Line = clampInt(c.Line+n-1, 0, b.LineCount()-1)
	c.Col = lastCol(b, c.Line)
	c.Want = WantEnd
	return c
}

// WordForward is w: to the start of the next word, where a punctuation
// run is a word of its own and an empty line is a stop.
func WordForward(b TextBuffer, c Cursor, n int) Cursor {
	c = normalize(b, c)
	p := pos{c.Line, c.Col}
	for i := 0; i < n; i++ {
		if s := classAt(b, p); s == clsWord || s == clsPunct {
			for {
				np, ok := nextPos(b, p)
				if !ok {
					return cursorAt(p)
				}
				p = np
				if classAt(b, p) != s {
					break
				}
			}
		} else {
			np, ok := nextPos(b, p)
			if !ok {
				break
			}
			p = np
		}
		for classAt(b, p) == clsWS {
			np, ok := nextPos(b, p)
			if !ok {
				return cursorAt(p)
			}
			p = np
		}
	}
	return cursorAt(p)
}

// WordBack is b: to the start of the previous word; empty lines stop it.
func WordBack(b TextBuffer, c Cursor, n int) Cursor {
	c = normalize(b, c)
	p := pos{c.Line, c.Col}
	for i := 0; i < n; i++ {
		pp, ok := prevPos(b, p)
		if !ok {
			break
		}
		p = pp
		for classAt(b, p) == clsWS {
			pp, ok := prevPos(b, p)
			if !ok {
				return cursorAt(p)
			}
			p = pp
		}
		if classAt(b, p) == clsEmpty {
			continue
		}
		s := classAt(b, p)
		for {
			pp, ok := prevPos(b, p)
			if !ok || classAt(b, pp) != s {
				break
			}
			p = pp
		}
	}
	return cursorAt(p)
}

// WordEnd is e: to the end of the next word; empty lines are skipped.
func WordEnd(b TextBuffer, c Cursor, n int) Cursor {
	c = normalize(b, c)
	p := pos{c.Line, c.Col}
	for i := 0; i < n; i++ {
		np, ok := nextPos(b, p)
		if !ok {
			break
		}
		p = np
		for classAt(b, p) == clsWS || classAt(b, p) == clsEmpty {
			np, ok := nextPos(b, p)
			if !ok {
				return cursorAt(p)
			}
			p = np
		}
		s := classAt(b, p)
		for {
			np, ok := nextPos(b, p)
			if !ok || classAt(b, np) != s {
				break
			}
			p = np
		}
	}
	return cursorAt(p)
}

// FindOnLine is the f/F/t/T family, on the cursor's line only. A till
// that would land on the cursor fails outright, as in vim — it does not
// skip to the next occurrence (that is `;`'s special case).
func FindOnLine(b TextBuffer, c Cursor, target rune, till, back bool, n int) (Cursor, bool) {
	c = normalize(b, c)
	rs := lineRunes(b, c.Line)

	land := -1
	count := 0
	if back {
		for i := c.Col - 1; i >= 0; i-- {
			if rs[i] == target {
				count++
				if count == n {
					land = i
					if till {
						land = i + 1
					}
					break
				}
			}
		}
	} else {
		for i := c.Col + 1; i < len(rs); i++ {
			if rs[i] == target {
				count++
				if count == n {
					land = i
					if till {
						land = i - 1
					}
					break
				}
			}
		}
	}
	if land < 0 || land == c.Col {
		return c, false
	}
	c.Col = land
	c.Want = land
	return c, true
}

func cursorAt(p pos) Cursor {
	return Cursor{Line: p.line, Col: p.col, Want: p.col}
}
