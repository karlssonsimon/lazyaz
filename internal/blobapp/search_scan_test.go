package blobapp

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/karlssonsimon/lazyaz/internal/ui"
)

// fakeRangeReader serves a synthetic blob, recording what was asked for
// so the tests can assert on how much the scanner actually fetched.
type fakeRangeReader struct {
	data      []byte
	reads     int
	bytesRead int64
	err       error
}

func (f *fakeRangeReader) ReadBlobRange(ctx context.Context, offset, count int64) ([]byte, error) {
	if f.err != nil {
		return nil, f.err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	f.reads++
	if offset < 0 || offset >= int64(len(f.data)) {
		return nil, nil
	}
	end := offset + count
	if end > int64(len(f.data)) {
		end = int64(len(f.data))
	}
	out := f.data[offset:end]
	f.bytesRead += int64(len(out))
	return out, nil
}

func mustPattern(t *testing.T, query string) ui.SearchPattern {
	t.Helper()
	p, err := ui.CompileSearchPattern(query)
	if err != nil {
		t.Fatalf("CompileSearchPattern(%q): %v", query, err)
	}
	return p
}

// linesBlob builds a blob of n numbered lines, with needle planted on
// the given line. Returns the blob and the byte offset of the needle.
func linesBlob(n, needleLine int, needle string) ([]byte, int64) {
	var sb strings.Builder
	var offset int64 = -1
	for i := 0; i < n; i++ {
		if i == needleLine {
			offset = int64(sb.Len()) + int64(len(fmt.Sprintf("line %04d ", i)))
			fmt.Fprintf(&sb, "line %04d %s\n", i, needle)
			continue
		}
		fmt.Fprintf(&sb, "line %04d filler\n", i)
	}
	return []byte(sb.String()), offset
}

func TestScanForwardFindsMatchInFirstChunk(t *testing.T) {
	data, want := linesBlob(50, 3, "NEEDLE")
	r := &fakeRangeReader{data: data}
	s := blobScanner{reader: r, chunk: 512}

	got, err := s.forward(context.Background(), mustPattern(t, "NEEDLE"), 0, int64(len(data)), 1<<20)
	if err != nil {
		t.Fatalf("forward: %v", err)
	}
	if got.outcome != scanFound {
		t.Fatalf("outcome = %v, want scanFound", got.outcome)
	}
	if got.match.Start != want {
		t.Errorf("match at %d, want %d", got.match.Start, want)
	}
}

// The whole point of streaming: a match far past the initially loaded
// window still gets found.
func TestScanForwardFindsMatchManyChunksAway(t *testing.T) {
	data, want := linesBlob(2000, 1900, "NEEDLE")
	r := &fakeRangeReader{data: data}
	s := blobScanner{reader: r, chunk: 256}

	got, err := s.forward(context.Background(), mustPattern(t, "NEEDLE"), 0, int64(len(data)), 1<<20)
	if err != nil {
		t.Fatalf("forward: %v", err)
	}
	if got.outcome != scanFound {
		t.Fatalf("outcome = %v, want scanFound", got.outcome)
	}
	if got.match.Start != want {
		t.Errorf("match at %d, want %d", got.match.Start, want)
	}
	if r.reads < 2 {
		t.Errorf("only %d reads — the test is not exercising multiple chunks", r.reads)
	}
}

// Line-aligned chunking exists so a match is never lost at a chunk
// edge. Sweep the needle across every offset near a boundary.
func TestScanForwardFindsMatchAtEveryChunkBoundary(t *testing.T) {
	const chunk = 128

	for line := 0; line < 40; line++ {
		data, want := linesBlob(60, line, "NEEDLE")
		r := &fakeRangeReader{data: data}
		s := blobScanner{reader: r, chunk: chunk}

		got, err := s.forward(context.Background(), mustPattern(t, "NEEDLE"), 0, int64(len(data)), 1<<20)
		if err != nil {
			t.Fatalf("line %d: forward: %v", line, err)
		}
		if got.outcome != scanFound {
			t.Fatalf("line %d: outcome = %v, want scanFound", line, got.outcome)
		}
		if got.match.Start != want {
			t.Errorf("line %d: match at %d, want %d", line, got.match.Start, want)
		}
	}
}

func TestScanForwardReportsEndOfBlob(t *testing.T) {
	data, _ := linesBlob(100, -1, "")
	r := &fakeRangeReader{data: data}
	s := blobScanner{reader: r, chunk: 256}

	got, err := s.forward(context.Background(), mustPattern(t, "NEEDLE"), 0, int64(len(data)), 1<<20)
	if err != nil {
		t.Fatalf("forward: %v", err)
	}
	if got.outcome != scanHitEnd {
		t.Errorf("outcome = %v, want scanHitEnd", got.outcome)
	}
}

// The budget bounds one search. Hitting it must report where to resume
// so n picks up without re-reading what was already paid for.
func TestScanForwardStopsAtBudgetAndResumes(t *testing.T) {
	data, want := linesBlob(2000, 1900, "NEEDLE")
	pattern := mustPattern(t, "NEEDLE")

	r := &fakeRangeReader{data: data}
	s := blobScanner{reader: r, chunk: 256}

	first, err := s.forward(context.Background(), pattern, 0, int64(len(data)), 1024)
	if err != nil {
		t.Fatalf("forward: %v", err)
	}
	if first.outcome != scanBudgetSpent {
		t.Fatalf("outcome = %v, want scanBudgetSpent", first.outcome)
	}
	if first.resumeAt <= 0 {
		t.Fatalf("resumeAt = %d, want a positive offset", first.resumeAt)
	}
	if first.bytesRead > 1024+256 {
		t.Errorf("read %d bytes on a 1024 budget — overshoot is larger than one chunk", first.bytesRead)
	}

	// Resuming with a generous budget must find the needle.
	second, err := s.forward(context.Background(), pattern, first.resumeAt, int64(len(data)), 1<<20)
	if err != nil {
		t.Fatalf("resume: %v", err)
	}
	if second.outcome != scanFound {
		t.Fatalf("resumed outcome = %v, want scanFound", second.outcome)
	}
	if second.match.Start != want {
		t.Errorf("resumed match at %d, want %d", second.match.Start, want)
	}
}

func TestScanBackwardFindsPrecedingMatch(t *testing.T) {
	// Needles on lines 5 and 40; searching back from line 60 must land
	// on the later of the two.
	var sb strings.Builder
	offsets := map[int]int64{}
	for i := 0; i < 60; i++ {
		if i == 5 || i == 40 {
			offsets[i] = int64(sb.Len()) + int64(len(fmt.Sprintf("line %04d ", i)))
			fmt.Fprintf(&sb, "line %04d NEEDLE\n", i)
			continue
		}
		fmt.Fprintf(&sb, "line %04d filler\n", i)
	}
	data := []byte(sb.String())

	r := &fakeRangeReader{data: data}
	s := blobScanner{reader: r, chunk: 128}

	got, err := s.backward(context.Background(), mustPattern(t, "NEEDLE"), int64(len(data)), 1<<20)
	if err != nil {
		t.Fatalf("backward: %v", err)
	}
	if got.outcome != scanFound {
		t.Fatalf("outcome = %v, want scanFound", got.outcome)
	}
	if got.match.Start != offsets[40] {
		t.Errorf("match at %d, want %d (the later needle)", got.match.Start, offsets[40])
	}

	// From just before the later needle, the earlier one is next.
	got, err = s.backward(context.Background(), mustPattern(t, "NEEDLE"), offsets[40], 1<<20)
	if err != nil {
		t.Fatalf("backward: %v", err)
	}
	if got.outcome != scanFound || got.match.Start != offsets[5] {
		t.Errorf("second backward search gave %v at %d, want scanFound at %d", got.outcome, got.match.Start, offsets[5])
	}
}

func TestScanBackwardReportsStartOfBlob(t *testing.T) {
	data, _ := linesBlob(100, -1, "")
	r := &fakeRangeReader{data: data}
	s := blobScanner{reader: r, chunk: 256}

	got, err := s.backward(context.Background(), mustPattern(t, "NEEDLE"), int64(len(data)), 1<<20)
	if err != nil {
		t.Fatalf("backward: %v", err)
	}
	if got.outcome != scanHitEnd {
		t.Errorf("outcome = %v, want scanHitEnd", got.outcome)
	}
}

func TestScanForwardHonorsCancellation(t *testing.T) {
	data, _ := linesBlob(5000, 4900, "NEEDLE")
	r := &fakeRangeReader{data: data}
	s := blobScanner{reader: r, chunk: 128}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := s.forward(ctx, mustPattern(t, "NEEDLE"), 0, int64(len(data)), 1<<20)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}

func TestScanForwardPropagatesReadError(t *testing.T) {
	boom := errors.New("range read failed")
	r := &fakeRangeReader{data: []byte("some data\n"), err: boom}
	s := blobScanner{reader: r, chunk: 128}

	_, err := s.forward(context.Background(), mustPattern(t, "NEEDLE"), 0, 10, 1<<20)
	if !errors.Is(err, boom) {
		t.Errorf("err = %v, want the read error", err)
	}
}

// A line longer than one chunk cannot be kept whole. The scan must still
// terminate and still find matches that sit inside a chunk, even though
// one straddling the forced split is knowingly missed.
func TestScanForwardTerminatesOnLineLongerThanChunk(t *testing.T) {
	data := []byte(strings.Repeat("x", 5000) + "NEEDLE" + strings.Repeat("y", 5000))
	r := &fakeRangeReader{data: data}
	s := blobScanner{reader: r, chunk: 128}

	got, err := s.forward(context.Background(), mustPattern(t, "x{50}"), 0, int64(len(data)), 1<<20)
	if err != nil {
		t.Fatalf("forward: %v", err)
	}
	if got.outcome != scanFound {
		t.Errorf("outcome = %v, want scanFound on a chunk-local match", got.outcome)
	}
	if r.reads > 200 {
		t.Errorf("%d reads for a 10KB blob at 128-byte chunks — the scan is not advancing", r.reads)
	}
}
