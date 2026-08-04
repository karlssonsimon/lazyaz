package vim

import "sort"

// MarkSet is a set of marked item keys. It wraps the map the apps used
// to hold directly, and deliberately exposes it: the mark delegates
// keep the map by reference so a toggle renders on the next frame
// without re-setting the delegate (SetDelegate recomputes pagination
// over every visible item — too expensive at 200k rows).
//
// MarkSet is a value type over a shared map, same semantics as passing
// the map itself: copies of the model see the same marks.
type MarkSet struct {
	m map[string]struct{}
}

// NewMarkSet returns a set backed by a live map. Use this rather than
// the zero value anywhere marks will be added — the zero value is
// read-only (safe for Contains/Len/Items, which is what "no scope
// selected" returns).
func NewMarkSet() MarkSet {
	return MarkSet{m: make(map[string]struct{})}
}

// Toggle flips the mark on key and reports whether it is now marked.
func (s MarkSet) Toggle(key string) bool {
	if _, ok := s.m[key]; ok {
		delete(s.m, key)
		return false
	}
	s.m[key] = struct{}{}
	return true
}

// Add marks key.
func (s MarkSet) Add(key string) {
	s.m[key] = struct{}{}
}

// Contains reports whether key is marked.
func (s MarkSet) Contains(key string) bool {
	_, ok := s.m[key]
	return ok
}

// Len is the number of marked keys.
func (s MarkSet) Len() int {
	return len(s.m)
}

// Clear unmarks everything, in place — the backing map's identity is
// what the delegates hold, so it must never be replaced.
func (s MarkSet) Clear() {
	for k := range s.m {
		delete(s.m, k)
	}
}

// Sorted returns the marked keys in order. Nil when nothing is marked.
func (s MarkSet) Sorted() []string {
	if len(s.m) == 0 {
		return nil
	}
	keys := make([]string, 0, len(s.m))
	for k := range s.m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// Items exposes the backing map for delegate wiring. Mutations through
// the set are visible to every holder of this map.
func (s MarkSet) Items() map[string]struct{} {
	return s.m
}
