package vim

import "testing"

// scanCounter wraps a find function and counts invocations, so the
// tests can assert the cache actually prevents scans.
type scanCounter struct {
	index map[string]int
	calls int
}

func (s *scanCounter) find(key string) int {
	s.calls++
	if idx, ok := s.index[key]; ok {
		return idx
	}
	return -1
}

func TestVisualStartStop(t *testing.T) {
	var v Visual
	if v.Active() {
		t.Fatal("zero Visual is active")
	}
	v.Start("blob-7")
	if !v.Active() || v.Anchor() != "blob-7" {
		t.Fatalf("after Start: active=%v anchor=%q", v.Active(), v.Anchor())
	}
	v.Stop()
	if v.Active() || v.Anchor() != "" {
		t.Fatalf("after Stop: active=%v anchor=%q", v.Active(), v.Anchor())
	}
}

func TestAnchorIdxCachesPerVersion(t *testing.T) {
	sc := &scanCounter{index: map[string]int{"a": 4}}
	var v Visual
	v.Start("a")

	if got := v.AnchorIdx(1, sc.find); got != 4 {
		t.Fatalf("AnchorIdx = %d, want 4", got)
	}
	if got := v.AnchorIdx(1, sc.find); got != 4 {
		t.Fatalf("cached AnchorIdx = %d, want 4", got)
	}
	if sc.calls != 1 {
		t.Fatalf("find ran %d times at one version, want 1", sc.calls)
	}

	// A new version means the visible set changed — re-resolve.
	sc.index["a"] = 9
	if got := v.AnchorIdx(2, sc.find); got != 9 {
		t.Fatalf("AnchorIdx after version bump = %d, want 9", got)
	}
	if sc.calls != 2 {
		t.Fatalf("find ran %d times across two versions, want 2", sc.calls)
	}
}

// An anchor filtered out of view resolves to -1, and that negative
// result is cached too — matching blobapp, which otherwise rescans on
// every keypress while a filter hides the anchor.
func TestAnchorIdxCachesNegativeResult(t *testing.T) {
	sc := &scanCounter{index: map[string]int{}}
	var v Visual
	v.Start("hidden")

	if got := v.AnchorIdx(1, sc.find); got != -1 {
		t.Fatalf("AnchorIdx = %d, want -1", got)
	}
	v.AnchorIdx(1, sc.find)
	if sc.calls != 1 {
		t.Fatalf("find ran %d times for a cached miss, want 1", sc.calls)
	}
}

// An empty anchor never scans: visual mode can start on an empty list.
func TestAnchorIdxEmptyAnchorSkipsScan(t *testing.T) {
	sc := &scanCounter{index: map[string]int{}}
	var v Visual
	v.Start("")

	if got := v.AnchorIdx(1, sc.find); got != -1 {
		t.Fatalf("AnchorIdx = %d, want -1", got)
	}
	if sc.calls != 0 {
		t.Fatalf("find ran %d times for an empty anchor, want 0", sc.calls)
	}
}

// Swapping ends is done with both indices already in hand, so the cache
// stays warm — the visible set did not change during a swap.
func TestSetAnchorWithIdxKeepsCacheWarm(t *testing.T) {
	sc := &scanCounter{index: map[string]int{"a": 2}}
	var v Visual
	v.Start("a")
	v.AnchorIdx(1, sc.find)

	v.SetAnchorWithIdx("b", 6, 1)
	if got := v.AnchorIdx(1, sc.find); got != 6 {
		t.Fatalf("AnchorIdx after swap = %d, want 6", got)
	}
	if v.Anchor() != "b" {
		t.Fatalf("anchor = %q, want b", v.Anchor())
	}
	if sc.calls != 1 {
		t.Fatalf("find ran %d times, want 1 (swap must not rescan)", sc.calls)
	}
}

func TestRange(t *testing.T) {
	tests := []struct {
		name     string
		anchor   string
		cursor   int
		wantLo   int
		wantHi   int
		wantOK   bool
		inactive bool
	}{
		{name: "anchor before cursor", anchor: "a", cursor: 7, wantLo: 4, wantHi: 7, wantOK: true},
		{name: "anchor after cursor", anchor: "z", cursor: 3, wantLo: 3, wantHi: 9, wantOK: true},
		{name: "anchor is cursor row", anchor: "a", cursor: 4, wantLo: 4, wantHi: 4, wantOK: true},
		{name: "absent anchor collapses to cursor", anchor: "gone", cursor: 5, wantLo: 5, wantHi: 5, wantOK: true},
		{name: "empty anchor collapses to cursor", anchor: "", cursor: 2, wantLo: 2, wantHi: 2, wantOK: true},
		{name: "inactive gives no range", anchor: "a", cursor: 5, inactive: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sc := &scanCounter{index: map[string]int{"a": 4, "z": 9}}
			var v Visual
			if !tt.inactive {
				v.Start(tt.anchor)
			}
			lo, hi, ok := v.Range(tt.cursor, 1, sc.find)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if !ok {
				return
			}
			if lo != tt.wantLo || hi != tt.wantHi {
				t.Errorf("range = [%d,%d], want [%d,%d]", lo, hi, tt.wantLo, tt.wantHi)
			}
		})
	}
}
