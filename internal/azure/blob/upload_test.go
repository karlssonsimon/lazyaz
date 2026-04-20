package blob

import (
	"context"
	"strings"
	"testing"
)

// Compile-time assertion that ExistingBlobs exists with the expected
// signature. Real-behavior tests require Azure so are omitted per the
// codebase convention.
var _ func(*Service, context.Context, Account, string, []string) (map[string]struct{}, error) = (*Service).ExistingBlobs

// Compile-time assertion that UploadBlob exists with the expected signature.
var _ func(*Service, context.Context, Account, string, string, string, func(int64)) error = (*Service).UploadBlob

func TestExistingBlobsZeroNames(t *testing.T) {
	s := &Service{}
	got, err := s.ExistingBlobs(context.Background(), Account{}, "c", nil)
	if err != nil {
		t.Fatalf("ExistingBlobs with zero names returned err: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty set, got %d entries", len(got))
	}
}

func TestProgressReaderReportsCumulativeBytes(t *testing.T) {
	var seen []int64
	r := &progressReader{
		r:         strings.NewReader("hello world"),
		onAdvance: func(n int64) { seen = append(seen, n) },
	}

	buf := make([]byte, 4)
	if _, err := r.Read(buf); err != nil {
		t.Fatalf("first read: %v", err)
	}
	if _, err := r.Read(buf); err != nil {
		t.Fatalf("second read: %v", err)
	}
	if _, err := r.Read(buf); err != nil {
		t.Fatalf("third read: %v", err)
	}

	// strings.NewReader returns up to len(buf) bytes; "hello world" = 11.
	// Expected cumulative: 4, 8, 11.
	want := []int64{4, 8, 11}
	if len(seen) != len(want) {
		t.Fatalf("want %d callbacks, got %d (values=%v)", len(want), len(seen), seen)
	}
	for i, v := range seen {
		if v != want[i] {
			t.Fatalf("callback %d: want %d, got %d (values=%v)", i, want[i], v, seen)
		}
	}
}

func TestProgressReaderIgnoresNilCallback(t *testing.T) {
	r := &progressReader{r: strings.NewReader("abc")}
	buf := make([]byte, 8)
	_, err := r.Read(buf)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	// Should not panic.
}
