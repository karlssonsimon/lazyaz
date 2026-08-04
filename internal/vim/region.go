package vim

// MotionKind classifies a motion for the operator grammar: an operator
// acting through a motion yanks a span whose shape depends on the kind,
// not just the target position.
type MotionKind int

const (
	// KindExclusive: the span stops before the target (w, b, 0, h, l,
	// F, T, and backward spans generally).
	KindExclusive MotionKind = iota
	// KindInclusive: the span includes the rune under the target
	// (e, E, f, t, $).
	KindInclusive
	// KindLinewise: whole lines (j, k, and the doubled yy).
	KindLinewise
)

// Region is an operator's span in cursor positions. End is exclusive
// and its Col may equal the line's rune length — the boundary before
// the newline. For linewise regions Start and End mark the first and
// last line; consumers widen to full lines.
type Region struct {
	Start    Cursor
	End      Cursor
	Linewise bool
}

// Empty reports a charwise region with nothing in it.
func (r Region) Empty() bool {
	return !r.Linewise && r.Start.Line == r.End.Line && r.Start.Col == r.End.Col
}

// RegionFor builds the span an operator acts on, given where the motion
// started, where it landed, and its kind.
//
// Vim's exclusive adjustment is applied: an exclusive span ending in
// column 0 of a later line retreats to the end of the previous line,
// which is exactly why yw on the last word of a line stops at the line
// end instead of eating the newline. The further becomes-linewise
// refinement vim layers on top is rare enough to skip.
func RegionFor(b TextBuffer, from, to Cursor, kind MotionKind) Region {
	from = normalize(b, from)
	to = normalize(b, to)

	if kind == KindLinewise {
		lo, hi := from.Line, to.Line
		if lo > hi {
			lo, hi = hi, lo
		}
		end := Cursor{Line: hi, Col: len(lineRunes(b, hi))}
		end.Want = end.Col
		return Region{Start: Cursor{Line: lo}, End: end, Linewise: true}
	}

	start, end := from, to
	if posAfter(start, end) {
		start, end = end, start
	}
	if kind == KindInclusive {
		end = boundaryAfter(b, end)
	}
	if kind == KindExclusive && end.Col == 0 && end.Line > start.Line {
		col := len(lineRunes(b, end.Line-1))
		end = Cursor{Line: end.Line - 1, Col: col, Want: col}
	}
	return Region{Start: start, End: end}
}

func posAfter(a, c Cursor) bool {
	return a.Line > c.Line || (a.Line == c.Line && a.Col > c.Col)
}

// boundaryAfter advances a position one rune, allowing Col to land on
// the line-length boundary (before the newline) rather than wrapping.
func boundaryAfter(b TextBuffer, c Cursor) Cursor {
	if c.Col < len(lineRunes(b, c.Line)) {
		c.Col++
		c.Want = c.Col
		return c
	}
	return c
}

// --- text objects ---

// QuoteObject resolves i"/a" and friends: line-scoped as in vim, pairs
// counted from the line start, taking the pair enclosing the cursor or
// the next one after it. A quote behind an odd run of backslashes is
// not a delimiter — JSON is the workload here. Around takes trailing
// whitespace, or leading when there is none.
func QuoteObject(b TextBuffer, c Cursor, quote rune, around bool) (Region, bool) {
	c = normalize(b, c)
	rs := lineRunes(b, c.Line)

	var qs []int
	esc := false
	for i, r := range rs {
		switch {
		case esc:
			esc = false
		case r == '\\':
			esc = true
		case r == quote:
			qs = append(qs, i)
		}
	}

	for k := 0; k+1 < len(qs); k += 2 {
		open, close := qs[k], qs[k+1]
		if c.Col > close {
			continue
		}
		if !around {
			return regionOnLine(c.Line, open+1, close), true
		}
		start, end := open, close+1
		e := end
		for e < len(rs) && classOf(rs[e]) == clsWS {
			e++
		}
		if e > end {
			end = e
		} else {
			for start > 0 && classOf(rs[start-1]) == clsWS {
				start--
			}
		}
		return regionOnLine(c.Line, start, end), true
	}
	return Region{}, false
}

// BracketObject resolves i(/a( and friends: a nesting-aware scan,
// backward for the unmatched opener and forward for its closer,
// cross-line but bounded by the buffer — an object reaching past the
// loaded window fails honestly rather than yanking a lie. The cursor
// may sit inside the pair or on either bracket.
func BracketObject(b TextBuffer, c Cursor, open, close rune, around bool) (Region, bool) {
	c = normalize(b, c)
	cp := pos{c.Line, c.Col}

	var opener, closer pos
	openerFound, closerFound := false, false

	switch runeAtPos(b, cp) {
	case open:
		opener, openerFound = cp, true
	case close:
		closer, closerFound = cp, true
	}

	if !openerFound {
		from := cp
		if closerFound {
			from = closer
		}
		depth := 0
		p := from
		for {
			pp, ok := prevPos(b, p)
			if !ok {
				return Region{}, false
			}
			p = pp
			switch runeAtPos(b, p) {
			case close:
				depth++
			case open:
				if depth == 0 {
					opener, openerFound = p, true
				} else {
					depth--
				}
			}
			if openerFound {
				break
			}
		}
	}

	if !closerFound {
		depth := 0
		p := opener
		for {
			np, ok := nextPos(b, p)
			if !ok {
				return Region{}, false
			}
			p = np
			switch runeAtPos(b, p) {
			case open:
				depth++
			case close:
				if depth == 0 {
					closer, closerFound = p, true
				} else {
					depth--
				}
			}
			if closerFound {
				break
			}
		}
	}

	// Containment: a balanced pair found behind the cursor does not
	// enclose it.
	if cp.line > closer.line || (cp.line == closer.line && cp.col > closer.col) {
		return Region{}, false
	}

	if around {
		start := Cursor{Line: opener.line, Col: opener.col, Want: opener.col}
		end := boundaryAfter(b, Cursor{Line: closer.line, Col: closer.col, Want: closer.col})
		return Region{Start: start, End: end}, true
	}
	start := boundaryAfter(b, Cursor{Line: opener.line, Col: opener.col, Want: opener.col})
	if opener.line != closer.line && start.Col >= len(lineRunes(b, start.Line)) && opener.col == len(lineRunes(b, opener.line))-1 {
		// Opener at the very end of its line: inner content starts at
		// the boundary, which byte-conversion places before the newline.
		start = Cursor{Line: opener.line, Col: opener.col + 1, Want: opener.col + 1}
	}
	end := Cursor{Line: closer.line, Col: closer.col, Want: closer.col}
	return Region{Start: start, End: end}, true
}

// WordObject resolves iw/aw and iW/aW: the class run under the cursor,
// with around taking trailing whitespace or leading when none trails.
func WordObject(b TextBuffer, c Cursor, big, around bool) (Region, bool) {
	c = normalize(b, c)
	rs := lineRunes(b, c.Line)
	if len(rs) == 0 {
		return Region{}, false
	}
	cls := classOf
	if big {
		cls = classOfBig
	}

	s := cls(rs[c.Col])
	start, end := c.Col, c.Col+1
	for start > 0 && cls(rs[start-1]) == s {
		start--
	}
	for end < len(rs) && cls(rs[end]) == s {
		end++
	}

	if around {
		e := end
		for e < len(rs) && cls(rs[e]) == clsWS {
			e++
		}
		if e > end {
			end = e
		} else {
			for start > 0 && cls(rs[start-1]) == clsWS {
				start--
			}
		}
	}
	return regionOnLine(c.Line, start, end), true
}

func regionOnLine(line, start, end int) Region {
	return Region{
		Start: Cursor{Line: line, Col: start, Want: start},
		End:   Cursor{Line: line, Col: end, Want: end},
	}
}

func runeAtPos(b TextBuffer, p pos) rune {
	rs := lineRunes(b, p.line)
	if p.col < 0 || p.col >= len(rs) {
		return 0
	}
	return rs[p.col]
}

// tagTok is one <...> occurrence found by the tag scanner.
type tagTok struct {
	name       string
	closing    bool
	start, end pos // the < and the >
}

// TagObject resolves it/at: the content between an opening tag and its
// matching close, vim's tag object. Matching is a name-aware stack, so
// nested same-name tags resolve to the innermost pair containing the
// cursor — which may sit in the content or inside either tag. Tags
// themselves must fit on one line (attributes included); the content
// may span lines, bounded by the loaded window like the bracket
// objects. Self-closing tags, comments and declarations are skipped.
func TagObject(b TextBuffer, c Cursor, around bool) (Region, bool) {
	c = normalize(b, c)
	cp := pos{c.Line, c.Col}

	var toks []tagTok
	for line := 0; line < b.LineCount(); line++ {
		rs := lineRunes(b, line)
		for i := 0; i < len(rs); i++ {
			if rs[i] != '<' {
				continue
			}
			j := i + 1
			for j < len(rs) && rs[j] != '>' {
				j++
			}
			if j >= len(rs) {
				break // tag not closed on this line — unsupported
			}
			inner := rs[i+1 : j]
			tok := tagTok{start: pos{line, i}, end: pos{line, j}}
			if len(inner) > 0 && inner[0] == '/' {
				tok.closing = true
				inner = inner[1:]
			}
			selfClosing := len(inner) > 0 && inner[len(inner)-1] == '/'
			nameEnd := 0
			for nameEnd < len(inner) && inner[nameEnd] != ' ' && inner[nameEnd] != '/' {
				nameEnd++
			}
			tok.name = string(inner[:nameEnd])
			i = j
			if tok.name == "" || tok.name[0] == '!' || tok.name[0] == '?' || selfClosing {
				continue
			}
			toks = append(toks, tok)
		}
	}

	// Stack-match pairs; the best is the innermost whose full extent
	// contains the cursor.
	type openTag struct {
		tok tagTok
	}
	var stack []openTag
	var best *[2]tagTok
	for _, tok := range toks {
		if !tok.closing {
			stack = append(stack, openTag{tok})
			continue
		}
		for len(stack) > 0 {
			top := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if top.tok.name != tok.name {
				continue // unclosed inner tag — drop it and keep looking
			}
			containsCursor := !posBefore(cp, top.tok.start) && !posBefore(tok.end, cp)
			if containsCursor && (best == nil || posBefore(best[0].start, top.tok.start)) {
				pair := [2]tagTok{top.tok, tok}
				best = &pair
			}
			break
		}
	}
	if best == nil {
		return Region{}, false
	}

	open, closeTag := best[0], best[1]
	if around {
		start := Cursor{Line: open.start.line, Col: open.start.col, Want: open.start.col}
		end := boundaryAfter(b, Cursor{Line: closeTag.end.line, Col: closeTag.end.col, Want: closeTag.end.col})
		return Region{Start: start, End: end}, true
	}
	start := boundaryAfter(b, Cursor{Line: open.end.line, Col: open.end.col, Want: open.end.col})
	end := Cursor{Line: closeTag.start.line, Col: closeTag.start.col, Want: closeTag.start.col}
	return Region{Start: start, End: end}, true
}

func posBefore(a, b pos) bool {
	return a.line < b.line || (a.line == b.line && a.col < b.col)
}
