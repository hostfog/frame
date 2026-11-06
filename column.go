package frame

import (
	"cmp"
	"fmt"
	"math/bits"
	"slices"
	"time"
)

// Elem is the set of Go types a column can hold.
type Elem interface {
	int64 | float64 | string | bool | time.Time
}

// Type identifies the element type of a column.
type Type int

const (
	Int64 Type = iota
	Float64
	String
	Bool
	Time
)

func (t Type) String() string {
	switch t {
	case Int64:
		return "int64"
	case Float64:
		return "float64"
	case String:
		return "string"
	case Bool:
		return "bool"
	case Time:
		return "time"
	}
	return fmt.Sprintf("Type(%d)", int(t))
}

func typeOf[T Elem]() Type {
	var z T
	switch any(z).(type) {
	case int64:
		return Int64
	case float64:
		return Float64
	case string:
		return String
	case bool:
		return Bool
	}
	return Time
}

// Series is a named, immutable column of values. Every Series is a *Column[T];
// use Get or a type assertion to reach the typed values.
type Series interface {
	Name() string
	Len() int
	Type() Type
	IsNull(i int) bool
	NullCount() int
	// Value returns the i-th value, or nil if it is null.
	Value(i int) any

	take(idx []int) Series
	rename(name string) Series
	concat(other Series) (Series, error)
	fill(v any) (Series, error)
	compare() func(i, j int) int
	codes() ([]int, int)
}

// Column is a typed column with a null bitmap. Columns are never modified
// after construction, so frames can share them freely.
type Column[T Elem] struct {
	name  string
	data  []T
	nulls []uint64 // bit set means null; nil when there are no nulls
}

// NewColumn returns a column holding a copy of values, none of them null.
func NewColumn[T Elem](name string, values []T) *Column[T] {
	return &Column[T]{name: name, data: slices.Clone(values)}
}

// NewNullColumn returns a column holding a copy of values where valid[i]
// reports whether values[i] is present.
func NewNullColumn[T Elem](name string, values []T, valid []bool) (*Column[T], error) {
	if len(valid) != len(values) {
		return nil, fmt.Errorf("%w: %d values, %d validity flags", ErrLength, len(values), len(valid))
	}
	c := NewColumn(name, values)
	for i, ok := range valid {
		if !ok {
			c.setNull(i)
		}
	}
	return c, nil
}

func (c *Column[T]) Name() string { return c.name }
func (c *Column[T]) Len() int     { return len(c.data) }
func (c *Column[T]) Type() Type   { return typeOf[T]() }

func (c *Column[T]) IsNull(i int) bool {
	return c.nulls != nil && c.nulls[i/64]&(1<<(uint(i)%64)) != 0
}

func (c *Column[T]) NullCount() int {
	n := 0
	for _, w := range c.nulls {
		n += bits.OnesCount64(w)
	}
	return n
}

// At returns the i-th value and whether it is present.
func (c *Column[T]) At(i int) (T, bool) {
	return c.data[i], !c.IsNull(i)
}

func (c *Column[T]) Value(i int) any {
	if c.IsNull(i) {
		return nil
	}
	return c.data[i]
}

// Values returns a copy of the data. Null slots hold the zero value.
func (c *Column[T]) Values() []T {
	return slices.Clone(c.data)
}

func (c *Column[T]) setNull(i int) {
	if c.nulls == nil {
		c.nulls = make([]uint64, (len(c.data)+63)/64)
	}
	c.nulls[i/64] |= 1 << (uint(i) % 64)
	var zero T
	c.data[i] = zero
}

// take gathers the rows at idx. An index of -1 produces a null.
func (c *Column[T]) take(idx []int) Series {
	out := &Column[T]{name: c.name, data: make([]T, len(idx))}
	for j, i := range idx {
		if i < 0 || c.IsNull(i) {
			out.setNull(j)
			continue
		}
		out.data[j] = c.data[i]
	}
	return out
}

func (c *Column[T]) rename(name string) Series {
	out := *c
	out.name = name
	return &out
}

func (c *Column[T]) concat(other Series) (Series, error) {
	o, ok := other.(*Column[T])
	if !ok {
		return nil, fmt.Errorf("%w: %q is %s, %q is %s", ErrType, c.name, c.Type(), other.Name(), other.Type())
	}
	out := &Column[T]{name: c.name, data: make([]T, 0, len(c.data)+len(o.data))}
	out.data = append(append(out.data, c.data...), o.data...)
	for i := range c.data {
		if c.IsNull(i) {
			out.setNull(i)
		}
	}
	for i := range o.data {
		if o.IsNull(i) {
			out.setNull(len(c.data) + i)
		}
	}
	return out, nil
}

func (c *Column[T]) fill(v any) (Series, error) {
	x, err := convert(v, c.Type())
	if err != nil {
		return nil, fmt.Errorf("column %q: %w", c.name, err)
	}
	out := &Column[T]{name: c.name, data: slices.Clone(c.data)}
	for i := range out.data {
		if c.IsNull(i) {
			out.data[i] = x.(T)
		}
	}
	return out, nil
}

// compare returns a comparator over row indices. It ignores nulls.
func (c *Column[T]) compare() func(i, j int) int {
	switch d := any(c.data).(type) {
	case []int64:
		return func(i, j int) int { return cmp.Compare(d[i], d[j]) }
	case []float64:
		return func(i, j int) int { return cmp.Compare(d[i], d[j]) }
	case []string:
		return func(i, j int) int { return cmp.Compare(d[i], d[j]) }
	case []bool:
		return func(i, j int) int {
			switch {
			case d[i] == d[j]:
				return 0
			case d[j]:
				return -1
			}
			return 1
		}
	}
	d := any(c.data).([]time.Time)
	return func(i, j int) int { return d[i].Compare(d[j]) }
}

// codes assigns each row a dense integer by value, in order of first
// appearance. Nulls share one code.
func (c *Column[T]) codes() ([]int, int) {
	if d, ok := any(c.data).([]time.Time); ok {
		return factorize(len(d), c.IsNull, func(i int) [2]int64 {
			return [2]int64{d[i].Unix(), int64(d[i].Nanosecond())}
		})
	}
	return factorize(len(c.data), c.IsNull, func(i int) T { return c.data[i] })
}

func factorize[K comparable](n int, isNull func(int) bool, key func(int) K) ([]int, int) {
	codes := make([]int, n)
	seen := make(map[K]int)
	next, nullCode := 0, -1
	for i := range n {
		if isNull(i) {
			if nullCode < 0 {
				nullCode = next
				next++
			}
			codes[i] = nullCode
			continue
		}
		k := key(i)
		code, ok := seen[k]
		if !ok {
			code = next
			seen[k] = code
			next++
		}
		codes[i] = code
	}
	return codes, next
}

// groupCodes combines the codes of several columns into one dense code per
// distinct row, in order of first appearance.
func groupCodes(cols []Series) ([]int, int) {
	codes, n := cols[0].codes()
	for _, c := range cols[1:] {
		next, m := c.codes()
		seen := make(map[int]int, n)
		for i, code := range codes {
			key := code*m + next[i]
			g, ok := seen[key]
			if !ok {
				g = len(seen)
				seen[key] = g
			}
			codes[i] = g
		}
		n = len(seen)
	}
	return codes, n
}

// firstIndex returns the first row of each group.
func firstIndex(codes []int, n int) []int {
	first := make([]int, 0, n)
	for i, g := range codes {
		if g == len(first) {
			first = append(first, i)
		}
	}
	return first
}
