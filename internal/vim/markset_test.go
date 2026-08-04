package vim

import (
	"reflect"
	"testing"
)

func TestMarkSetToggle(t *testing.T) {
	s := NewMarkSet()

	if !s.Toggle("a") {
		t.Fatal("first toggle should mark")
	}
	if !s.Contains("a") || s.Len() != 1 {
		t.Fatalf("after mark: contains=%v len=%d", s.Contains("a"), s.Len())
	}
	if s.Toggle("a") {
		t.Fatal("second toggle should unmark")
	}
	if s.Contains("a") || s.Len() != 0 {
		t.Fatalf("after unmark: contains=%v len=%d", s.Contains("a"), s.Len())
	}
}

// The delegates hold the backing map by reference so selection changes
// render without re-setting the delegate. Clear must therefore empty
// the map in place, never replace it.
func TestMarkSetClearPreservesMapIdentity(t *testing.T) {
	s := NewMarkSet()
	s.Add("a")
	s.Add("b")

	held := s.Items() // what a delegate would hold
	s.Clear()
	if len(held) != 0 {
		t.Fatalf("delegate's view has %d entries after Clear, want 0", len(held))
	}

	s.Add("c")
	if _, ok := held["c"]; !ok {
		t.Fatal("delegate's view missed a mark added after Clear — map identity was lost")
	}
}

func TestMarkSetSorted(t *testing.T) {
	s := NewMarkSet()
	for _, k := range []string{"b", "c", "a"} {
		s.Add(k)
	}
	if got := s.Sorted(); !reflect.DeepEqual(got, []string{"a", "b", "c"}) {
		t.Errorf("Sorted() = %v", got)
	}

	var empty MarkSet
	if got := empty.Sorted(); got != nil {
		t.Errorf("zero-value Sorted() = %v, want nil", got)
	}
}

// The zero value is read-safe: sbapp's currentMarks returns "no scope"
// as an empty set that only gets read.
func TestMarkSetZeroValueReads(t *testing.T) {
	var s MarkSet
	if s.Contains("a") || s.Len() != 0 || s.Items() != nil {
		t.Errorf("zero value: contains=%v len=%d items=%v", s.Contains("a"), s.Len(), s.Items())
	}
}
