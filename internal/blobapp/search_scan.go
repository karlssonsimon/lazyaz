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
	outcome   scanOutcome
	match     blobMatch
	resumeAt  int64
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

		count := chunk
		if pos+count > blobSize {
			count = blobSize - pos
		}
		data, err := s.reader.ReadBlobRange(ctx, pos, count)
		if err != nil {
			return res, err
		}
		if len(data) == 0 {
			break
		}
		res.bytesRead += int64(len(data))

		// Trim to the last newline unless this chunk runs to the end of
		// the blob, where there is nothing left to carry over.
		searchable := data
		advance := int64(len(data))
		if pos+int64(len(data)) < blobSize {
			if idx := bytes.LastIndexByte(data, '\n'); idx >= 0 {
				searchable = data[:idx+1]
				advance = int64(idx + 1)
			}
		}

		if loc := ui.SearchBufferForward(p, searchable, 0); loc != nil {
			res.outcome = scanFound
			res.match = blobMatch{Start: pos + int64(loc.Start), End: pos + int64(loc.End)}
			return res, nil
		}

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
		data, err := s.reader.ReadBlobRange(ctx, lo, hi-lo)
		if err != nil {
			return res, err
		}
		if len(data) == 0 {
			break
		}
		res.bytesRead += int64(len(data))

		// Don't begin mid-line: drop everything before the first newline
		// and let the next chunk cover it.
		if lo > 0 {
			if idx := bytes.IndexByte(data, '\n'); idx >= 0 {
				lo += int64(idx + 1)
				data = data[idx+1:]
			}
		}

		if loc := ui.SearchBufferBackward(p, data, len(data)); loc != nil {
			res.outcome = scanFound
			res.match = blobMatch{Start: lo + int64(loc.Start), End: lo + int64(loc.End)}
			return res, nil
		}

		if lo >= hi {
			// No newline in a chunk that started mid-line would leave lo
			// unchanged and spin; step back explicitly.
			lo = hi - chunk
			if lo < 0 {
				lo = 0
			}
		}
		hi = lo
	}

	return res, nil
}
