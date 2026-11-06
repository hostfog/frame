// Package frame provides an immutable, column-oriented DataFrame with typed,
// nullable columns: int64, float64, string, bool and time.Time.
//
// Every operation returns a new Frame and never modifies its receiver.
package frame

import (
	"errors"
	"fmt"
	"slices"
	"time"
)

var (
	ErrNoColumn  = errors.New("frame: no such column")
	ErrDuplicate = errors.New("frame: duplicate column")
	ErrType      = errors.New("frame: type mismatch")
	ErrLength    = errors.New("frame: length mismatch")
)

// Frame is an ordered set of equal-length columns with unique names.
type Frame struct {
	cols  []Series
	index map[string]int
	n     int
}

// New returns a frame of the given columns, which must have equal lengths and
// unique names.
func New(cols ...Series) (*Frame, error) {
	n := 0
	if len(cols) > 0 && cols[0] != nil {
		n = cols[0].Len()
	}
	return newFrame(n, cols)
}

func newFrame(n int, cols []Series) (*Frame, error) {
	f := &Frame{cols: cols, index: make(map[string]int, len(cols)), n: n}
	for i, c := range cols {
		if c == nil {
			return nil, fmt.Errorf("frame: column %d is nil", i)
		}
		if c.Len() != n {
			return nil, fmt.Errorf("%w: column %q has %d rows, want %d", ErrLength, c.Name(), c.Len(), n)
		}
		if _, ok := f.index[c.Name()]; ok {
			return nil, fmt.Errorf("%w: %q", ErrDuplicate, c.Name())
		}
		f.index[c.Name()] = i
	}
	return f, nil
}

// Len returns the number of rows.
func (f *Frame) Len() int { return f.n }

// Columns returns the column names in order.
func (f *Frame) Columns() []string {
	names := make([]string, len(f.cols))
	for i, c := range f.cols {
		names[i] = c.Name()
	}
	return names
}

// Column returns the named column.
func (f *Frame) Column(name string) (Series, error) {
	i, ok := f.index[name]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrNoColumn, name)
	}
	return f.cols[i], nil
}

func (f *Frame) lookup(names []string) ([]Series, error) {
	cols := make([]Series, len(names))
	for i, name := range names {
		c, err := f.Column(name)
		if err != nil {
			return nil, err
		}
		cols[i] = c
	}
	return cols, nil
}

// Get returns the named column as a *Column[T].
func Get[T Elem](f *Frame, name string) (*Column[T], error) {
	s, err := f.Column(name)
	if err != nil {
		return nil, err
	}
	c, ok := s.(*Column[T])
	if !ok {
		return nil, fmt.Errorf("%w: column %q is %s, not %s", ErrType, name, s.Type(), typeOf[T]())
	}
	return c, nil
}

func (f *Frame) take(idx []int) *Frame {
	cols := make([]Series, len(f.cols))
	for i, c := range f.cols {
		cols[i] = c.take(idx)
	}
	return &Frame{cols: cols, index: f.index, n: len(idx)}
}

func (f *Frame) rows(lo, hi int) *Frame {
	idx := make([]int, hi-lo)
	for i := range idx {
		idx[i] = lo + i
	}
	return f.take(idx)
}

// Head returns the first n rows.
func (f *Frame) Head(n int) *Frame {
	return f.rows(0, min(max(n, 0), f.n))
}

// Tail returns the last n rows.
func (f *Frame) Tail(n int) *Frame {
	return f.rows(f.n-min(max(n, 0), f.n), f.n)
}

// Select returns the named columns in the given order.
func (f *Frame) Select(names ...string) (*Frame, error) {
	cols, err := f.lookup(names)
	if err != nil {
		return nil, err
	}
	return newFrame(f.n, cols)
}

// Drop returns the frame without the named columns.
func (f *Frame) Drop(names ...string) (*Frame, error) {
	if _, err := f.lookup(names); err != nil {
		return nil, err
	}
	var cols []Series
	for _, c := range f.cols {
		if !slices.Contains(names, c.Name()) {
			cols = append(cols, c)
		}
	}
	return newFrame(f.n, cols)
}

// Rename returns the frame with column from renamed to to.
func (f *Frame) Rename(from, to string) (*Frame, error) {
	i, ok := f.index[from]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrNoColumn, from)
	}
	cols := slices.Clone(f.cols)
	cols[i] = cols[i].rename(to)
	return newFrame(f.n, cols)
}

// WithColumn returns the frame with s added, or replacing the column of the
// same name.
func (f *Frame) WithColumn(s Series) (*Frame, error) {
	n := f.n
	if len(f.cols) == 0 {
		n = s.Len()
	}
	cols := slices.Clone(f.cols)
	if i, ok := f.index[s.Name()]; ok {
		cols[i] = s
	} else {
		cols = append(cols, s)
	}
	return newFrame(n, cols)
}

// Row is a view of one row, passed to Filter and Compute. The typed getters
// return false when the value is null or the column is missing or of another
// type.
type Row struct {
	f *Frame
	i int
}

// Index returns the row's position in its frame.
func (r Row) Index() int { return r.i }

func (r Row) col(name string) Series {
	if i, ok := r.f.index[name]; ok {
		return r.f.cols[i]
	}
	return nil
}

// Value returns the named value, or nil if it is null or missing.
func (r Row) Value(name string) any {
	if c := r.col(name); c != nil {
		return c.Value(r.i)
	}
	return nil
}

func (r Row) IsNull(name string) bool { return r.Value(name) == nil }

func (r Row) Int(name string) (int64, bool)      { return rowGet[int64](r, name) }
func (r Row) Float(name string) (float64, bool)  { return rowGet[float64](r, name) }
func (r Row) Str(name string) (string, bool)     { return rowGet[string](r, name) }
func (r Row) Bool(name string) (bool, bool)      { return rowGet[bool](r, name) }
func (r Row) Time(name string) (time.Time, bool) { return rowGet[time.Time](r, name) }

func rowGet[T Elem](r Row, name string) (T, bool) {
	if c, ok := r.col(name).(*Column[T]); ok {
		return c.At(r.i)
	}
	var zero T
	return zero, false
}
