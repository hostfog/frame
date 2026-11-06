package frame

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"
)

// CSVOptions configures ReadCSV. The zero value reads comma-separated data
// and infers every column type.
type CSVOptions struct {
	// Comma is the field delimiter. Defaults to ','.
	Comma rune

	// Types sets the type of named columns instead of inferring it.
	Types map[string]Type

	// NullValues are the fields read as null. Defaults to "", "NA", "N/A",
	// "NaN", "null" and "NULL".
	NullValues []string

	// TimeLayout parses time columns. By default RFC 3339, time.DateTime and
	// time.DateOnly are tried in turn.
	TimeLayout string
}

var defaultNulls = []string{"", "NA", "N/A", "NaN", "null", "NULL"}

// ReadCSV reads a CSV with a header row. Inferred columns are the first of
// int64, float64, bool, time and string that parses every non-null field.
func ReadCSV(r io.Reader, opts CSVOptions) (*Frame, error) {
	cr := csv.NewReader(r)
	if opts.Comma != 0 {
		cr.Comma = opts.Comma
	}
	records, err := cr.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, errors.New("frame: CSV has no header")
	}
	header, records := records[0], records[1:]

	for name := range opts.Types {
		if !slices.Contains(header, name) {
			return nil, fmt.Errorf("%w: %q", ErrNoColumn, name)
		}
	}
	nulls := opts.NullValues
	if nulls == nil {
		nulls = defaultNulls
	}
	isNull := func(s string) bool { return slices.Contains(nulls, s) }
	layouts := []string{time.RFC3339Nano, time.DateTime, time.DateOnly}
	if opts.TimeLayout != "" {
		layouts = []string{opts.TimeLayout}
	}

	cols := make([]Series, len(header))
	for c, name := range header {
		raw := make([]string, len(records))
		for r, rec := range records {
			raw[r] = rec[c]
		}

		if t, ok := opts.Types[name]; ok {
			if cols[c], err = parseColumn(name, raw, t, isNull, layouts); err != nil {
				return nil, err
			}
			continue
		}
		cols[c] = inferColumn(name, raw, isNull, layouts)
	}
	return newFrame(len(records), cols)
}

func inferColumn(name string, raw []string, isNull func(string) bool, layouts []string) Series {
	if slices.ContainsFunc(raw, func(s string) bool { return !isNull(s) }) {
		for _, t := range []Type{Int64, Float64, Bool, Time} {
			if s, err := parseColumn(name, raw, t, isNull, layouts); err == nil {
				return s
			}
		}
	}
	s, _ := parseColumn(name, raw, String, isNull, layouts)
	return s
}

func parseColumn(name string, raw []string, t Type, isNull func(string) bool, layouts []string) (Series, error) {
	switch t {
	case Int64:
		return parseAs(name, raw, isNull, func(s string) (int64, error) { return strconv.ParseInt(s, 10, 64) })
	case Float64:
		return parseAs(name, raw, isNull, func(s string) (float64, error) { return strconv.ParseFloat(s, 64) })
	case String:
		return parseAs(name, raw, isNull, func(s string) (string, error) { return s, nil })
	case Bool:
		return parseAs(name, raw, isNull, parseBool)
	case Time:
		return parseAs(name, raw, isNull, func(s string) (time.Time, error) { return parseTime(s, layouts) })
	}
	return nil, fmt.Errorf("frame: unknown type %s", t)
}

func parseAs[T Elem](name string, raw []string, isNull func(string) bool, parse func(string) (T, error)) (Series, error) {
	c := &Column[T]{name: name, data: make([]T, len(raw))}
	for i, s := range raw {
		if isNull(s) {
			c.setNull(i)
			continue
		}
		v, err := parse(s)
		if err != nil {
			return nil, fmt.Errorf("frame: column %q row %d: %w", name, i+1, err)
		}
		c.data[i] = v
	}
	return c, nil
}

func parseBool(s string) (bool, error) {
	switch {
	case strings.EqualFold(s, "true"):
		return true, nil
	case strings.EqualFold(s, "false"):
		return false, nil
	}
	return false, fmt.Errorf("invalid bool %q", s)
}

func parseTime(s string, layouts []string) (time.Time, error) {
	var err error
	for _, layout := range layouts {
		var t time.Time
		if t, err = time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, err
}

// WriteCSV writes the frame with a header row. Nulls are written as empty
// fields and times as RFC 3339.
func (f *Frame) WriteCSV(w io.Writer) error {
	cw := csv.NewWriter(w)
	if err := cw.Write(f.Columns()); err != nil {
		return err
	}
	rec := make([]string, len(f.cols))
	for i := range f.n {
		for c, s := range f.cols {
			rec[c] = formatValue(s, i)
		}
		if err := cw.Write(rec); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

func formatValue(s Series, i int) string {
	switch v := s.Value(i).(type) {
	case nil:
		return ""
	case int64:
		return strconv.FormatInt(v, 10)
	case float64:
		return formatFloat(v)
	case string:
		return v
	case bool:
		return strconv.FormatBool(v)
	case time.Time:
		return v.Format(time.RFC3339Nano)
	}
	return ""
}

// formatFloat writes the shortest exact form and keeps a decimal point on
// whole numbers so the column reads back as float64.
func formatFloat(v float64) string {
	format := byte('f')
	if a := math.Abs(v); a != 0 && (a < 1e-6 || a >= 1e21) {
		format = 'g'
	}
	s := strconv.FormatFloat(v, format, -1, 64)
	if !strings.ContainsAny(s, ".eNI") {
		s += ".0"
	}
	return s
}
