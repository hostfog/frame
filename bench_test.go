package frame

import (
	"math/rand"
	"testing"
)

func benchFrame(b *testing.B, rows int) *Frame {
	rng := rand.New(rand.NewSource(1))
	keys := make([]int64, rows)
	names := make([]string, rows)
	vals := make([]float64, rows)
	labels := []string{"a", "b", "c", "d", "e", "f", "g", "h"}
	for i := range rows {
		keys[i] = rng.Int63n(1000)
		names[i] = labels[rng.Intn(len(labels))]
		vals[i] = rng.Float64()
	}
	return mustNew(b, NewColumn("k", keys), NewColumn("s", names), NewColumn("v", vals))
}

func BenchmarkGroupBy(b *testing.B) {
	f := benchFrame(b, 1_000_000)
	b.ResetTimer()
	for range b.N {
		if _, err := f.GroupBy("k", "s").Agg(Sum("v"), Mean("v"), Count("v")); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSort(b *testing.B) {
	f := benchFrame(b, 1_000_000)
	b.ResetTimer()
	for range b.N {
		if _, err := f.Sort(Asc("s"), Desc("v")); err != nil {
			b.Fatal(err)
		}
	}
}
