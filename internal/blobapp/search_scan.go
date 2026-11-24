package blobapp

import (
	"bytes"
	"context"

	"github.com/karlssonsimon/lazyaz/internal/ui"
)

// searchChunkBytes is how much of a blob one range read pulls down while
// scanning. Large enough that a gigabyte blob is hundreds of round trips
// rather than thousands, small enough that cancelling feels immediate.
const searchChunkBytes int64 = 4 << 20

var newline = []byte{'\n'}

// blobMatch is a byte range in the blob itself, as opposed to
// ui.SearchMatch which is relative to whatever buffer was searched.
type blobMatch struct {
	Start int64
	End   int64
}

// scanOutcome says why a scan stopped. The three cases map onto three
// different messages and three different meanings for the next n.
type scanOutcome int

const (
	// scanFound: match is populated.
	scanFound scanOutcome = iota
	// scanBudgetSpent: the byte budget ran out; resumeAt says where to
	// pick up so the next attempt does not pay for the same bytes twice.
	scanBudgetSpent
	// scanHitEnd: reached the end (or start) of the blob without a match.
	scanHitEnd
)

type scanResult struct {
	outcome  scanOutcome
	match    blobMatch
	resumeAt int64
	// newlines is how many line breaks lie between where the scan
	// started and where it stopped — the match, or resumeAt. The scan
	// reads those bytes anyway, and the count is what tells a hit far
	// from the loaded window its line number.
	newlines  int
	bytesRead int64
}

// rangeReader is the slice of the blob service the scanner needs. Narrow
// on purpose: it keeps the scan testable against a synthetic blob, the
// same way uploader does for uploads.
type rangeReader interface {
	ReadBlobRange(ctx context.Context, offset, count int64) ([]byte, error)
}

type blobScanner struct {
	reader rangeReader
	// chunk is the read size. Zero means searchChunkBytes; tests set it
	// small so chunk-boundary behavior is reachable.
	chunk int64
}

func (s blobScanner) chunkSize() int64 {
	if s.chunk > 0 {
		return s.chunk
	}
	return searchChunkBytes
}

// forward scans from `from` towards the end of the blob.
//
// Chunks are trimmed back to the last newline so a line is never split
// across two reads, and the trimmed tail is re-read as the head of the
// next chunk. That is what keeps a match spanning a chunk edge findable
// with a regex, whose match length has no bound to overlap by.
//
// The first read starts one byte before `from`. A search usually starts
// in the middle of a line, and whether ^ or \b hold at that position
// depends on the byte in front of it.
//
// A line longer than one chunk cannot be kept whole. Such a chunk is
// searched as-is and the scan moves past it, so a match straddling that
// forced split is missed. It takes a multi-megabyte line with no newline
// to hit; the alternative is unbounded buffering.
func (s blobScanner) forward(ctx context.Context, p ui.SearchPattern, from, blobSize, budget int64) (scanResult, error) {
	res := scanResult{outcome: scanHitEnd}
	if !p.Valid() || blobSize <= 0 {
		return res, nil
	}
	if from < 0 {
		from = 0
	}

	chunk := s.chunkSize()
	for pos := from; pos < blobSize; {
		if err := ctx.Err(); err != nil {
			return res, err
		}
		if res.bytesRead >= budget {
			res.outcome = scanBudgetSpent
			res.resumeAt = pos
			return res, nil
		}

		lead := int64(0)
		if pos == from && from > 0 {
			lead = 1
		}
		count := chunk
		if pos+count > blobSize {
			count = blobSize - pos
		}
		data, err := s.reader.ReadBlobRange(ctx, pos-lead, count+lead)
		if err != nil {
			return res, err
		}
		if int64(len(data)) <= lead {
			break
		}
		res.bytesRead += int64(len(data))

		// Trim to the last newline unless this chunk runs to the end of
		// the blob, where there is nothing left to carry over.
		searchable := data
		advance := int64(len(data)) - lead
		if pos-lead+int64(len(data)) < blobSize {
			if idx := int64(bytes.LastIndexByte(data, '\n')); idx >= lead {
				searchable = data[:idx+1]
				advance = idx + 1 - lead
			}
		}

		if loc := ui.SearchBufferForward(p, searchable, int(lead)); loc != nil {
			res.outcome = scanFound
			res.match = blobMatch{Start: pos - lead + int64(loc.Start), End: pos - lead + int64(loc.End)}
			res.newlines += bytes.Count(searchable[lead:loc.Start], newline)
			return res, nil
		}

		res.newlines += bytes.Count(searchable[lead:], newline)
		pos += advance
	}

	return res, nil
}

// backward scans from `from` towards the start of the blob, returning
// the last match that begins before it.
//
// Chunks are aligned forward to the first newline so a read never begins
// mid-line; the skipped head becomes the tail of the next (earlier)
// chunk, which is why hi moves to the aligned low bound rather than the
// requested one.
//
// The first read runs one byte past `from`, for the same reason forward
// starts one byte early: $ and \b at the last position searched depend
// on the byte that follows it.
func (s blobScanner) backward(ctx context.Context, p ui.SearchPattern, from, budget int64) (scanResult, error) {
	res := scanResult{outcome: scanHitEnd}
	if !p.Valid() || from <= 0 {
		return res, nil
	}

	chunk := s.chunkSize()
	for hi := from; hi > 0; {
		if err := ctx.Err(); err != nil {
			return res, err
		}
		if res.bytesRead >= budget {
			res.outcome = scanBudgetSpent
			res.resumeAt = hi
			return res, nil
		}

		lo := hi - chunk
		if lo < 0 {
			lo = 0
		}
		tail := int64(0)
		if hi == from {
			tail = 1
		}
		data, err := s.reader.ReadBlobRange(ctx, lo, hi-lo+tail)
		if err != nil {
			return res, err
		}
		if len(data) == 0 {
			break
		}
		res.bytesRead += int64(len(data))

		// limit is where hi falls in data; anything after it is the one
		// byte of context, when the blob had one to give.
		limit := int(hi - lo)
		if limit > len(data) {
			limit = len(data)
		}

		// Don't begin mid-line: leave everything before the first newline
		// to the next chunk.
		skip := 0
		if lo > 0 {
			if idx := bytes.IndexByte(data[:limit], '\n'); idx >= 0 {
				skip = idx + 1
			}
		}

		if loc := ui.SearchBufferBackward(p, data[skip:], limit-skip); loc != nil {
			start := skip + loc.Start
			res.outcome = scanFound
			res.match = blobMatch{Start: lo + int64(start), End: lo + int64(skip+loc.End)}
			res.newlines += bytes.Count(data[start:limit], newline)
			return res, nil
		}

		next := lo + int64(skip)
		if next >= hi {
			// The only newline was the chunk's last byte, so aligning
			// left nothing to search and would not move hi; step back
			// over the chunk instead.
			next = lo
		}
		res.newlines += bytes.Count(data[next-lo:limit], newline)
		hi = next
	}

	return res, nil
}
