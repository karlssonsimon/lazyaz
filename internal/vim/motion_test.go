package vim

import (
	"testing"

	"github.com/karlssonsimon/lazyaz/internal/keymap"
)

// sliceBuffer is the trivial TextBuffer for tests.
type sliceBuffer []string

func (b sliceBuffer) LineCount() int    { return len(b) }
func (b sliceBuffer) Line(i int) string { return b[i] }

func at(line, col int) Cursor { return Cursor{Line: line, Col: col, Want: col} }

func TestHorizontalMotions(t *testing.T) {
	b := sliceBuffer{"abc def", ""}

	tests := []struct {
		name string
		got  Cursor
		want Cursor
	}{
		{"l moves right", MoveRight(b, at(0, 0), 1), at(0, 1)},
		{"3l", MoveRight(b, at(0, 0), 3), at(0, 3)},
		{"l clamps at last char", MoveRight(b, at(0, 6), 5), at(0, 6)},
		{"h moves left", MoveLeft(b, at(0, 3), 1), at(0, 2)},
		{"h clamps at zero", MoveLeft(b, at(0, 1), 9), at(0, 0)},
		{"h does not wrap lines", MoveLeft(b, at(1, 0), 1), at(1, 0)},
		{"0 to line start", LineStart(at(0, 5)), at(0, 0)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got.Line != tt.want.Line || tt.got.Col != tt.want.Col {
				t.Errorf("got (%d,%d), want (%d,%d)", tt.got.Line, tt.got.Col, tt.want.Line, tt.want.Col)
			}
		})
	}
}

func TestVerticalMotionsKeepWantColumn(t *testing.T) {
	b := sliceBuffer{"a long first line", "ab", "another long line here"}

	c := at(0, 10)
	c = MoveDown(b, c, 1)
	if c.Line != 1 || c.Col != 1 {
		t.Fatalf("j onto short line: (%d,%d), want (1,1) clamped to last char", c.Line, c.Col)
	}
	c = MoveDown(b, c, 1)
	if c.Line != 2 || c.Col != 10 {
		t.Fatalf("j back onto long line: (%d,%d), want (2,10) — the goal column was lost", c.Line, c.Col)
	}

	// Counted and clamped at the edges.
	c = MoveUp(b, c, 99)
	if c.Line != 0 {
		t.Fatalf("99k landed on line %d, want 0", c.Line)
	}
	if got := MoveDown(b, at(0, 0), 99); got.Line != 2 {
		t.Fatalf("99j landed on line %d, want 2", got.Line)
	}
}

func TestLineEndIsSticky(t *testing.T) {
	b := sliceBuffer{"short", "a much longer line", "mid"}

	c := LineEnd(b, at(0, 0), 1)
	if c.Col != 4 {
		t.Fatalf("$ on line 0: col %d, want 4", c.Col)
	}
	c = MoveDown(b, c, 1)
	if c.Col != 17 {
		t.Fatalf("j after $: col %d, want 17 (sticky end)", c.Col)
	}
	c = MoveDown(b, c, 1)
	if c.Col != 2 {
		t.Fatalf("j again: col %d, want 2 (still riding line ends)", c.Col)
	}

	// [count]$ goes count-1 lines down, to that line's end.
	c = LineEnd(b, at(0, 0), 2)
	if c.Line != 1 || c.Col != 17 {
		t.Fatalf("2$: (%d,%d), want (1,17)", c.Line, c.Col)
	}
}

func TestWordForward(t *testing.T) {
	//            0123456789012345
	b := sliceBuffer{"foo bar.baz  qux", "", "next"}

	tests := []struct {
		name  string
		start Cursor
		n     int
		want  Cursor
	}{
		{"w to next word", at(0, 0), 1, at(0, 4)},
		{"w stops at punctuation", at(0, 4), 1, at(0, 7)},
		{"w from punct to word", at(0, 7), 1, at(0, 8)},
		{"w over double space", at(0, 8), 1, at(0, 13)},
		{"2w", at(0, 0), 2, at(0, 7)},
		{"w stops on empty line", at(0, 13), 1, at(1, 0)},
		{"w from empty line", at(1, 0), 1, at(2, 0)},
		{"w clamps at buffer end", at(2, 0), 9, at(2, 3)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := WordForward(b, tt.start, tt.n)
			if got.Line != tt.want.Line || got.Col != tt.want.Col {
				t.Errorf("got (%d,%d), want (%d,%d)", got.Line, got.Col, tt.want.Line, tt.want.Col)
			}
		})
	}
}

func TestWordBack(t *testing.T) {
	b := sliceBuffer{"foo bar.baz", "", "next word"}

	tests := []struct {
		name  string
		start Cursor
		n     int
		want  Cursor
	}{
		{"b to word start", at(0, 6), 1, at(0, 4)},
		{"b from word start to previous", at(0, 4), 1, at(0, 0)},
		{"b over punct run", at(0, 8), 1, at(0, 7)},
		{"b stops on empty line", at(2, 0), 1, at(1, 0)},
		{"b across empty line", at(1, 0), 1, at(0, 8)},
		{"b clamps at start", at(0, 0), 5, at(0, 0)},
		{"2b", at(2, 5), 2, at(1, 0)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := WordBack(b, tt.start, tt.n)
			if got.Line != tt.want.Line || got.Col != tt.want.Col {
				t.Errorf("got (%d,%d), want (%d,%d)", got.Line, got.Col, tt.want.Line, tt.want.Col)
			}
		})
	}
}

func TestWordEnd(t *testing.T) {
	b := sliceBuffer{"foo bar.baz", "", "next"}

	tests := []struct {
		name  string
		start Cursor
		n     int
		want  Cursor
	}{
		{"e to end of word", at(0, 0), 1, at(0, 2)},
		{"e from end jumps to next end", at(0, 2), 1, at(0, 6)},
		{"e onto punct end", at(0, 6), 1, at(0, 7)},
		{"e skips empty lines", at(0, 10), 1, at(2, 3)},
		{"2e", at(0, 0), 2, at(0, 6)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := WordEnd(b, tt.start, tt.n)
			if got.Line != tt.want.Line || got.Col != tt.want.Col {
				t.Errorf("got (%d,%d), want (%d,%d)", got.Line, got.Col, tt.want.Line, tt.want.Col)
			}
		})
	}
}

func TestWordMotionsUnicode(t *testing.T) {
	b := sliceBuffer{"det är vackert"}

	c := WordForward(b, at(0, 0), 1)
	if c.Col != 4 {
		t.Fatalf("w over 'det ': col %d, want 4 (rune index, not byte)", c.Col)
	}
	c = WordForward(b, c, 1)
	if c.Col != 7 {
		t.Fatalf("w over 'är ': col %d, want 7", c.Col)
	}
	if got := WordEnd(b, at(0, 0), 3); got.Col != 13 {
		t.Fatalf("3e: col %d, want 13 (end of vackert)", got.Col)
	}
}

func TestFindOnLine(t *testing.T) {
	//               0123456789
	b := sliceBuffer{"abcabcabca"}

	tests := []struct {
		name    string
		start   int
		target  rune
		till    bool
		back    bool
		n       int
		wantCol int
		found   bool
	}{
		{"f finds next", 0, 'c', false, false, 1, 2, true},
		{"2fc", 0, 'c', false, false, 2, 5, true},
		{"f misses", 0, 'z', false, false, 1, 0, false},
		{"t stops before", 0, 'c', true, false, 1, 1, true},
		{"F backwards", 9, 'c', false, true, 1, 8, true},
		{"2Fc", 9, 'c', false, true, 2, 5, true},
		{"T stops after", 9, 'c', true, true, 1, 9, false},
		{"T lands cleanly when not adjacent", 6, 'a', true, true, 1, 4, true},
		{"f from own position skips self", 2, 'c', false, false, 1, 5, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, found := FindOnLine(b, at(0, tt.start), tt.target, tt.till, tt.back, tt.n)
			if found != tt.found {
				t.Fatalf("found = %v, want %v", found, tt.found)
			}
			if found && got.Col != tt.wantCol {
				t.Errorf("col = %d, want %d", got.Col, tt.wantCol)
			}
			if !found && got.Col != tt.start {
				t.Errorf("failed find moved the cursor to %d", got.Col)
			}
		})
	}
}

// Motions must never produce a cursor outside the buffer, whatever the
// input. Sweep every motion from every position of an awkward buffer.
func TestMotionsNeverEscapeTheBuffer(t *testing.T) {
	b := sliceBuffer{"", "x", "ab cd", "", "  ", "slutet"}

	motions := map[string]func(Cursor) Cursor{
		"h":  func(c Cursor) Cursor { return MoveLeft(b, c, 3) },
		"l":  func(c Cursor) Cursor { return MoveRight(b, c, 3) },
		"j":  func(c Cursor) Cursor { return MoveDown(b, c, 2) },
		"k":  func(c Cursor) Cursor { return MoveUp(b, c, 2) },
		"w":  func(c Cursor) Cursor { return WordForward(b, c, 2) },
		"b":  func(c Cursor) Cursor { return WordBack(b, c, 2) },
		"e":  func(c Cursor) Cursor { return WordEnd(b, c, 2) },
		"0":  func(c Cursor) Cursor { return LineStart(c) },
		"$":  func(c Cursor) Cursor { return LineEnd(b, c, 2) },
		"fx": func(c Cursor) Cursor { got, _ := FindOnLine(b, c, 'x', false, false, 1); return got },
	}

	for line := 0; line < b.LineCount(); line++ {
		maxCol := len([]rune(b.Line(line)))
		for col := 0; col <= maxCol; col++ {
			for name, motion := range motions {
				got := motion(at(line, col))
				if got.Line < 0 || got.Line >= b.LineCount() {
					t.Fatalf("%s from (%d,%d) escaped to line %d", name, line, col, got.Line)
				}
				lineLen := len([]rune(b.Line(got.Line)))
				limit := lineLen - 1
				if limit < 0 {
					limit = 0
				}
				if got.Col < 0 || got.Col > limit {
					t.Fatalf("%s from (%d,%d) escaped to col %d on line %d (len %d)",
						name, line, col, got.Col, got.Line, lineLen)
				}
			}
		}
	}
}

func motionKeys() MotionKeys {
	km := keymap.Default()
	return MotionKeys{
		Left: km.MotionLeft, Right: km.MotionRight,
		Down: km.PreviewDown, Up: km.PreviewUp,
		WordForward: km.MotionWordForward, WordBack: km.MotionWordBack, WordEnd: km.MotionWordEnd,
		LineStart: km.MotionLineStart, LineEnd: km.MotionLineEnd,
		FindChar: km.FindChar, FindCharBack: km.FindCharBack,
		TillChar: km.TillChar, TillCharBack: km.TillCharBack,
		RepeatFind: km.RepeatFind, RepeatFindBack: km.RepeatFindBack,
	}
}

func TestBufferMotionFindChord(t *testing.T) {
	b := sliceBuffer{"abcabcabca"}
	mk := motionKeys()
	var r Resolver
	cur := at(0, 0)

	// f arms; the target rune completes.
	got, res := r.BufferMotion(mk, "f", b, cur)
	if res != BufPending || !r.FindPending() {
		t.Fatalf("f gave %v pending=%v, want BufPending", res, r.FindPending())
	}
	got, res = r.BufferMotion(mk, "c", b, got)
	if res != BufMoved || got.Col != 2 {
		t.Fatalf("fc gave %v col=%d, want BufMoved col 2", res, got.Col)
	}

	// ; repeats, , reverses.
	got, res = r.BufferMotion(mk, ";", b, got)
	if res != BufMoved || got.Col != 5 {
		t.Fatalf("; gave %v col=%d, want col 5", res, got.Col)
	}
	got, res = r.BufferMotion(mk, ",", b, got)
	if res != BufMoved || got.Col != 2 {
		t.Fatalf(", gave %v col=%d, want col 2", res, got.Col)
	}

	// A missed find consumes the key but does not move.
	got, res = r.BufferMotion(mk, "f", b, got)
	got, res = r.BufferMotion(mk, "z", b, got)
	if res != BufFailed || got.Col != 2 {
		t.Fatalf("fz gave %v col=%d, want BufFailed col 2", res, got.Col)
	}

	// Esc cancels a pending chord.
	r.BufferMotion(mk, "f", b, got)
	_, res = r.BufferMotion(mk, "esc", b, got)
	if res != BufFailed || r.FindPending() {
		t.Fatalf("esc during chord gave %v pending=%v", res, r.FindPending())
	}
}

// While a find is pending, a digit is the target — f3 finds the
// character 3, it does not start a count.
func TestBufferMotionDigitIsFindTarget(t *testing.T) {
	b := sliceBuffer{"ab3cd"}
	mk := motionKeys()
	var r Resolver

	r.BufferMotion(mk, "f", b, at(0, 0))
	got, res := r.BufferMotion(mk, "3", b, at(0, 0))
	if res != BufMoved || got.Col != 2 {
		t.Fatalf("f3 gave %v col=%d, want BufMoved col 2", res, got.Col)
	}
	if r.PendingCount() != 0 {
		t.Fatalf("count = %d after f3, want 0", r.PendingCount())
	}
}

// 3fx finds the third x; the count survives the two-key chord.
func TestBufferMotionCountedFind(t *testing.T) {
	b := sliceBuffer{"x.x.x.x"}
	mk := motionKeys()
	var r Resolver

	r.Digit("3")
	r.BufferMotion(mk, "f", b, at(0, 0))
	got, res := r.BufferMotion(mk, "x", b, at(0, 0))
	if res != BufMoved || got.Col != 6 {
		t.Fatalf("3fx gave %v col=%d, want col 6", res, got.Col)
	}
}

// Repeating a till skips the adjacent target, vim's ; special case.
func TestBufferMotionTillRepeatSkips(t *testing.T) {
	b := sliceBuffer{"a.b.c.d"}
	mk := motionKeys()
	var r Resolver

	r.BufferMotion(mk, "t", b, at(0, 0))
	got, _ := r.BufferMotion(mk, ".", b, at(0, 0))
	if got.Col != 0 {
		t.Fatalf("t. from 0 landed at %d, want 0 (before the dot at 1)", got.Col)
	}
	got, res := r.BufferMotion(mk, ";", b, got)
	if res != BufMoved || got.Col != 2 {
		t.Fatalf("; after till gave %v col=%d, want col 2 (skipped the adjacent dot)", res, got.Col)
	}
}

// 0 with a pending count is not a motion — the caller's digit handling
// owns it.
func TestBufferMotionZeroDefersToCount(t *testing.T) {
	b := sliceBuffer{"abcdef"}
	mk := motionKeys()
	var r Resolver

	r.Digit("1")
	_, res := r.BufferMotion(mk, "0", b, at(0, 5))
	if res != BufNone {
		t.Fatalf("0 with pending count gave %v, want BufNone", res)
	}
	r.ClearCount()
	got, res := r.BufferMotion(mk, "0", b, at(0, 5))
	if res != BufMoved || got.Col != 0 {
		t.Fatalf("bare 0 gave %v col=%d, want BufMoved col 0", res, got.Col)
	}
}

// A repeat with nothing to repeat is consumed, not routed onward.
func TestBufferMotionRepeatWithoutFind(t *testing.T) {
	b := sliceBuffer{"abc"}
	mk := motionKeys()
	var r Resolver

	_, res := r.BufferMotion(mk, ";", b, at(0, 0))
	if res != BufFailed {
		t.Fatalf("; without a find gave %v, want BufFailed", res)
	}
}
