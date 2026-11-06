package frame

import (
	"fmt"
	"slices"
)

// JoinType selects which unmatched rows a join keeps.
type JoinType int

const (
	Inner JoinType = iota
	Left
	Right
	Outer
)

// Join hash-joins f with right on the named key columns, which must have the
// same types on both sides. Null keys never match.
//
// The result holds the keys, then f's other columns, then right's other
// columns; right-side names that clash get a "_right" suffix. Rows follow f's
// order, except for a Right join, which follows right's order.
func (f *Frame) Join(right *Frame, how JoinType, on ...string) (*Frame, error) {
	if len(on) == 0 {
		return nil, fmt.Errorf("frame: Join needs at least one key")
	}
	lkeys, err := f.lookup(on)
	if err != nil {
		return nil, err
	}
	rkeys, err := right.lookup(on)
	if err != nil {
		return nil, err
	}

	keys := make([]Series, len(on))
	for i := range on {
		if keys[i], err = lkeys[i].concat(rkeys[i]); err != nil {
			return nil, err
		}
	}
	codes, n := groupCodes(keys)
	for i := range codes {
		for _, k := range keys {
			if k.IsNull(i) {
				codes[i] = -1
				break
			}
		}
	}
	lc, rc := codes[:f.n], codes[f.n:]

	var li, ri []int
	switch how {
	case Inner:
		li, ri, _ = match(lc, rc, n, false)
	case Left:
		li, ri, _ = match(lc, rc, n, true)
	case Right:
		ri, li, _ = match(rc, lc, n, true)
	case Outer:
		var matched []bool
		li, ri, matched = match(lc, rc, n, true)
		for j, ok := range matched {
			if !ok {
				li = append(li, -1)
				ri = append(ri, j)
			}
		}
	default:
		return nil, fmt.Errorf("frame: unknown join type %d", how)
	}

	cols := make([]Series, 0, len(f.cols)+len(right.cols))
	kidx := make([]int, len(li))
	for i := range li {
		kidx[i] = li[i]
		if kidx[i] < 0 {
			kidx[i] = f.n + ri[i]
		}
	}
	for _, k := range keys {
		cols = append(cols, k.take(kidx))
	}
	for _, c := range f.cols {
		if !slices.Contains(on, c.Name()) {
			cols = append(cols, c.take(li))
		}
	}
	for _, c := range right.cols {
		if slices.Contains(on, c.Name()) {
			continue
		}
		out := c.take(ri)
		if _, clash := f.index[c.Name()]; clash {
			out = out.rename(c.Name() + "_right")
		}
		cols = append(cols, out)
	}
	return newFrame(len(li), cols)
}

// match pairs each probe row, in order, with every build row of the same
// code. Codes below zero never match. With keep, unmatched probe rows pair
// with -1. matched reports which build rows found a partner.
func match(probe, build []int, ncodes int, keep bool) (pi, bi []int, matched []bool) {
	start := make([]int, ncodes+1)
	for _, c := range build {
		if c >= 0 {
			start[c+1]++
		}
	}
	for c := range ncodes {
		start[c+1] += start[c]
	}
	rows := make([]int, start[ncodes])
	pos := slices.Clone(start[:ncodes])
	for j, c := range build {
		if c >= 0 {
			rows[pos[c]] = j
			pos[c]++
		}
	}

	matched = make([]bool, len(build))
	for i, c := range probe {
		if c >= 0 && start[c] < start[c+1] {
			for _, j := range rows[start[c]:start[c+1]] {
				pi = append(pi, i)
				bi = append(bi, j)
				matched[j] = true
			}
		} else if keep {
			pi = append(pi, i)
			bi = append(bi, -1)
		}
	}
	return pi, bi, matched
}
