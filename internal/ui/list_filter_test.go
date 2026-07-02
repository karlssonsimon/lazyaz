package ui

import (
	"reflect"
	"testing"
)

func TestListFilterExtendedSyntax(t *testing.T) {
	targets := []string{"app.json", "app.tmp", "readme.md"}

	ranks := ListFilter("!.tmp$", targets)
	got := make([]int, len(ranks))
	for i, r := range ranks {
		got[i] = r.Index
	}
	if !reflect.DeepEqual(got, []int{0, 2}) {
		t.Fatalf("!.tmp$ ranks = %v, want indices [0 2]", got)
	}
}

func TestListFilterMatchedIndexesAreRunePositions(t *testing.T) {
	ranks := ListFilter("'json", []string{"app.json"})
	if len(ranks) != 1 {
		t.Fatalf("expected 1 rank, got %d", len(ranks))
	}
	if !reflect.DeepEqual(ranks[0].MatchedIndexes, []int{4, 5, 6, 7}) {
		t.Fatalf("MatchedIndexes = %v, want [4 5 6 7]", ranks[0].MatchedIndexes)
	}
}
