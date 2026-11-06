package frame

import (
	"fmt"
	"math"
)

type aggOp int

const (
	opSum aggOp = iota
	opMean
	opMin
	opMax
	opCount
	opFirst
	opLast
	opStd
)

var opNames = [...]string{"sum", "mean", "min", "max", "count", "first", "last", "std"}

// Agg is an aggregation of one column. The output is named "<col>_<op>"
// unless renamed with As.
type Agg struct {
	col  string
	name string
	op   aggOp
}

func newAgg(col string, op aggOp) Agg { return Agg{col, col + "_" + opNames[op], op} }

// Sum totals present values. Int columns stay int64; empty groups sum to 0.
func Sum(col string) Agg { return newAgg(col, opSum) }

// Mean averages present values as float64.
func Mean(col string) Agg { return newAgg(col, opMean) }

func Min(col string) Agg { return newAgg(col, opMin) }
func Max(col string) Agg { return newAgg(col, opMax) }

// Count counts present values.
func Count(col string) Agg { return newAgg(col, opCount) }

// First takes the first present value.
func First(col string) Agg { return newAgg(col, opFirst) }

// Last takes the last present value.
func Last(col string) Agg { return newAgg(col, opLast) }

// Std is the sample standard deviation of present values.
func Std(col string) Agg { return newAgg(col, opStd) }

// As names the output column.
func (a Agg) As(name string) Agg {
	a.name = name
	return a
}

// GroupBy is a frame grouped by key columns, ready to aggregate.
type GroupBy struct {
	f    *Frame
	keys []string
}

// GroupBy groups rows by the named columns. Null keys form their own group.
func (f *Frame) GroupBy(keys ...string) *GroupBy {
	return &GroupBy{f: f, keys: keys}
}

// Agg returns one row per group, in order of first appearance, holding the
// key columns followed by each aggregation.
func (g *GroupBy) Agg(aggs ...Agg) (*Frame, error) {
	if len(g.keys) == 0 {
		return nil, fmt.Errorf("frame: GroupBy needs at least one key")
	}
	keys, err := g.f.lookup(g.keys)
	if err != nil {
		return nil, err
	}

	codes, n := groupCodes(keys)
	first := firstIndex(codes, n)

	cols := make([]Series, 0, len(keys)+len(aggs))
	for _, k := range keys {
		cols = append(cols, k.take(first))
	}
	for _, a := range aggs {
		s, err := g.f.Column(a.col)
		if err != nil {
			return nil, err
		}
		out, err := aggregate(s, a.op, codes, n)
		if err != nil {
			return nil, err
		}
		cols = append(cols, out.rename(a.name))
	}
	return newFrame(n, cols)
}

func aggregate(s Series, op aggOp, codes []int, n int) (Series, error) {
	switch op {
	case opCount:
		out := &Column[int64]{data: make([]int64, n)}
		for i, g := range codes {
			if !s.IsNull(i) {
				out.data[g]++
			}
		}
		return out, nil

	case opFirst, opLast, opMin, opMax:
		cmp := s.compare()
		pick := make([]int, n)
		for g := range pick {
			pick[g] = -1
		}
		for i, g := range codes {
			if s.IsNull(i) {
				continue
			}
			p := pick[g]
			if p < 0 || op == opLast || (op == opMin && cmp(i, p) < 0) || (op == opMax && cmp(i, p) > 0) {
				pick[g] = i
			}
		}
		return s.take(pick), nil

	case opSum:
		switch c := s.(type) {
		case *Column[int64]:
			return sum(c, codes, n), nil
		case *Column[float64]:
			return sum(c, codes, n), nil
		}

	case opMean, opStd:
		if num, ok := numeric(s); ok {
			return moments(s, num, op == opStd, codes, n), nil
		}
	}
	return nil, fmt.Errorf("%w: cannot %s %s column %q", ErrType, opNames[op], s.Type(), s.Name())
}

func sum[T int64 | float64](c *Column[T], codes []int, n int) *Column[T] {
	out := &Column[T]{data: make([]T, n)}
	for i, g := range codes {
		if !c.IsNull(i) {
			out.data[g] += c.data[i]
		}
	}
	return out
}

func numeric(s Series) (func(int) float64, bool) {
	switch c := s.(type) {
	case *Column[int64]:
		return func(i int) float64 { return float64(c.data[i]) }, true
	case *Column[float64]:
		return func(i int) float64 { return c.data[i] }, true
	}
	return nil, false
}

// moments computes per-group mean or sample standard deviation using
// Welford's algorithm.
func moments(s Series, num func(int) float64, std bool, codes []int, n int) *Column[float64] {
	count := make([]int, n)
	mean := make([]float64, n)
	m2 := make([]float64, n)
	for i, g := range codes {
		if s.IsNull(i) {
			continue
		}
		x := num(i)
		count[g]++
		d := x - mean[g]
		mean[g] += d / float64(count[g])
		m2[g] += d * (x - mean[g])
	}

	out := &Column[float64]{data: make([]float64, n)}
	for g := range n {
		switch {
		case !std && count[g] > 0:
			out.data[g] = mean[g]
		case std && count[g] > 1:
			out.data[g] = math.Sqrt(m2[g] / float64(count[g]-1))
		default:
			out.setNull(g)
		}
	}
	return out
}
