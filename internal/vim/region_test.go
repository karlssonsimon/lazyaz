package vim

import "testing"

func TestBigWordMotions(t *testing.T) {
	//            0         1
	//            0123456789012345678
	b := sliceBuffer{`foo.bar() "baz,qux"`, "next"}

	tests := []struct {
		name string
		got  Cursor
		want Cursor
	}{
		{"W jumps whole non-whitespace runs", WORDForward(b, at(0, 0), 1), at(0, 10)},
		{"W onto next line", WORDForward(b, at(0, 10), 1), at(1, 0)},
		{"B back to WORD start", WORDBack(b, at(0, 15), 1), at(0, 10)},
		{"B across WORDs", WORDBack(b, at(0, 10), 1), at(0, 0)},
		{"E to WORD end", WORDEnd(b, at(0, 0), 1), at(0, 8)},
		{"E again", WORDEnd(b, at(0, 8), 1), at(0, 18)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got.Line != tt.want.Line || tt.got.Col != tt.want.Col {
				t.Errorf("got (%d,%d), want (%d,%d)", tt.got.Line, tt.got.Col, tt.want.Line, tt.want.Col)
			}
		})
	}
}

func TestRegionKinds(t *testing.T) {
	b := sliceBuffer{"foo bar baz", "second line", "third"}

	tests := []struct {
		name      string
		from, to  Cursor
		kind      MotionKind
		wantStart Cursor
		wantEnd   Cursor
		linewise  bool
	}{
		{"exclusive forward", at(0, 0), at(0, 4), KindExclusive, at(0, 0), at(0, 4), false},
		{"inclusive forward includes the end rune", at(0, 0), at(0, 2), KindInclusive, at(0, 0), at(0, 3), false},
		{"backward orders and stays exclusive", at(0, 4), at(0, 0), KindExclusive, at(0, 0), at(0, 4), false},
		{"linewise spans whole lines either direction", at(1, 3), at(0, 5), KindLinewise, at(0, 0), at(1, 11), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RegionFor(b, tt.from, tt.to, tt.kind)
			if got.Start != tt.wantStart || got.End != tt.wantEnd || got.Linewise != tt.linewise {
				t.Errorf("got %+v, want start=%v end=%v linewise=%v", got, tt.wantStart, tt.wantEnd, tt.linewise)
			}
		})
	}
}

// Vim's exclusive adjustment: a span ending in column 0 of a later line
// retreats to the end of the previous line. This is why yw on the last
// word of a line stops at the line end instead of eating the newline.
func TestRegionExclusiveEndOfLineRetreat(t *testing.T) {
	b := sliceBuffer{"foo bar", "next line"}

	// w from "bar" lands on (1,0); the yank region must stop at (0,7).
	to := WordForward(b, at(0, 4), 1)
	if to.Line != 1 || to.Col != 0 {
		t.Fatalf("setup: w landed at (%d,%d), want (1,0)", to.Line, to.Col)
	}
	got := RegionFor(b, at(0, 4), to, KindExclusive)
	if got.Start != at(0, 4) || got.End.Line != 0 || got.End.Col != 7 {
		t.Errorf("region = %+v, want (0,4)..(0,7) — the retreat did not fire", got)
	}
}

func TestQuoteObject(t *testing.T) {
	//            0          1
	//            01234567 89012345678 9
	b := sliceBuffer{`say "hello world" end`, `pre "a" mid "b" post`, `esc "he \"x\" y" end`, `unclosed "oops`}

	tests := []struct {
		name      string
		cur       Cursor
		around    bool
		wantStart Cursor
		wantEnd   Cursor
		ok        bool
	}{
		{"inside the quotes", at(0, 8), false, at(0, 5), at(0, 16), true},
		{"on the opening quote", at(0, 4), false, at(0, 5), at(0, 16), true},
		{"before the pair jumps forward", at(0, 1), false, at(0, 5), at(0, 16), true},
		{"around includes quotes and trailing space", at(0, 8), true, at(0, 4), at(0, 18), true},
		{"second pair on the line", at(1, 13), false, at(1, 13), at(1, 14), true},
		{"escaped quotes are not delimiters", at(2, 10), false, at(2, 5), at(2, 15), true},
		{"unclosed pair fails", at(3, 11), false, Cursor{}, Cursor{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := QuoteObject(b, tt.cur, '"', tt.around)
			if ok != tt.ok {
				t.Fatalf("ok = %v, want %v", ok, tt.ok)
			}
			if ok && (got.Start != tt.wantStart || got.End != tt.wantEnd) {
				t.Errorf("region = %+v, want %v..%v", got, tt.wantStart, tt.wantEnd)
			}
		})
	}
}

func TestBracketObject(t *testing.T) {
	b := sliceBuffer{`f(a, g(b), c) tail`, `{`, `  "k": [1, 2],`, `}`}

	tests := []struct {
		name        string
		cur         Cursor
		open, close rune
		around      bool
		wantStart   Cursor
		wantEnd     Cursor
		ok          bool
	}{
		{"inner parens from inside", at(0, 3), '(', ')', false, at(0, 2), at(0, 12), true},
		{"nested pair from within it", at(0, 7), '(', ')', false, at(0, 7), at(0, 8), true},
		{"nesting skipped from outside it", at(0, 10), '(', ')', false, at(0, 2), at(0, 12), true},
		{"around includes the brackets", at(0, 3), '(', ')', true, at(0, 1), at(0, 13), true},
		{"on the opening bracket", at(0, 1), '(', ')', false, at(0, 2), at(0, 12), true},
		{"on the closing bracket", at(0, 12), '(', ')', false, at(0, 2), at(0, 12), true},
		{"multi-line braces", at(2, 4), '{', '}', false, at(1, 1), at(3, 0), true},
		{"no enclosing pair fails", at(0, 15), '(', ')', false, Cursor{}, Cursor{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := BracketObject(b, tt.cur, tt.open, tt.close, tt.around)
			if ok != tt.ok {
				t.Fatalf("ok = %v, want %v", ok, tt.ok)
			}
			if ok && (got.Start != tt.wantStart || got.End != tt.wantEnd) {
				t.Errorf("region = %+v, want %v..%v", got, tt.wantStart, tt.wantEnd)
			}
		})
	}
}

func TestWordObject(t *testing.T) {
	b := sliceBuffer{"foo bar.baz  end"}

	tests := []struct {
		name      string
		cur       Cursor
		big       bool
		around    bool
		wantStart Cursor
		wantEnd   Cursor
	}{
		{"iw mid-word", at(0, 5), false, false, at(0, 4), at(0, 7)},
		{"iw on punctuation is the punct run", at(0, 7), false, false, at(0, 7), at(0, 8)},
		{"aw takes leading spaces when none trail", at(0, 13), false, true, at(0, 11), at(0, 16)},
		{"aw beside punctuation takes the leading space", at(0, 5), false, true, at(0, 3), at(0, 7)},
		{"iW spans the punctuated run", at(0, 5), true, false, at(0, 4), at(0, 11)},
		{"aW takes the double space", at(0, 5), true, true, at(0, 4), at(0, 13)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := WordObject(b, tt.cur, tt.big, tt.around)
			if !ok {
				t.Fatal("word object failed")
			}
			if got.Start != tt.wantStart || got.End != tt.wantEnd {
				t.Errorf("region = %+v, want %v..%v", got, tt.wantStart, tt.wantEnd)
			}
		})
	}
}

// The operator grammar end to end: y arms, motions complete as yanks
// with the right kinds, objects resolve, and cancellation is clean at
// every depth.
func TestOperatorGrammar(t *testing.T) {
	//               0123456789012345
	b := sliceBuffer{`foo bar "qux" z`, "second line"}
	mk := motionKeys()

	yank := func(t *testing.T, r *Resolver, keys ...string) BufferAction {
		t.Helper()
		var act BufferAction
		for _, k := range keys {
			act = r.BufferMotion(mk, k, b, at(0, 0), false)
		}
		return act
	}

	t.Run("yw is exclusive", func(t *testing.T) {
		var r Resolver
		r.ArmOperator()
		act := yank(t, &r, "w")
		if act.Kind != BufYank || act.Region.Start != at(0, 0) || act.Region.End != at(0, 4) {
			t.Fatalf("yw = %+v", act)
		}
	})

	t.Run("ye is inclusive", func(t *testing.T) {
		var r Resolver
		r.ArmOperator()
		act := yank(t, &r, "e")
		if act.Kind != BufYank || act.Region.End != at(0, 3) {
			t.Fatalf("ye = %+v", act)
		}
	})

	t.Run("y3w counts inside the operator", func(t *testing.T) {
		var r Resolver
		r.ArmOperator()
		act := yank(t, &r, "3", "w")
		if act.Kind != BufYank || act.Region.End != at(0, 9) {
			t.Fatalf("y3w = %+v", act)
		}
	})

	t.Run("yfx yanks through the target", func(t *testing.T) {
		var r Resolver
		r.ArmOperator()
		act := yank(t, &r, "f", "r")
		if act.Kind != BufYank || act.Region.End != at(0, 7) {
			t.Fatalf("yfr = %+v", act)
		}
	})

	t.Run("yy is linewise", func(t *testing.T) {
		var r Resolver
		r.ArmOperator()
		act := yank(t, &r, "y")
		if act.Kind != BufYank || !act.Region.Linewise || act.Region.Start.Line != 0 || act.Region.End.Line != 0 {
			t.Fatalf("yy = %+v", act)
		}
	})

	t.Run("2yy spans two lines", func(t *testing.T) {
		var r Resolver
		r.Digit("2")
		r.ArmOperator()
		act := yank(t, &r, "y")
		if act.Kind != BufYank || !act.Region.Linewise || act.Region.End.Line != 1 {
			t.Fatalf("2yy = %+v", act)
		}
	})

	t.Run("yi quote", func(t *testing.T) {
		var r Resolver
		r.ArmOperator()
		act := yank(t, &r, "i", "\"")
		if act.Kind != BufYank || act.Region.Start != at(0, 9) || act.Region.End != at(0, 12) {
			t.Fatalf("yi\" = %+v", act)
		}
	})

	t.Run("vi quote selects", func(t *testing.T) {
		var r Resolver
		var act BufferAction
		for _, k := range []string{"i", "\""} {
			act = r.BufferMotion(mk, k, b, at(0, 10), true)
		}
		if act.Kind != BufSelect || act.Region.Start != at(0, 9) || act.Region.End != at(0, 12) {
			t.Fatalf("vi\" = %+v", act)
		}
	})

	t.Run("i without operator or visual is not a grammar key", func(t *testing.T) {
		var r Resolver
		act := r.BufferMotion(mk, "i", b, at(0, 0), false)
		if act.Kind != BufNone {
			t.Fatalf("bare i = %+v, want BufNone", act)
		}
	})

	t.Run("y then garbage cancels consumed", func(t *testing.T) {
		var r Resolver
		r.ArmOperator()
		act := yank(t, &r, "q")
		if act.Kind != BufFailed || r.OperatorPending() {
			t.Fatalf("yq = %+v pending=%v", act, r.OperatorPending())
		}
	})

	t.Run("y esc cancels", func(t *testing.T) {
		var r Resolver
		r.ArmOperator()
		act := yank(t, &r, "esc")
		if act.Kind != BufFailed || r.BufferPending() {
			t.Fatalf("y-esc = %+v pending=%v", act, r.BufferPending())
		}
	})

	t.Run("yi esc cancels the object", func(t *testing.T) {
		var r Resolver
		r.ArmOperator()
		act := yank(t, &r, "i", "esc")
		if act.Kind != BufFailed || r.BufferPending() {
			t.Fatalf("yi-esc = %+v pending=%v", act, r.BufferPending())
		}
	})

	t.Run("yb yanks backward exclusively", func(t *testing.T) {
		var r Resolver
		r.ArmOperator()
		var act BufferAction
		for _, k := range []string{"b"} {
			act = r.BufferMotion(mk, k, b, at(0, 6), false)
		}
		if act.Kind != BufYank || act.Region.Start != at(0, 4) || act.Region.End != at(0, 6) {
			t.Fatalf("yb = %+v", act)
		}
	})

	t.Run("yW uses the big word", func(t *testing.T) {
		var r Resolver
		r.ArmOperator()
		var act BufferAction
		for _, k := range []string{"W"} {
			act = r.BufferMotion(mk, k, b, at(0, 8), false)
		}
		if act.Kind != BufYank || act.Region.End != at(0, 14) {
			t.Fatalf("yW = %+v", act)
		}
	})
}
