package fuzzy

import (
	"fmt"
	"testing"
)

func benchNames() []string {
	names := make([]string, 200_000)
	for i := range names {
		names[i] = fmt.Sprintf("data/logs/2026/%02d/app-%06d.json", i%12, i)
	}
	return names
}

func BenchmarkRanks200kFuzzy(b *testing.B) {
	names := benchNames()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Ranks("aplog", names)
	}
}

func BenchmarkRanks200kExact(b *testing.B) {
	names := benchNames()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Ranks("'2026/07", names)
	}
}
