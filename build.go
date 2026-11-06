package frame

import (
	"fmt"
	"math"
	"reflect"
	"time"
)

var timeType = reflect.TypeOf(time.Time{})

// FromRecords builds a frame from rows of values in column order. Column
// types are inferred from the Go values: any integer kind becomes int64, a
// mix of integers and floats becomes float64, and nil is null. A column of
// only nils is a string column.
func FromRecords(names []string, rows [][]any) (*Frame, error) {
	vals := make([][]any, len(names))
	for i := range vals {
		vals[i] = make([]any, len(rows))
	}
	for r, row := range rows {
		if len(row) != len(names) {
			return nil, fmt.Errorf("%w: row %d has %d values, want %d", ErrLength, r, len(row), len(names))
		}
		for c, v := range row {
			vals[c][r] = v
		}
	}

	cols := make([]Series, len(names))
	for i, name := range names {
		t, err := inferType(vals[i])
		if err != nil {
			return nil, fmt.Errorf("column %q: %w", name, err)
		}
		if cols[i], err = makeColumn(name, t, vals[i]); err != nil {
			return nil, err
		}
	}
	return newFrame(len(rows), cols)
}

// FromStructs builds a frame from a slice of structs or struct pointers, one
// column per exported field. The `frame:"name"` tag renames a field and
// `frame:"-"` skips it. Pointer fields are nullable.
func FromStructs[T any](rows []T) (*Frame, error) {
	rt := reflect.TypeOf((*T)(nil)).Elem()
	ptr := rt.Kind() == reflect.Pointer
	if ptr {
		rt = rt.Elem()
	}
	if rt.Kind() != reflect.Struct {
		return nil, fmt.Errorf("frame: FromStructs needs structs, got %s", rt)
	}

	type field struct {
		index    int
		name     string
		typ      Type
		nullable bool
	}
	var fields []field
	for i := range rt.NumField() {
		sf := rt.Field(i)
		tag := sf.Tag.Get("frame")
		if !sf.IsExported() || tag == "-" {
			continue
		}
		name := tag
		if name == "" {
			name = sf.Name
		}
		ft := sf.Type
		nullable := ft.Kind() == reflect.Pointer
		if nullable {
			ft = ft.Elem()
		}
		typ, ok := typeOfKind(ft)
		if !ok {
			return nil, fmt.Errorf("frame: field %s has unsupported type %s", sf.Name, sf.Type)
		}
		fields = append(fields, field{i, name, typ, nullable})
	}

	vals := make([][]any, len(fields))
	for i := range vals {
		vals[i] = make([]any, len(rows))
	}
	for r, row := range rows {
		rv := reflect.ValueOf(row)
		if ptr {
			if rv.IsNil() {
				return nil, fmt.Errorf("frame: row %d is nil", r)
			}
			rv = rv.Elem()
		}
		for i, f := range fields {
			fv := rv.Field(f.index)
			if f.nullable {
				if fv.IsNil() {
					continue
				}
				fv = fv.Elem()
			}
			vals[i][r] = fv.Interface()
		}
	}

	cols := make([]Series, len(fields))
	for i, f := range fields {
		var err error
		if cols[i], err = makeColumn(f.name, f.typ, vals[i]); err != nil {
			return nil, err
		}
	}
	return newFrame(len(rows), cols)
}

func typeOfKind(t reflect.Type) (Type, bool) {
	if t == timeType {
		return Time, true
	}
	switch t.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return Int64, true
	case reflect.Float32, reflect.Float64:
		return Float64, true
	case reflect.String:
		return String, true
	case reflect.Bool:
		return Bool, true
	}
	return 0, false
}

func inferType(vals []any) (Type, error) {
	t, seen := String, false
	for _, v := range vals {
		if v == nil {
			continue
		}
		vt, ok := typeOfKind(reflect.TypeOf(v))
		if !ok {
			return 0, fmt.Errorf("%w: unsupported value type %T", ErrType, v)
		}
		switch {
		case !seen:
			t, seen = vt, true
		case vt == t:
		case vt == Float64 && t == Int64, vt == Int64 && t == Float64:
			t = Float64
		default:
			return 0, fmt.Errorf("%w: mixed %s and %s values", ErrType, t, vt)
		}
	}
	return t, nil
}

func makeColumn(name string, t Type, vals []any) (Series, error) {
	switch t {
	case Int64:
		return fromAny[int64](name, vals)
	case Float64:
		return fromAny[float64](name, vals)
	case String:
		return fromAny[string](name, vals)
	case Bool:
		return fromAny[bool](name, vals)
	}
	return fromAny[time.Time](name, vals)
}

func fromAny[T Elem](name string, vals []any) (Series, error) {
	c := &Column[T]{name: name, data: make([]T, len(vals))}
	for i, v := range vals {
		if v == nil {
			c.setNull(i)
			continue
		}
		x, err := convert(v, c.Type())
		if err != nil {
			return nil, fmt.Errorf("column %q row %d: %w", name, i, err)
		}
		c.data[i] = x.(T)
	}
	return c, nil
}

// convert returns v as the Go type of t, accepting any integer or float kind
// for numeric types and named types with a matching underlying kind.
func convert(v any, t Type) (any, error) {
	if tv, ok := v.(time.Time); ok && t == Time {
		return tv, nil
	}
	rv := reflect.ValueOf(v)
	switch k := rv.Kind(); {
	case t == Int64 && k >= reflect.Int && k <= reflect.Int64:
		return rv.Int(), nil
	case t == Int64 && k >= reflect.Uint && k <= reflect.Uint64:
		if rv.Uint() > math.MaxInt64 {
			return nil, fmt.Errorf("%w: %d overflows int64", ErrType, rv.Uint())
		}
		return int64(rv.Uint()), nil
	case t == Float64 && k >= reflect.Int && k <= reflect.Int64:
		return float64(rv.Int()), nil
	case t == Float64 && k >= reflect.Uint && k <= reflect.Uint64:
		return float64(rv.Uint()), nil
	case t == Float64 && (k == reflect.Float32 || k == reflect.Float64):
		return rv.Float(), nil
	case t == String && k == reflect.String:
		return rv.String(), nil
	case t == Bool && k == reflect.Bool:
		return rv.Bool(), nil
	}
	return nil, fmt.Errorf("%w: cannot use %T as %s", ErrType, v, t)
}
