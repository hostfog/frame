package frame

import (
	"math"
	"slices"
)

// Describe returns count, mean, std, min, quartiles and max for every numeric
// column, ignoring nulls.
func (f *Frame) Describe() *Frame {
	cols := []Series{NewColumn("stat", []string{"count", "mean", "std", "min", "25%", "50%", "75%", "max"})}
	for _, s := range f.cols {
		num, ok := numeric(s)
		if !ok {
			continue
		}
		var vals []float64
		for i := range f.n {
			if !s.IsNull(i) {
				vals = append(vals, num(i))
			}
		}
		slices.Sort(vals)

		out := &Column[float64]{name: s.Name(), data: make([]float64, 8)}
		out.data[0] = float64(len(vals))
		if len(vals) == 0 {
			for i := 1; i < 8; i++ {
				out.setNull(i)
			}
			cols = append(cols, out)
			continue
		}

		var mean, m2 float64
		for i, x := range vals {
			d := x - mean
			mean += d / float64(i+1)
			m2 += d * (x - mean)
		}
		out.data[1] = mean
		if len(vals) > 1 {
			out.data[2] = math.Sqrt(m2 / float64(len(vals)-1))
		} else {
			out.setNull(2)
		}
		out.data[3] = vals[0]
		out.data[4] = quantile(vals, 0.25)
		out.data[5] = quantile(vals, 0.5)
		out.data[6] = quantile(vals, 0.75)
		out.data[7] = vals[len(vals)-1]
		cols = append(cols, out)
	}
	return &Frame{cols: cols, index: indexOf(cols), n: 8}
}

func indexOf(cols []Series) map[string]int {
	m := make(map[string]int, len(cols))
	for i, c := range cols {
		m[c.Name()] = i
	}
	return m
}

// quantile interpolates linearly between the closest ranks of sorted.
func quantile(sorted []float64, q float64) float64 {
	pos := q * float64(len(sorted)-1)
	lo := int(pos)
	if lo+1 >= len(sorted) {
		return sorted[lo]
	}
	return sorted[lo] + (sorted[lo+1]-sorted[lo])*(pos-float64(lo))
}
