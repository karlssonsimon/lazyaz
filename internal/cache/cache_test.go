package cache

import "testing"

func TestKey(t *testing.T) {
	tests := []struct {
		name  string
		parts []string
		want  string
	}{
		{name: "empty", parts: nil, want: ""},
		{name: "single", parts: []string{"a"}, want: "a"},
		{name: "two", parts: []string{"a", "b"}, want: "a\x00b"},
		{name: "three", parts: []string{"a", "b", "c"}, want: "a\x00b\x00c"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := Key(tc.parts...); got != tc.want {
				t.Fatalf("Key(%v) = %q, want %q", tc.parts, got, tc.want)
			}
		})
	}
}

func TestMapGetSet(t *testing.T) {
	m := NewMap[string]()

	if _, ok := m.Get("missing"); ok {
		t.Fatal("expected miss for empty map")
	}

	m.Set("k", []string{"a", "b"})
	got, ok := m.Get("k")
	if !ok {
		t.Fatal("expected hit after Set")
	}
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("got %v, want [a b]", got)
	}

	m.Set("k", []string{"x"})
	got, _ = m.Get("k")
	if len(got) != 1 || got[0] != "x" {
		t.Fatalf("got %v after overwrite, want [x]", got)
	}
}

func TestMapSatisfiesStore(t *testing.T) {
	var _ Store[string] = NewMap[string]()
}
