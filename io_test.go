package frame

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestReadCSV(t *testing.T) {
	in := `id,price,name,ok,day,empty
1,2.5,ann,true,2024-01-02,
2,NA,bob,FALSE,2024-01-03,NA
,3,"c, d",true,,
`
	f, err := ReadCSV(strings.NewReader(in), CSVOptions{})
	if err != nil {
		t.Fatal(err)
	}
	day := func(d int) time.Time { return time.Date(2024, 1, d, 0, 0, 0, 0, time.UTC) }
	checkRows(t, f, []string{"id", "price", "name", "ok", "day", "empty"}, [][]any{
		{int64(1), 2.5, "ann", true, day(2), nil},
		{int64(2), nil, "bob", false, day(3), nil},
		{nil, 3.0, "c, d", true, nil, nil},
	})
}

func TestReadCSVOptions(t *testing.T) {
	in := "zip;when;n\n007;02/01/2024;-\n010;03/01/2024;5\n"
	f, err := ReadCSV(strings.NewReader(in), CSVOptions{
		Comma:      ';',
		Types:      map[string]Type{"zip": String, "n": Float64},
		NullValues: []string{"-"},
		TimeLayout: "02/01/2006",
	})
	if err != nil {
		t.Fatal(err)
	}
	checkRows(t, f, []string{"zip", "when", "n"}, [][]any{
		{"007", time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC), nil},
		{"010", time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC), 5.0},
	})
}

func TestReadCSVErrors(t *testing.T) {
	tests := []struct {
		name string
		in   string
		opts CSVOptions
		err  error
	}{
		{"empty", "", CSVOptions{}, nil},
		{"ragged", "a,b\n1\n", CSVOptions{}, nil},
		{"bad type", "a\nx\n", CSVOptions{Types: map[string]Type{"a": Int64}}, nil},
		{"unknown column", "a\n1\n", CSVOptions{Types: map[string]Type{"b": Int64}}, ErrNoColumn},
		{"duplicate header", "a,a\n1,2\n", CSVOptions{}, ErrDuplicate},
	}
	for _, tt := range tests {
		_, err := ReadCSV(strings.NewReader(tt.in), tt.opts)
		if err == nil || (tt.err != nil && !errors.Is(err, tt.err)) {
			t.Errorf("%s: err = %v, want %v", tt.name, err, tt.err)
		}
	}
}

func sample(t testing.TB) *Frame {
	when := time.Date(2024, 5, 6, 7, 8, 9, 10, time.UTC)
	return mustNew(t,
		nullable(t, "i", []int64{1, 0, -3}, []bool{true, false, true}),
		nullable(t, "f", []float64{2, 0.1, 0}, []bool{true, true, false}),
		nullable(t, "s", []string{"a,\"b\"", "", "x"}, []bool{true, false, true}),
		nullable(t, "b", []bool{true, false, false}, []bool{true, true, false}),
		nullable(t, "t", []time.Time{when, {}, when}, []bool{true, false, true}),
	)
}

func TestCSVRoundTrip(t *testing.T) {
	f := sample(t)
	var buf bytes.Buffer
	if err := f.WriteCSV(&buf); err != nil {
		t.Fatal(err)
	}
	got, err := ReadCSV(&buf, CSVOptions{})
	if err != nil {
		t.Fatal(err)
	}
	checkRows(t, got, f.Columns(), rowsOf(f))
}

func TestJSONRoundTrip(t *testing.T) {
	f := sample(t)
	var buf bytes.Buffer
	if err := f.WriteJSON(&buf); err != nil {
		t.Fatal(err)
	}
	want := `[{"i":1,"f":2.0,"s":"a,\"b\"","b":true,"t":"2024-05-06T07:08:09.00000001Z"},` +
		`{"i":null,"f":0.1,"s":null,"b":false,"t":null},` +
		`{"i":-3,"f":null,"s":"x","b":null,"t":"2024-05-06T07:08:09.00000001Z"}]` + "\n"
	if buf.String() != want {
		t.Fatalf("WriteJSON =\n%s\nwant\n%s", buf.String(), want)
	}
	got, err := ReadJSON(&buf)
	if err != nil {
		t.Fatal(err)
	}
	checkRows(t, got, f.Columns(), rowsOf(f))
}

func TestReadJSON(t *testing.T) {
	in := `[{"a": 1, "b": "x"}, {"c": true, "a": 2.5}, {}]`
	f, err := ReadJSON(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	checkRows(t, f, []string{"a", "b", "c"}, [][]any{
		{1.0, "x", nil},
		{2.5, nil, true},
		{nil, nil, nil},
	})

	bad := []string{
		`{"a": 1}`,
		`[{"a": {"b": 1}}]`,
		`[{"a": [1]}]`,
		`[{"a": 1, "a": 2}]`,
		`[{"a": 1}, {"a": "x"}]`,
		`[{"a": 1}`,
		`[1]`,
	}
	for _, in := range bad {
		if _, err := ReadJSON(strings.NewReader(in)); err == nil {
			t.Errorf("ReadJSON(%s) expected error", in)
		}
	}
}
