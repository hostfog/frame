package frame

import (
	"slices"
)

// Filter returns the rows for which keep returns true.
func (f *Frame) Filter(keep func(Row) bool) *Frame {
	var idx []int
	for i := range f.n {
		if keep(Row{f, i}) {
			idx = append(idx, i)
		}
	}
	return f.take(idx)
}

// Where returns the rows whose value in the named column is present and
// satisfies keep.
func Where[T Elem](f *Frame, name string, keep func(T) bool) (*Frame, error) {
	c, err := Get[T](f, name)
	if err != nil {
		return nil, err
	}
	var idx []int
	for i, v := range c.data {
		if !c.IsNull(i) && keep(v) {
			idx = append(idx, i)
		}
	}
	return f.take(idx), nil
}

// Compute builds a column by calling fn for every row. A false second result
// makes the value null.
func Compute[T Elem](f *Frame, name string, fn func(Row) (T, bool)) *Column[T] {
	c := &Column[T]{name: name, data: make([]T, f.n)}
	for i := range f.n {
		if v, ok := fn(Row{f, i}); ok {
			c.data[i] = v
		} else {
			c.setNull(i)
		}
	}
	return c
}

// SortKey orders rows by one column.
type SortKey struct {
	Column string
	Desc   bool
}

func Asc(col string) SortKey  { return SortKey{Column: col} }
func Desc(col string) SortKey { return SortKey{Column: col, Desc: true} }

// Sort returns the rows ordered by keys. The sort is stable and nulls come
// last in either direction.
func (f *Frame) Sort(keys ...SortKey) (*Frame, error) {
	type key struct {
		null []bool // nil when the column has no nulls
		cmp  func(i, j int) int
		desc bool
	}
	ks := make([]key, len(keys))
	for i, k := range keys {
		s, err := f.Column(k.Column)
		if err != nil {
			return nil, err
		}
		ks[i] = key{cmp: s.compare(), desc: k.Desc}
		if s.NullCount() > 0 {
			ks[i].null = make([]bool, f.n)
			for j := range f.n {
				ks[i].null[j] = s.IsNull(j)
			}
		}
	}

	idx := make([]int, f.n)
	for i := range idx {
		idx[i] = i
	}
	// Ties fall back to row order, which keeps the unstable sort stable.
	slices.SortFunc(idx, func(a, b int) int {
		for _, k := range ks {
			if k.null != nil {
				an, bn := k.null[a], k.null[b]
				switch {
				case an && bn:
					continue
				case an:
					return 1
				case bn:
					return -1
				}
			}
			c := k.cmp(a, b)
			if k.desc {
				c = -c
			}
			if c != 0 {
				return c
			}
		}
		return a - b
	})
	return f.take(idx), nil
}

// FillNull returns the frame with nulls in the named column replaced by v.
// Numeric v is converted to the column type when it fits.
func (f *Frame) FillNull(name string, v any) (*Frame, error) {
	c, err := f.Column(name)
	if err != nil {
		return nil, err
	}
	filled, err := c.fill(v)
	if err != nil {
		return nil, err
	}
	return f.WithColumn(filled)
}

// DropNull returns the rows with no nulls in the named columns, or in any
// column if none are named.
func (f *Frame) DropNull(names ...string) (*Frame, error) {
	cols := f.cols
	if len(names) > 0 {
		var err error
		if cols, err = f.lookup(names); err != nil {
			return nil, err
		}
	}
	var idx []int
rows:
	for i := range f.n {
		for _, c := range cols {
			if c.IsNull(i) {
				continue rows
			}
		}
		idx = append(idx, i)
	}
	return f.take(idx), nil
}

// Unique returns the first row for each distinct combination of the named
// columns, or of all columns if none are named. Nulls compare equal.
func (f *Frame) Unique(names ...string) (*Frame, error) {
	cols := f.cols
	if len(names) > 0 {
		var err error
		if cols, err = f.lookup(names); err != nil {
			return nil, err
		}
	}
	if len(cols) == 0 {
		return f, nil
	}
	return f.take(firstIndex(groupCodes(cols))), nil
}
