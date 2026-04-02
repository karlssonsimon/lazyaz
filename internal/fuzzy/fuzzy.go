// Package fuzzy provides fuzzy string matching powered by fzf's algorithm.
// All overlay filters and search inputs go through this package so the
// matching implementation can be swapped in one place.
package fuzzy

import (
	"sort"
	"strings"

	"github.com/junegunn/fzf/src/algo"
	"github.com/junegunn/fzf/src/util"
)

// Result holds the outcome of a single fuzzy match.
type Result struct {
	Score int   // higher is better; 0 means no match
	Pos   []int // byte positions of matched characters (nil when not requested)
}

var slab = util.MakeSlab(100*1024, 2048)

// Match scores pattern against candidate using fzf's FuzzyMatchV2 algorithm.
// An empty pattern matches everything with score 0.
// Returns score > 0 on match, 0 on no match.
func Match(pattern, candidate string) Result {
	if pattern == "" {
		return Result{}
	}
	input := util.ToChars([]byte(strings.ToLower(candidate)))
	p := []rune(strings.ToLower(pattern))
	r, pos := algo.FuzzyMatchV2(false, true, true, &input, p, false, slab)
	if r.Start < 0 {
		return Result{}
	}
	var positions []int
	if pos != nil {
		positions = *pos
	}
	return Result{Score: r.Score, Pos: positions}
}

// Filter returns indices into items where the pattern matches, ordered by
// score (best first). getter extracts the matchable string from each item.
func Filter[T any](pattern string, items []T, getter func(T) string) []int {
	if pattern == "" {
		idx := make([]int, len(items))
		for i := range items {
			idx[i] = i
		}
		return idx
	}

	type scored struct {
		index int
		score int
	}

	p := []rune(strings.ToLower(pattern))
	var matches []scored
	for i, item := range items {
		input := util.ToChars([]byte(strings.ToLower(getter(item))))
		r, _ := algo.FuzzyMatchV2(false, true, true, &input, p, false, slab)
		if r.Start >= 0 {
			matches = append(matches, scored{index: i, score: r.Score})
		}
	}

	sort.Slice(matches, func(i, j int) bool {
		return matches[i].score > matches[j].score
	})

	idx := make([]int, len(matches))
	for i, m := range matches {
		idx[i] = m.index
	}
	return idx
}
