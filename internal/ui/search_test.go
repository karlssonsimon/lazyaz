package ui

import (
	"strings"
	"testing"
)

func TestCompileSearchPatternSmartcase(t *testing.T) {
	tests := []struct {
		name       string
		query      string
		haystack   string
		wantMatch  bool
		wantOffset int
	}{
		{"lowercase query is case-insensitive", "error", "FATAL ERROR here", true, 6},
		{"lowercase query still matches lowercase", "error", "an error here", true, 3},
		{"uppercase anywhere makes it case-sensitive", "Error", "an error here", false, 0},
		{"uppercase query matches exact case", "Error", "an Error here", true, 3},
		{"digits do not trigger case sensitivity", "err0r", "ERR0R", true, 0},
		{"regex alternation", "fatal|panic", "a panic occurred", true, 2},
		{"anchored match", "^start", "start of line", true, 0},
		{"character class", "[0-9]+", "abc 4711 def", true, 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := CompileSearchPattern(tt.query)
			if err != nil {
				t.Fatalf("CompileSearchPattern(%q) failed: %v", tt.query, err)
			}
			loc := p.Find([]byte(tt.haystack), 0)
			if tt.wantMatch {
				if loc == nil {
					t.Fatalf("no match for %q in %q", tt.query, tt.haystack)
				}
				if loc.Start != tt.wantOffset {
					t.Errorf("match at %d, want %d", loc.Start, tt.wantOffset)
				}
			} else if loc != nil {
				t.Errorf("unexpected match at %d for %q in %q", loc.Start, tt.query, tt.haystack)
			}
		})
	}
}

// An invalid regex has to surface as an error the user can see, not as a
// search that silently finds nothing.
func TestCompileSearchPatternRejectsInvalidRegex(t *testing.T) {
	for _, q := range []string{"[unclosed", "*leading", "(unbalanced"} {
		if _, err := CompileSearchPattern(q); err == nil {
			t.Errorf("CompileSearchPattern(%q) returned no error, want one", q)
		}
	}
}

func TestCompileSearchPatternRejectsEmpty(t *testing.T) {
	if _, err := CompileSearchPattern(""); err == nil {
		t.Error("empty query returned no error, want one")
	}
	if _, err := CompileSearchPattern("   "); err == nil {
		t.Error("whitespace-only query returned no error, want one")
	}
}

// buf is a multi-line fixture with known offsets.
//
//	offset 0  : "alpha beta\n"      (len 11)
//	offset 11 : "gamma delta\n"     (len 12)
//	offset 23 : "alpha epsilon\n"   (len 14)
//	offset 37 : "zeta alpha"        (len 10, no trailing newline)
const buf = "alpha beta\ngamma delta\nalpha epsilon\nzeta alpha"

func TestSearchBufferForward(t *testing.T) {
	p := mustPattern(t, "alpha")

	tests := []struct {
		name  string
		from  int
		want  int
		found bool
	}{
		{"from the very start", 0, 0, true},
		{"past the first match", 1, 23, true},
		{"exactly on the second match", 23, 23, true},
		{"past the second match", 24, 42, true},
		{"past the last match", 43, 0, false},
		{"from beyond the buffer", 999, 0, false},
		{"from a negative offset clamps to zero", -5, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loc := SearchBufferForward(p, []byte(buf), tt.from)
			if !tt.found {
				if loc != nil {
					t.Fatalf("found a match at %d, want none", loc.Start)
				}
				return
			}
			if loc == nil {
				t.Fatal("no match found, want one")
			}
			if loc.Start != tt.want {
				t.Errorf("match at %d, want %d", loc.Start, tt.want)
			}
			if got := buf[loc.Start:loc.End]; got != "alpha" {
				t.Errorf("matched %q, want %q", got, "alpha")
			}
		})
	}
}

func TestSearchBufferBackward(t *testing.T) {
	p := mustPattern(t, "alpha")

	tests := []struct {
		name  string
		from  int
		want  int
		found bool
	}{
		{"from the end finds the last match", len(buf), 42, true},
		{"just before the last match", 42, 23, true},
		{"just before the second match", 23, 0, true},
		{"before the first match finds nothing", 0, 0, false},
		{"from a negative offset finds nothing", -5, 0, false},
		{"from beyond the buffer finds the last match", 999, 42, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loc := SearchBufferBackward(p, []byte(buf), tt.from)
			if !tt.found {
				if loc != nil {
					t.Fatalf("found a match at %d, want none", loc.Start)
				}
				return
			}
			if loc == nil {
				t.Fatal("no match found, want one")
			}
			if loc.Start != tt.want {
				t.Errorf("match at %d, want %d", loc.Start, tt.want)
			}
		})
	}
}

// Every match in the loaded window is highlighted, so the finder that
// drives highlighting must return them all.
func TestSearchBufferAll(t *testing.T) {
	p := mustPattern(t, "alpha")

	got := SearchBufferAll(p, []byte(buf))
	want := []int{0, 23, 42}

	if len(got) != len(want) {
		t.Fatalf("found %d matches, want %d: %v", len(got), len(want), got)
	}
	for i, loc := range got {
		if loc.Start != want[i] {
			t.Errorf("match %d at %d, want %d", i, loc.Start, want[i])
		}
	}
}

func TestSearchBufferAllOnEmptyInput(t *testing.T) {
	p := mustPattern(t, "alpha")
	if got := SearchBufferAll(p, nil); len(got) != 0 {
		t.Errorf("found %d matches in an empty buffer, want 0", len(got))
	}
}

// A pattern that can match an empty string must not spin forever or
// report an unbounded number of matches.
func TestSearchBufferHandlesEmptyMatchingPattern(t *testing.T) {
	p := mustPattern(t, "x*")

	all := SearchBufferAll(p, []byte("abc"))
	if len(all) > 4 {
		t.Errorf("empty-matching pattern produced %d matches on a 3-byte buffer, want at most 4", len(all))
	}

	// Must terminate rather than sit on offset 0 forever.
	if loc := SearchBufferForward(p, []byte("abc"), 0); loc == nil {
		t.Error("expected some match for an empty-matching pattern")
	}
}

// Line boundaries matter for the chunked blob scan: a match is found
// relative to the buffer it is given, whatever it holds.
func TestSearchBufferAcrossLines(t *testing.T) {
	p := mustPattern(t, "delta")

	loc := SearchBufferForward(p, []byte(buf), 0)
	if loc == nil {
		t.Fatal("no match")
	}
	if loc.Start != 17 {
		t.Errorf("match at %d, want 17", loc.Start)
	}
}

func TestSearchBufferMultibyte(t *testing.T) {
	p := mustPattern(t, "vackert")
	content := "det är vackert här"

	loc := SearchBufferForward(p, []byte(content), 0)
	if loc == nil {
		t.Fatal("no match in multibyte content")
	}
	if got := content[loc.Start:loc.End]; got != "vackert" {
		t.Errorf("matched %q, want %q", got, "vackert")
	}
}

func mustPattern(t *testing.T, query string) SearchPattern {
	t.Helper()
	p, err := CompileSearchPattern(query)
	if err != nil {
		t.Fatalf("CompileSearchPattern(%q): %v", query, err)
	}
	return p
}

// Sanity-check the fixture offsets the tables above depend on.
func TestSearchFixtureOffsets(t *testing.T) {
	for offset, want := range map[int]string{0: "alpha", 23: "alpha", 42: "alpha", 17: "delta"} {
		if got := buf[offset : offset+len(want)]; got != want {
			t.Errorf("fixture offset %d is %q, want %q", offset, got, want)
		}
	}
	if strings.Count(buf, "alpha") != 3 {
		t.Errorf("fixture has %d occurrences of alpha, want 3", strings.Count(buf, "alpha"))
	}
}
