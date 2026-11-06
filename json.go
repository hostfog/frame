package frame

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"time"
)

// ReadJSON reads an array of flat objects, one per row. Columns appear in
// order of first appearance and missing keys are null. Numbers become int64
// when every value is whole, and strings become times when every value is
// RFC 3339.
func ReadJSON(r io.Reader) (*Frame, error) {
	dec := json.NewDecoder(r)
	dec.UseNumber()
	if err := expectDelim(dec, '['); err != nil {
		return nil, err
	}

	var names []string
	var vals [][]any
	index := map[string]int{}
	rows := 0
	for dec.More() {
		if err := expectDelim(dec, '{'); err != nil {
			return nil, err
		}
		for dec.More() {
			tok, err := dec.Token()
			if err != nil {
				return nil, err
			}
			key := tok.(string)
			var v any
			if err := dec.Decode(&v); err != nil {
				return nil, err
			}
			switch v.(type) {
			case map[string]any, []any:
				return nil, fmt.Errorf("frame: row %d key %q: nested values are not supported", rows, key)
			}
			c, ok := index[key]
			if !ok {
				c = len(names)
				index[key] = c
				names = append(names, key)
				vals = append(vals, make([]any, rows))
			}
			if len(vals[c]) > rows {
				return nil, fmt.Errorf("frame: row %d repeats key %q", rows, key)
			}
			vals[c] = append(vals[c], v)
		}
		if err := expectDelim(dec, '}'); err != nil {
			return nil, err
		}
		rows++
		for c := range vals {
			if len(vals[c]) < rows {
				vals[c] = append(vals[c], nil)
			}
		}
	}
	if err := expectDelim(dec, ']'); err != nil {
		return nil, err
	}

	cols := make([]Series, len(names))
	for c, name := range names {
		col, err := jsonColumn(name, vals[c])
		if err != nil {
			return nil, err
		}
		cols[c] = col
	}
	return newFrame(rows, cols)
}

func expectDelim(dec *json.Decoder, want json.Delim) error {
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	if tok != want {
		return fmt.Errorf("frame: expected %q in JSON, got %v", want, tok)
	}
	return nil
}

func jsonColumn(name string, vals []any) (Series, error) {
	for i, v := range vals {
		x, ok := v.(json.Number)
		if !ok {
			continue
		}
		if n, err := x.Int64(); err == nil {
			vals[i] = n
		} else if vals[i], err = x.Float64(); err != nil {
			return nil, fmt.Errorf("frame: column %q row %d: %w", name, i, err)
		}
	}

	t, err := inferType(vals)
	if err != nil {
		return nil, fmt.Errorf("column %q: %w", name, err)
	}
	if t == String {
		if times, ok := parseTimes(vals); ok {
			vals, t = times, Time
		}
	}
	return makeColumn(name, t, vals)
}

func parseTimes(vals []any) ([]any, bool) {
	out := make([]any, len(vals))
	present := false
	for i, v := range vals {
		if v == nil {
			continue
		}
		t, err := time.Parse(time.RFC3339Nano, v.(string))
		if err != nil {
			return nil, false
		}
		out[i], present = t, true
	}
	return out, present
}

// WriteJSON writes the frame as an array of objects, one per row. Nulls are
// written as null and times as RFC 3339 strings.
func (f *Frame) WriteJSON(w io.Writer) error {
	bw := bufio.NewWriter(w)
	keys := make([][]byte, len(f.cols))
	for c, s := range f.cols {
		k, err := json.Marshal(s.Name())
		if err != nil {
			return err
		}
		keys[c] = append(k, ':')
	}

	bw.WriteByte('[')
	for i := range f.n {
		if i > 0 {
			bw.WriteByte(',')
		}
		bw.WriteByte('{')
		for c, s := range f.cols {
			if c > 0 {
				bw.WriteByte(',')
			}
			bw.Write(keys[c])
			v, err := jsonValue(s, i)
			if err != nil {
				return err
			}
			bw.Write(v)
		}
		bw.WriteByte('}')
	}
	bw.WriteString("]\n")
	return bw.Flush()
}

func jsonValue(s Series, i int) ([]byte, error) {
	switch v := s.Value(i).(type) {
	case nil:
		return []byte("null"), nil
	case int64:
		return strconv.AppendInt(nil, v, 10), nil
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return nil, errors.New("frame: JSON cannot encode NaN or Inf")
		}
		return []byte(formatFloat(v)), nil
	default:
		return json.Marshal(v)
	}
}
