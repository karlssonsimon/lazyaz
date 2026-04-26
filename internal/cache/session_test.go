package cache

import (
	"reflect"
	"strconv"
	"testing"
	"time"
)

type item struct {
	ID    string
	Value int
}

func itemKey(i item) string { return i.ID }

func TestFetchSession_AppliesNewItems(t *testing.T) {
	s := NewFetchSession(nil, 1, itemKey)

	s.Apply([]item{{ID: "a", Value: 1}, {ID: "b", Value: 2}})

	got := s.Items()
	want := []item{{ID: "a", Value: 1}, {ID: "b", Value: 2}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("items = %v, want %v", got, want)
	}
}

func TestFetchSession_UpdatesExistingInPlace(t *testing.T) {
	current := []item{{ID: "a", Value: 1}, {ID: "b", Value: 2}, {ID: "c", Value: 3}}
	s := NewFetchSession(current, 1, itemKey)

	// "b" gets updated, others stay where they are.
	s.Apply([]item{{ID: "b", Value: 99}})

	got := s.Items()
	want := []item{{ID: "a", Value: 1}, {ID: "b", Value: 99}, {ID: "c", Value: 3}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("items = %v, want %v", got, want)
	}
}

func TestFetchSession_AppendsUnknownItems(t *testing.T) {
	current := []item{{ID: "a", Value: 1}}
	s := NewFetchSession(current, 1, itemKey)

	s.Apply([]item{{ID: "b", Value: 2}, {ID: "c", Value: 3}})

	got := s.Items()
	want := []item{{ID: "a", Value: 1}, {ID: "b", Value: 2}, {ID: "c", Value: 3}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("items = %v, want %v", got, want)
	}
}

func TestFetchSession_MixedPageUpdatesAndAppends(t *testing.T) {
	current := []item{{ID: "a", Value: 1}, {ID: "b", Value: 2}}
	s := NewFetchSession(current, 1, itemKey)

	// Page updates "a" and adds "c".
	s.Apply([]item{{ID: "a", Value: 10}, {ID: "c", Value: 3}})

	got := s.Items()
	want := []item{{ID: "a", Value: 10}, {ID: "b", Value: 2}, {ID: "c", Value: 3}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("items = %v, want %v", got, want)
	}
}

func TestFetchSession_MultiplePagesAccumulate(t *testing.T) {
	s := NewFetchSession[item](nil, 1, itemKey)

	s.Apply([]item{{ID: "a", Value: 1}})
	s.Apply([]item{{ID: "b", Value: 2}})
	s.Apply([]item{{ID: "a", Value: 11}, {ID: "c", Value: 3}})

	got := s.Items()
	want := []item{{ID: "a", Value: 11}, {ID: "b", Value: 2}, {ID: "c", Value: 3}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("items = %v, want %v", got, want)
	}
}

func TestFetchSession_FinalizeSweepsUnseen(t *testing.T) {
	current := []item{{ID: "a", Value: 1}, {ID: "b", Value: 2}, {ID: "c", Value: 3}}
	s := NewFetchSession(current, 1, itemKey)

	// Only "a" and "c" arrive; "b" was deleted server-side.
	s.Apply([]item{{ID: "a", Value: 10}, {ID: "c", Value: 30}})

	final := s.Finalize()
	want := []item{{ID: "a", Value: 10}, {ID: "c", Value: 30}}
	if !reflect.DeepEqual(final, want) {
		t.Fatalf("finalized = %v, want %v", final, want)
	}
}

func TestFetchSession_FinalizeEmptyKeepsNothing(t *testing.T) {
	current := []item{{ID: "a", Value: 1}, {ID: "b", Value: 2}}
	s := NewFetchSession(current, 1, itemKey)

	// No pages arrived — everything got deleted.
	final := s.Finalize()
	if len(final) != 0 {
		t.Fatalf("finalized = %v, want empty", final)
	}
}

func TestFetchSession_FinalizeWithNoChangesKeepsEverything(t *testing.T) {
	current := []item{{ID: "a", Value: 1}, {ID: "b", Value: 2}}
	s := NewFetchSession(current, 1, itemKey)

	// Server returned exactly the same things.
	s.Apply(current)

	final := s.Finalize()
	if !reflect.DeepEqual(final, current) {
		t.Fatalf("finalized = %v, want %v", final, current)
	}
}

func TestFetchSession_GenToken(t *testing.T) {
	s := NewFetchSession[item](nil, 42, itemKey)
	if got := s.Gen(); got != 42 {
		t.Fatalf("Gen() = %d, want 42", got)
	}
}

func TestFetchSession_ItemsDuringStreamingIsUnionOfOldAndNew(t *testing.T) {
	// This is the key UX guarantee: during a multi-page stream, the user
	// sees their old data plus whatever has arrived so far. Missing items
	// stay visible until Finalize.
	current := []item{{ID: "a", Value: 1}, {ID: "b", Value: 2}, {ID: "c", Value: 3}}
	s := NewFetchSession(current, 1, itemKey)

	// First page: only "a" and a new "d" arrive.
	s.Apply([]item{{ID: "a", Value: 10}, {ID: "d", Value: 4}})

	// During streaming, "b" and "c" are still visible.
	got := s.Items()
	want := []item{{ID: "a", Value: 10}, {ID: "b", Value: 2}, {ID: "c", Value: 3}, {ID: "d", Value: 4}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("mid-stream items = %v, want %v", got, want)
	}

	// Once Finalize runs, "b" and "c" disappear.
	final := s.Finalize()
	wantFinal := []item{{ID: "a", Value: 10}, {ID: "d", Value: 4}}
	if !reflect.DeepEqual(final, wantFinal) {
		t.Fatalf("finalized = %v, want %v", final, wantFinal)
	}
}

func TestFetchSession_CopiesCurrentSlice(t *testing.T) {
	// Seeding with a slice must not retain the caller's backing array.
	current := []item{{ID: "a", Value: 1}}
	s := NewFetchSession(current, 1, itemKey)

	current[0].Value = 999 // mutate caller's slice

	got := s.Items()
	if got[0].Value != 1 {
		t.Fatalf("session item mutated by caller; got %v", got)
	}
}

func TestFetchSession_EmptyCurrent(t *testing.T) {
	s := NewFetchSession[item](nil, 1, itemKey)
	if items := s.Items(); len(items) != 0 {
		t.Fatalf("Items() = %v, want empty", items)
	}
	if final := s.Finalize(); len(final) != 0 {
		t.Fatalf("Finalize() = %v, want empty", final)
	}
}

// TestFetchSession_LargeStreamCompletesQuickly is a perf regression test
// for the bug that froze the UI when load-all was run on a 100k+ blob
// container. The original linear-scan Apply was O(N²) per stream and
// would take minutes; the indexed implementation should finish in well
// under a second.
//
// This test does NOT use testing.B because we want it to fail loudly in
// CI rather than show up as a benchmark regression nobody runs.
func TestFetchSession_LargeStreamCompletesQuickly(t *testing.T) {
	if testing.Short() {
		t.Skip("perf regression; use go test -short to skip")
	}

	const totalItems = 100_000
	const pageSize = 5_000

	s := NewFetchSession[item](nil, 1, itemKey)

	start := time.Now()
	for offset := 0; offset < totalItems; offset += pageSize {
		page := make([]item, pageSize)
		for i := range page {
			page[i] = item{ID: strconv.Itoa(offset + i), Value: offset + i}
		}
		s.Apply(page)
	}
	elapsed := time.Since(start)

	if got := len(s.Items()); got != totalItems {
		t.Fatalf("got %d items, want %d", got, totalItems)
	}
	// Generous bound. With the O(N²) bug this takes minutes; with the
	// indexed implementation it's typically <100ms.
	if elapsed > 2*time.Second {
		t.Errorf("Apply too slow: streaming %d items took %s (regression of O(N²) bug?)",
			totalItems, elapsed)
	}
	t.Logf("streamed %d items in %d pages in %s", totalItems, totalItems/pageSize, elapsed)
}
