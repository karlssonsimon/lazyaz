package ui

import "testing"

type fakeMatcher struct {
	keys map[string]struct{}
}

func (m fakeMatcher) Matches(key string) bool {
	_, ok := m.keys[key]
	return ok
}

func TestShouldQuit(t *testing.T) {
	quit := fakeMatcher{keys: map[string]struct{}{"q": {}, "ctrl+c": {}}}

	tests := []struct {
		name         string
		key          string
		filterActive bool
		want         bool
	}{
		{name: "ctrl+c always quits", key: "ctrl+c", filterActive: true, want: true},
		{name: "q blocked while filtering", key: "q", filterActive: true, want: false},
		{name: "q quits when not filtering", key: "q", filterActive: false, want: true},
		{name: "other key does not quit", key: "x", filterActive: false, want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := ShouldQuit(tc.key, quit, tc.filterActive); got != tc.want {
				t.Fatalf("expected %t, got %t", tc.want, got)
			}
		})
	}
}
