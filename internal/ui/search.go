package ui

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

// SearchPattern is a compiled buffer-search query.
//
// It is deliberately free of any notion of where the bytes came from, so
// the same pattern serves the Service Bus message body (one string in
// memory) and the blob preview (a sliding window over a blob that may be
// gigabytes, scanned one chunk at a time).
type SearchPattern struct {
	re *regexp.Regexp
	// reAfter matches the query immediately after one leading rune. It
	// answers "does a match start right here?" for a position in the
	// middle of a buffer, where ^ and \b depend on the rune before it.
	reAfter *regexp.Regexp
	// Query is the text the user typed, kept for redisplay in the search
	// bar and in messages.
	Query string
}

// SearchMatch is a half-open byte range within whatever buffer was
// searched. Offsets are relative to that buffer, not to the blob.
type SearchMatch struct {
	Start int
	End   int
}

// CompileSearchPattern turns a user query into a pattern, applying
// smartcase: an all-lowercase query matches case-insensitively, while
// any uppercase character makes the whole query case-sensitive. This is
// vim's 'ignorecase' + 'smartcase' behavior, which is what a query typed
// into a / prompt is expected to do.
//
// ^ and $ anchor to lines, as they do in vim. The buffers searched are
// many lines long — a window of a blob, a multi-megabyte scan chunk —
// so anchoring to the buffer's ends would make ^error match only where
// a chunk happens to begin.
//
// The regexp engine is RE2, so there are no backreferences, but matching
// is linear in the input — which matters when the input is a multi-
// megabyte chunk of somebody's log file.
func CompileSearchPattern(query string) (SearchPattern, error) {
	if strings.TrimSpace(query) == "" {
		return SearchPattern{}, fmt.Errorf("empty search pattern")
	}

	flags := "m"
	if !hasUpper(query) {
		flags += "i"
	}
	expr := "(?" + flags + ")" + query

	re, err := regexp.Compile(expr)
	if err != nil {
		return SearchPattern{}, fmt.Errorf("invalid pattern: %w", err)
	}
	reAfter, err := regexp.Compile(`\A(?s:.)(` + expr + `)`)
	if err != nil {
		return SearchPattern{}, fmt.Errorf("invalid pattern: %w", err)
	}
	return SearchPattern{re: re, reAfter: reAfter, Query: query}, nil
}

// Valid reports whether the pattern was compiled successfully. The zero
// SearchPattern is not valid.
func (p SearchPattern) Valid() bool {
	return p.re != nil
}

// Find returns the first match at or after from, or nil.
func (p SearchPattern) Find(data []byte, from int) *SearchMatch {
	return SearchBufferForward(p, data, from)
}

// SearchBufferForward returns the first match starting at or after from.
// Offsets outside the buffer are clamped; a from past the end finds
// nothing.
//
// The regexp package can only search a slice from its first byte, and a
// slice cut at from looks like the start of the text: ^ and \b hold
// there whatever came before. Everything after the cut still sees its
// real neighbours, so only a match at the cut itself is in doubt, and
// that one is decided by reAfter with the preceding rune in view.
func SearchBufferForward(p SearchPattern, data []byte, from int) *SearchMatch {
	if !p.Valid() || len(data) == 0 {
		return nil
	}
	if from <= 0 {
		loc := p.re.FindIndex(data)
		if loc == nil {
			return nil
		}
		return &SearchMatch{Start: loc[0], End: loc[1]}
	}

	for ; from <= len(data); from++ {
		before := from - 1
		if idx := p.reAfter.FindSubmatchIndex(data[before:]); idx != nil {
			return &SearchMatch{Start: before + idx[2], End: before + idx[3]}
		}

		loc := p.re.FindIndex(data[from:])
		if loc == nil {
			return nil
		}
		if loc[0] > 0 {
			return &SearchMatch{Start: from + loc[0], End: from + loc[1]}
		}
		// A match at the cut that reAfter rejected exists only because
		// the cut hid what precedes it. Step past it and look again.
	}
	return nil
}

// SearchBufferBackward returns the last match that starts strictly
// before from. Searching backwards means scanning all matches up to that
// point and keeping the last, since RE2 has no reverse search.
func SearchBufferBackward(p SearchPattern, data []byte, from int) *SearchMatch {
	if !p.Valid() || len(data) == 0 || from <= 0 {
		return nil
	}
	if from > len(data) {
		from = len(data)
	}

	var last *SearchMatch
	for pos := 0; pos < from; {
		match := SearchBufferForward(p, data, pos)
		if match == nil || match.Start >= from {
			break
		}
		last = match
		pos = advance(match.Start, match.End)
	}
	return last
}

// SearchBufferAll returns every match in the buffer, in order. Used to
// highlight the matches on screen, so the buffer is expected to be
// small.
func SearchBufferAll(p SearchPattern, data []byte) []SearchMatch {
	if !p.Valid() || len(data) == 0 {
		return nil
	}

	var matches []SearchMatch
	for pos := 0; pos <= len(data); {
		match := SearchBufferForward(p, data, pos)
		if match == nil {
			break
		}
		matches = append(matches, *match)
		pos = advance(match.Start, match.End)
	}
	return matches
}

// advance computes where to resume scanning after a match. A pattern
// like `x*` can match the empty string, which would otherwise leave the
// scan parked on the same offset forever, so an empty match steps
// forward by one byte.
func advance(start, end int) int {
	if end > start {
		return end
	}
	return start + 1
}

func hasUpper(s string) bool {
	for _, r := range s {
		if unicode.IsUpper(r) {
			return true
		}
	}
	return false
}
