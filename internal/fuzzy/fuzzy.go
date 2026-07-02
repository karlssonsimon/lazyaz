// Package fuzzy provides string matching powered by fzf's algorithm,
// including fzf's extended-search syntax. All overlay filters, list
// filters, and search inputs go through this package so the matching
// implementation lives in one place.
//
// Extended-search syntax (all matching is case-insensitive):
//
//	foo      fuzzy match
//	'foo     exact substring
//	^foo     prefix
//	foo$     suffix
//	^foo$    exact equal
//	!foo     exclude exact substring (also !^foo and !foo$)
//	foo bar  AND — every space-separated term must match
//	a | b    OR — pipe joins adjacent terms into alternatives
//
// A backslash-escaped space ("\ ") is a literal space inside a term.
package fuzzy

import (
	"sort"
	"strings"
	"sync"

	"github.com/junegunn/fzf/src/algo"
	"github.com/junegunn/fzf/src/util"
)

// Result holds the outcome of a single match.
type Result struct {
	Matched bool
	Score   int   // higher is better; meaningful only when Matched
	Pos     []int // rune positions of matched characters (nil for negations)
}

// The fzf algorithm scratch space is not safe for concurrent use, and
// bubbles runs list filtering on a background goroutine while overlay
// filters run on the update loop — so slabs are pooled per call.
var slabPool = sync.Pool{
	New: func() any { return util.MakeSlab(100*1024, 2048) },
}

type termKind int

const (
	termFuzzy termKind = iota
	termExact
	termPrefix
	termSuffix
	termEqual
)

type term struct {
	kind termKind
	inv  bool
	text []rune // lowercased
}

// Pattern is a parsed extended-search pattern: an AND across sets,
// where each set is an OR of alternative terms.
type Pattern struct {
	sets [][]term
}

// Empty reports whether the pattern imposes no constraint.
func (p Pattern) Empty() bool { return len(p.sets) == 0 }

// ParsePattern parses fzf extended-search syntax into a Pattern.
func ParsePattern(pattern string) Pattern {
	var sets [][]term
	pendingOr := false
	for _, tok := range tokenize(pattern) {
		if tok == "|" {
			// Joins the next term into the current OR set. A leading
			// or dangling pipe has no set to join and is ignored.
			pendingOr = len(sets) > 0
			continue
		}
		t, ok := parseTerm(tok)
		if !ok {
			pendingOr = false
			continue
		}
		if pendingOr {
			sets[len(sets)-1] = append(sets[len(sets)-1], t)
		} else {
			sets = append(sets, []term{t})
		}
		pendingOr = false
	}
	return Pattern{sets: sets}
}

// tokenize splits on spaces, honoring backslash-escaped spaces.
func tokenize(pattern string) []string {
	var tokens []string
	var cur strings.Builder
	escaped := false
	for _, r := range pattern {
		switch {
		case escaped:
			if r != ' ' {
				// Only "\ " is an escape; anything else keeps the
				// backslash so Windows-style paths still match literally.
				cur.WriteRune('\\')
			}
			cur.WriteRune(r)
			escaped = false
		case r == '\\':
			escaped = true
		case r == ' ':
			if cur.Len() > 0 {
				tokens = append(tokens, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteRune(r)
		}
	}
	if escaped {
		cur.WriteRune('\\')
	}
	if cur.Len() > 0 {
		tokens = append(tokens, cur.String())
	}
	return tokens
}

func parseTerm(tok string) (term, bool) {
	t := term{kind: termFuzzy}
	if strings.HasPrefix(tok, "!") {
		t.inv = true
		tok = tok[1:]
	}
	switch {
	case strings.HasPrefix(tok, "'"):
		t.kind = termExact
		tok = tok[1:]
	case strings.HasPrefix(tok, "^"):
		t.kind = termPrefix
		tok = tok[1:]
	}
	if strings.HasSuffix(tok, "$") && len(tok) > 1 {
		tok = tok[:len(tok)-1]
		if t.kind == termPrefix {
			t.kind = termEqual
		} else {
			t.kind = termSuffix
		}
	}
	// Bare inverted terms use exact matching, as in fzf: !foo means
	// "does not contain foo", not "does not fuzzy-match foo".
	if t.inv && t.kind == termFuzzy {
		t.kind = termExact
	}
	if tok == "" {
		return term{}, false
	}
	t.text = []rune(strings.ToLower(tok))
	return t, true
}

// matchTerm runs one term's matcher. Returns the matched rune
// positions only when requested.
func matchTerm(t term, input *util.Chars, withPos bool, slab *util.Slab) (score int, pos []int, found bool) {
	var r algo.Result
	var p *[]int
	switch t.kind {
	case termExact:
		r, p = algo.ExactMatchNaive(false, true, true, input, t.text, withPos, slab)
	case termPrefix:
		r, p = algo.PrefixMatch(false, true, true, input, t.text, withPos, slab)
	case termSuffix:
		r, p = algo.SuffixMatch(false, true, true, input, t.text, withPos, slab)
	case termEqual:
		r, p = algo.EqualMatch(false, true, true, input, t.text, withPos, slab)
	default:
		r, p = algo.FuzzyMatchV2(false, true, true, input, t.text, withPos, slab)
	}
	if r.Start < 0 {
		return 0, nil, false
	}
	if !withPos {
		return r.Score, nil, true
	}
	if p != nil {
		return r.Score, *p, true
	}
	// Non-fuzzy matchers report a contiguous range instead of positions.
	rangePos := make([]int, 0, r.End-r.Start)
	for i := r.Start; i < r.End; i++ {
		rangePos = append(rangePos, i)
	}
	return r.Score, rangePos, true
}

// match evaluates the whole pattern against candidate: every OR set
// must have at least one passing alternative.
func (p Pattern) match(candidate string, withPos bool, slab *util.Slab) Result {
	if p.Empty() {
		return Result{Matched: true}
	}
	input := util.ToChars([]byte(strings.ToLower(candidate)))
	total := 0
	var allPos []int
	for _, set := range p.sets {
		setOK := false
		bestScore := 0
		var bestPos []int
		for _, t := range set {
			score, pos, found := matchTerm(t, &input, withPos && !t.inv, slab)
			if t.inv {
				// A passing negation constrains but doesn't score or
				// highlight anything.
				if !found {
					setOK = true
				}
				continue
			}
			if found && (!setOK || score > bestScore) {
				setOK = true
				bestScore = score
				bestPos = pos
			}
		}
		if !setOK {
			return Result{}
		}
		total += bestScore
		allPos = append(allPos, bestPos...)
	}
	if withPos {
		sort.Ints(allPos)
	}
	return Result{Matched: true, Score: total, Pos: allPos}
}

// Match scores pattern against candidate. An empty pattern matches
// everything with score 0. Pos holds matched rune positions for
// highlight rendering.
func Match(pattern, candidate string) Result {
	if pattern == "" {
		return Result{Matched: true}
	}
	slab := slabPool.Get().(*util.Slab)
	defer slabPool.Put(slab)
	return ParsePattern(pattern).match(candidate, true, slab)
}

// Filter returns indices into items where the pattern matches, ordered
// by score (best first, ties keep input order). getter extracts the
// matchable string from each item.
func Filter[T any](pattern string, items []T, getter func(T) string) []int {
	if pattern == "" {
		idx := make([]int, len(items))
		for i := range items {
			idx[i] = i
		}
		return idx
	}

	p := ParsePattern(pattern)
	slab := slabPool.Get().(*util.Slab)
	defer slabPool.Put(slab)

	type scored struct {
		index int
		score int
	}
	var matches []scored
	for i, item := range items {
		if r := p.match(getter(item), false, slab); r.Matched {
			matches = append(matches, scored{index: i, score: r.Score})
		}
	}

	sort.SliceStable(matches, func(i, j int) bool {
		return matches[i].score > matches[j].score
	})

	idx := make([]int, len(matches))
	for i, m := range matches {
		idx[i] = m.index
	}
	return idx
}

// RankedMatch is one Ranks result: the target index, its score, and
// the matched rune positions for underline rendering.
type RankedMatch struct {
	Index int
	Score int
	Pos   []int
}

// Ranks matches pattern against targets and returns the matches with
// positions, best score first (ties keep input order). This is the
// building block for bubbles list FilterFunc adapters.
func Ranks(pattern string, targets []string) []RankedMatch {
	if pattern == "" {
		return nil
	}
	p := ParsePattern(pattern)
	slab := slabPool.Get().(*util.Slab)
	defer slabPool.Put(slab)

	var matches []RankedMatch
	for i, target := range targets {
		if r := p.match(target, true, slab); r.Matched {
			matches = append(matches, RankedMatch{Index: i, Score: r.Score, Pos: r.Pos})
		}
	}
	sort.SliceStable(matches, func(i, j int) bool {
		return matches[i].Score > matches[j].Score
	})
	return matches
}
