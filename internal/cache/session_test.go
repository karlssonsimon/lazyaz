package cache

import (
	"reflect"
	"testing"
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

func TestFetchSession_ReseedDoesNotTrackSeen(t *testing.T) {
	// Simulates a cached emit arriving at the start of a fetch. The cached
	// items must NOT count as "seen" — otherwise the sweep at Finalize
	// would keep items that no longer exist server-side.
	s := NewFetchSession[item](nil, 1, itemKey)

	// Cached emit delivers these items.
	s.Reseed([]item{{ID: "a", Value: 1}, {ID: "b", Value: 2}, {ID: "c", Value: 3}})

	// Mid-stream: session is showing the cached items, user sees them.
	got := s.Items()
	want := []item{{ID: "a", Value: 1}, {ID: "b", Value: 2}, {ID: "c", Value: 3}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("after Reseed items = %v, want %v", got, want)
	}

	// Network page arrives: "b" was deleted server-side, "a" got updated,
	// "d" is new.
	s.Apply([]item{{ID: "a", Value: 10}, {ID: "c", Value: 3}, {ID: "d", Value: 4}})

	// During streaming, b is still visible (union of cached + seen).
	mid := s.Items()
	wantMid := []item{{ID: "a", Value: 10}, {ID: "b", Value: 2}, {ID: "c", Value: 3}, {ID: "d", Value: 4}}
	if !reflect.DeepEqual(mid, wantMid) {
		t.Fatalf("mid-stream items = %v, want %v", mid, wantMid)
	}

	// Finalize sweeps b because Reseed did NOT mark it as seen.
	final := s.Finalize()
	wantFinal := []item{{ID: "a", Value: 10}, {ID: "c", Value: 3}, {ID: "d", Value: 4}}
	if !reflect.DeepEqual(final, wantFinal) {
		t.Fatalf("finalized = %v, want %v", final, wantFinal)
	}
}

func TestFetchSession_ReseedPreservesAlreadySeen(t *testing.T) {
	// Reseed should not wipe the seen set — any pages already applied
	// stay authoritative.
	s := NewFetchSession[item](nil, 1, itemKey)

	s.Apply([]item{{ID: "a", Value: 1}}) // a is now seen
	s.Reseed([]item{{ID: "a", Value: 99}, {ID: "b", Value: 2}})

	// After reseed, items reflect the reseed; but "a" is still in seen.
	// "b" was not applied, so it's not seen.
	final := s.Finalize()
	want := []item{{ID: "a", Value: 99}} // b gets swept
	if !reflect.DeepEqual(final, want) {
		t.Fatalf("finalized = %v, want %v", final, want)
	}
}

func TestFetchSession_ReseedCopiesSlice(t *testing.T) {
	s := NewFetchSession[item](nil, 1, itemKey)
	src := []item{{ID: "a", Value: 1}}
	s.Reseed(src)
	src[0].Value = 999
	if s.Items()[0].Value != 1 {
		t.Fatalf("session item mutated by caller's slice; got %v", s.Items())
	}
}
