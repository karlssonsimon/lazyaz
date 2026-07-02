package fuzzy

import (
	"reflect"
	"testing"
)

func matchNames(t *testing.T, pattern string, candidates []string) []string {
	t.Helper()
	idx := Filter(pattern, candidates, func(s string) string { return s })
	out := make([]string, len(idx))
	for i, j := range idx {
		out[i] = candidates[j]
	}
	return out
}

func TestExtendedSyntax(t *testing.T) {
	names := []string{
		"data/logs/2026/app.json",
		"data/logs/2026/app.tmp",
		"backup/app.json",
		"data/metrics/cpu.parquet",
	}

	cases := []struct {
		pattern string
		want    []string
	}{
		// Exact substring: no fuzzy gaps allowed.
		{"'logs/2026", []string{"data/logs/2026/app.json", "data/logs/2026/app.tmp"}},
		// Prefix.
		{"^backup", []string{"backup/app.json"}},
		// Suffix.
		{".json$", []string{"data/logs/2026/app.json", "backup/app.json"}},
		// Negation is exact: everything not containing "app".
		{"!app", []string{"data/metrics/cpu.parquet"}},
		// Negated suffix.
		{"!.tmp$ ^data", []string{"data/logs/2026/app.json", "data/metrics/cpu.parquet"}},
		// AND of terms.
		{"^data .json$", []string{"data/logs/2026/app.json"}},
		// OR set.
		{"'parquet | 'tmp", []string{"data/logs/2026/app.tmp", "data/metrics/cpu.parquet"}},
		// OR combined with AND: (parquet OR tmp) AND ^data.
		{"'parquet | 'tmp ^data", []string{"data/logs/2026/app.tmp", "data/metrics/cpu.parquet"}},
		// Empty pattern matches all, input order.
		{"", names},
	}

	for _, c := range cases {
		got := matchNames(t, c.pattern, names)
		gotSet := map[string]bool{}
		for _, n := range got {
			gotSet[n] = true
		}
		if len(got) != len(c.want) {
			t.Errorf("pattern %q matched %v, want %v", c.pattern, got, c.want)
			continue
		}
		for _, w := range c.want {
			if !gotSet[w] {
				t.Errorf("pattern %q missing %q (got %v)", c.pattern, w, got)
			}
		}
	}
}

func TestEqualMatchTerm(t *testing.T) {
	names := []string{"app", "app.json", "the-app"}
	if got := matchNames(t, "^app$", names); !reflect.DeepEqual(got, []string{"app"}) {
		t.Fatalf("^app$ matched %v, want [app]", got)
	}
}

func TestEscapedSpace(t *testing.T) {
	names := []string{"my file.txt", "myfile.txt"}
	if got := matchNames(t, `'my\ file`, names); !reflect.DeepEqual(got, []string{"my file.txt"}) {
		t.Fatalf(`'my\ file matched %v, want [my file.txt]`, got)
	}
}

func TestMatchPositionsForHighlight(t *testing.T) {
	res := Match("'log", "data/logs")
	if !res.Matched {
		t.Fatal("'log should match data/logs")
	}
	if !reflect.DeepEqual(res.Pos, []int{5, 6, 7}) {
		t.Fatalf("Pos = %v, want [5 6 7]", res.Pos)
	}
}

func TestNegationOnlyPatternMatchesWithoutPositions(t *testing.T) {
	res := Match("!zzz", "data/logs")
	if !res.Matched {
		t.Fatal("!zzz should match a candidate without zzz")
	}
	if len(res.Pos) != 0 {
		t.Fatalf("negation-only match should carry no positions, got %v", res.Pos)
	}
	if res2 := Match("!logs", "data/logs"); res2.Matched {
		t.Fatal("!logs must not match data/logs")
	}
}

func TestCaseInsensitive(t *testing.T) {
	if res := Match("'READ", "readme.md"); !res.Matched {
		t.Fatal("matching should be case-insensitive")
	}
}

func TestFilterTiesKeepInputOrder(t *testing.T) {
	names := []string{"aaa-1", "aaa-2", "aaa-3"}
	if got := matchNames(t, "'aaa", names); !reflect.DeepEqual(got, names) {
		t.Fatalf("equal-score matches reordered: %v", got)
	}
}
