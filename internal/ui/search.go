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
// The regexp engine is RE2, so there are no backreferences, but matching
// is linear in the input — which matters when the input is a multi-
// megabyte chunk of somebody's log file.
func CompileSearchPattern(query string) (SearchPattern, error) {
	if strings.TrimSpace(query) == "" {
		return SearchPattern{}, fmt.Errorf("empty search pattern")
	}

	expr := query
	if !hasUpper(query) {
		expr = "(?i)" + expr
	}

	re, err := regexp.Compile(expr)
	if err != nil {
		return SearchPattern{}, fmt.Errorf("invalid pattern: %w", err)
	}
	return SearchPattern{re: re, Query: query}, nil
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
func SearchBufferForward(p SearchPattern, data []byte, from int) *SearchMatch {
	if !p.Valid() || len(data) == 0 {
		return nil
	}
	if from < 0 {
		from = 0
	}
	if from > len(data) {
		return nil
	}

	loc := p.re.FindIndex(data[from:])
	if loc == nil {
		return nil
	}
	return &SearchMatch{Start: from + loc[0], End: from + loc[1]}
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
		loc := p.re.FindIndex(data[pos:])
		if loc == nil {
			break
		}
		start := pos + loc[0]
		if start >= from {
			break
		}
		match := SearchMatch{Start: start, End: pos + loc[1]}
		last = &match
		pos = advance(start, pos+loc[1])
	}
	return last
}

// SearchBufferAll returns every match in the buffer, in order. Used to
// highlight the matches inside the loaded window, which is small enough
// that scanning all of it is cheap.
func SearchBufferAll(p SearchPattern, data []byte) []SearchMatch {
	if !p.Valid() || len(data) == 0 {
		return nil
	}

	var matches []SearchMatch
	for pos := 0; pos <= len(data); {
		loc := p.re.FindIndex(data[pos:])
		if loc == nil {
			break
		}
		start, end := pos+loc[0], pos+loc[1]
		matches = append(matches, SearchMatch{Start: start, End: end})
		pos = advance(start, end)
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
