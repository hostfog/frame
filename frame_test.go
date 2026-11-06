package frame

import (
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"
)

func mustNew(t testing.TB, cols ...Series) *Frame {
	t.Helper()
	f, err := New(cols...)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func nullable[T Elem](t testing.TB, name string, values []T, valid []bool) *Column[T] {
	t.Helper()
	c, err := NewNullColumn(name, values, valid)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func rowsOf(f *Frame) [][]any {
	out := make([][]any, f.Len())
	for i := range out {
		for _, c := range f.cols {
			out[i] = append(out[i], c.Value(i))
		}
	}
	return out
}

func checkRows(t *testing.T, f *Frame, cols []string, want [][]any) {
	t.Helper()
	if got := f.Columns(); !reflect.DeepEqual(got, cols) {
		t.Fatalf("columns = %v, want %v", got, cols)
	}
	if got := rowsOf(f); !reflect.DeepEqual(got, want) {
		t.Fatalf("rows =\n%v\nwant\n%v", got, want)
	}
}

func people(t testing.TB) *Frame {
	return mustNew(t,
		NewColumn("name", []string{"ann", "bob", "cat", "dan"}),
		nullable(t, "age", []int64{31, 0, 25, 40}, []bool{true, false, true, true}),
		NewColumn("score", []float64{1.5, 2, 3.25, 4}),
	)
}

func TestNew(t *testing.T) {
	tests := []struct {
		name string
		cols []Series
		err  error
	}{
		{"ok", []Series{NewColumn("a", []int64{1}), NewColumn("b", []string{"x"})}, nil},
		{"empty", nil, nil},
		{"length", []Series{NewColumn("a", []int64{1}), NewColumn("b", []string{"x", "y"})}, ErrLength},
		{"duplicate", []Series{NewColumn("a", []int64{1}), NewColumn("a", []string{"x"})}, ErrDuplicate},
	}
	for _, tt := range tests {
		_, err := New(tt.cols...)
		if !errors.Is(err, tt.err) {
			t.Errorf("%s: err = %v, want %v", tt.name, err, tt.err)
		}
	}

	if _, err := NewNullColumn("a", []int64{1, 2}, []bool{true}); !errors.Is(err, ErrLength) {
		t.Errorf("NewNullColumn err = %v", err)
	}
}

func TestHeadTail(t *testing.T) {
	f := people(t)
	tests := []struct {
		got  *Frame
		want []any
	}{
		{f.Head(2), []any{"ann", "bob"}},
		{f.Head(10), []any{"ann", "bob", "cat", "dan"}},
		{f.Head(-1), nil},
		{f.Tail(1), []any{"dan"}},
		{f.Tail(0), nil},
	}
	for i, tt := range tests {
		var got []any
		for _, r := range rowsOf(tt.got) {
			got = append(got, r[0])
		}
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("case %d: %v, want %v", i, got, tt.want)
		}
	}
}

func TestSelectDropRename(t *testing.T) {
	f := people(t)

	s, err := f.Select("score", "name")
	if err != nil || !reflect.DeepEqual(s.Columns(), []string{"score", "name"}) {
		t.Fatalf("Select = %v, %v", s, err)
	}
	if _, err := f.Select("nope"); !errors.Is(err, ErrNoColumn) {
		t.Fatalf("Select missing err = %v", err)
	}

	d, err := f.Drop("age")
	if err != nil || !reflect.DeepEqual(d.Columns(), []string{"name", "score"}) {
		t.Fatalf("Drop = %v, %v", d, err)
	}
	if _, err := f.Drop("nope"); !errors.Is(err, ErrNoColumn) {
		t.Fatalf("Drop missing err = %v", err)
	}

	r, err := f.Rename("age", "years")
	if err != nil || !reflect.DeepEqual(r.Columns(), []string{"name", "years", "score"}) {
		t.Fatalf("Rename = %v, %v", r, err)
	}
	if _, err := r.Column("years"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Rename("age", "name"); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("Rename clash err = %v", err)
	}
}

func TestGet(t *testing.T) {
	f := people(t)
	c, err := Get[float64](f, "score")
	if err != nil || c.Len() != 4 {
		t.Fatalf("Get = %v, %v", c, err)
	}
	if _, err := Get[string](f, "score"); !errors.Is(err, ErrType) {
		t.Fatalf("Get wrong type err = %v", err)
	}
}

func TestFilter(t *testing.T) {
	f := people(t)

	got := f.Filter(func(r Row) bool {
		age, ok := r.Int("age")
		return ok && age > 30
	})
	checkRows(t, got, []string{"name", "age", "score"}, [][]any{
		{"ann", int64(31), 1.5},
		{"dan", int64(40), 4.0},
	})

	got, err := Where(f, "score", func(v float64) bool { return v >= 3 })
	if err != nil {
		t.Fatal(err)
	}
	if got.Len() != 2 {
		t.Fatalf("Where len = %d", got.Len())
	}
	if _, err := Where(f, "score", func(v int64) bool { return true }); !errors.Is(err, ErrType) {
		t.Fatalf("Where wrong type err = %v", err)
	}
	if got := f.Filter(func(r Row) bool { return r.IsNull("age") }); got.Len() != 1 {
		t.Fatalf("null filter len = %d", got.Len())
	}
}

func TestWithColumn(t *testing.T) {
	f := people(t)
	double := Compute(f, "double", func(r Row) (float64, bool) {
		age, ok := r.Int("age")
		return float64(age) * 2, ok
	})
	got, err := f.WithColumn(double)
	if err != nil {
		t.Fatal(err)
	}
	c, _ := Get[float64](got, "double")
	if v, ok := c.At(0); !ok || v != 62 {
		t.Fatalf("double[0] = %v, %v", v, ok)
	}
	if !c.IsNull(1) {
		t.Fatal("double[1] should be null")
	}

	replaced, err := f.WithColumn(NewColumn("age", []string{"a", "b", "c", "d"}))
	if err != nil || !reflect.DeepEqual(replaced.Columns(), f.Columns()) {
		t.Fatalf("replace = %v, %v", replaced, err)
	}
	if _, err := f.WithColumn(NewColumn("x", []int64{1})); !errors.Is(err, ErrLength) {
		t.Fatalf("short column err = %v", err)
	}
}

func TestSort(t *testing.T) {
	f := mustNew(t,
		NewColumn("g", []string{"b", "a", "b", "a", "a"}),
		nullable(t, "v", []int64{1, 0, 3, 2, 2}, []bool{true, false, true, true, true}),
		NewColumn("id", []int64{0, 1, 2, 3, 4}),
	)
	tests := []struct {
		keys []SortKey
		want []int64
	}{
		{[]SortKey{Asc("v")}, []int64{0, 3, 4, 2, 1}},
		{[]SortKey{Desc("v")}, []int64{2, 3, 4, 0, 1}},
		{[]SortKey{Asc("g"), Desc("v")}, []int64{3, 4, 1, 2, 0}},
		{[]SortKey{Desc("g"), Asc("v")}, []int64{0, 2, 3, 4, 1}},
		{nil, []int64{0, 1, 2, 3, 4}},
	}
	for _, tt := range tests {
		got, err := f.Sort(tt.keys...)
		if err != nil {
			t.Fatal(err)
		}
		ids, _ := Get[int64](got, "id")
		if !reflect.DeepEqual(ids.Values(), tt.want) {
			t.Errorf("Sort(%v) ids = %v, want %v", tt.keys, ids.Values(), tt.want)
		}
	}
	if _, err := f.Sort(Asc("nope")); !errors.Is(err, ErrNoColumn) {
		t.Fatalf("err = %v", err)
	}
}

func TestSortTypes(t *testing.T) {
	day := func(d int) time.Time { return time.Date(2024, 1, d, 0, 0, 0, 0, time.UTC) }
	f := mustNew(t,
		NewColumn("b", []bool{true, false, true}),
		NewColumn("t", []time.Time{day(3), day(1), day(2)}),
		NewColumn("f", []float64{0.5, -1, 2}),
	)
	tests := []struct {
		key  string
		want []any
	}{
		{"b", []any{false, true, true}},
		{"t", []any{day(1), day(2), day(3)}},
		{"f", []any{-1.0, 0.5, 2.0}},
	}
	for _, tt := range tests {
		got, _ := f.Sort(Asc(tt.key))
		c, _ := got.Column(tt.key)
		var vals []any
		for i := range c.Len() {
			vals = append(vals, c.Value(i))
		}
		if !reflect.DeepEqual(vals, tt.want) {
			t.Errorf("sort %s = %v, want %v", tt.key, vals, tt.want)
		}
	}
}

func TestFillDropNull(t *testing.T) {
	f := people(t)

	filled, err := f.FillNull("age", 0)
	if err != nil {
		t.Fatal(err)
	}
	if c, _ := filled.Column("age"); c.NullCount() != 0 || c.Value(1) != int64(0) {
		t.Fatalf("filled age = %v", c.Value(1))
	}
	if _, err := f.FillNull("age", "x"); !errors.Is(err, ErrType) {
		t.Fatalf("FillNull wrong type err = %v", err)
	}
	if _, err := f.FillNull("score", 1.5); err != nil {
		t.Fatal(err)
	}

	for _, cols := range [][]string{nil, {"age"}} {
		got, err := f.DropNull(cols...)
		if err != nil || got.Len() != 3 {
			t.Fatalf("DropNull(%v) = %d rows, %v", cols, got.Len(), err)
		}
	}
	if got, _ := f.DropNull("name"); got.Len() != 4 {
		t.Fatalf("DropNull(name) = %d rows", got.Len())
	}
}

func TestUnique(t *testing.T) {
	f := mustNew(t,
		NewColumn("a", []string{"x", "y", "x", "x", "y"}),
		nullable(t, "b", []int64{1, 1, 1, 0, 0}, []bool{true, true, true, false, false}),
		NewColumn("id", []int64{0, 1, 2, 3, 4}),
	)
	tests := []struct {
		cols []string
		want []int64
	}{
		{[]string{"a"}, []int64{0, 1}},
		{[]string{"a", "b"}, []int64{0, 1, 3, 4}},
		{[]string{"b"}, []int64{0, 3}},
		{nil, []int64{0, 1, 2, 3, 4}},
	}
	for _, tt := range tests {
		got, err := f.Unique(tt.cols...)
		if err != nil {
			t.Fatal(err)
		}
		ids, _ := Get[int64](got, "id")
		if !reflect.DeepEqual(ids.Values(), tt.want) {
			t.Errorf("Unique(%v) = %v, want %v", tt.cols, ids.Values(), tt.want)
		}
	}
}

func TestGroupBy(t *testing.T) {
	f := mustNew(t,
		NewColumn("team", []string{"red", "blue", "red", "blue", "red"}),
		nullable(t, "pts", []int64{10, 7, 0, 3, 20}, []bool{true, true, false, true, true}),
		NewColumn("w", []float64{1, 2, 3, 4, 5}),
		NewColumn("who", []string{"a", "b", "c", "d", "e"}),
	)
	got, err := f.GroupBy("team").Agg(
		Sum("pts"), Mean("pts"), Min("pts"), Max("pts"), Count("pts").As("n"),
		First("who"), Last("who"), Std("w"), Sum("w").As("w_total"),
	)
	if err != nil {
		t.Fatal(err)
	}
	checkRows(t, got,
		[]string{"team", "pts_sum", "pts_mean", "pts_min", "pts_max", "n", "who_first", "who_last", "w_std", "w_total"},
		[][]any{
			{"red", int64(30), 15.0, int64(10), int64(20), int64(2), "a", "e", 2.0, 9.0},
			{"blue", int64(10), 5.0, int64(3), int64(7), int64(2), "b", "d", math.Sqrt2, 6.0},
		})
}

func TestGroupByMultiKeyAndNulls(t *testing.T) {
	f := mustNew(t,
		nullable(t, "a", []string{"x", "", "x", ""}, []bool{true, false, true, false}),
		NewColumn("b", []int64{1, 1, 2, 1}),
		nullable(t, "v", []float64{1, 2, 3, 0}, []bool{true, true, true, false}),
	)
	got, err := f.GroupBy("a", "b").Agg(Sum("v"), Mean("v"), Std("v"), First("v"))
	if err != nil {
		t.Fatal(err)
	}
	checkRows(t, got, []string{"a", "b", "v_sum", "v_mean", "v_std", "v_first"}, [][]any{
		{"x", int64(1), 1.0, 1.0, nil, 1.0},
		{nil, int64(1), 2.0, 2.0, nil, 2.0},
		{"x", int64(2), 3.0, 3.0, nil, 3.0},
	})
}

func TestGroupByErrors(t *testing.T) {
	f := people(t)
	tests := []struct {
		name string
		g    *GroupBy
		aggs []Agg
		err  error
	}{
		{"no keys", f.GroupBy(), []Agg{Count("age")}, nil},
		{"missing key", f.GroupBy("nope"), []Agg{Count("age")}, ErrNoColumn},
		{"missing col", f.GroupBy("name"), []Agg{Sum("nope")}, ErrNoColumn},
		{"sum string", f.GroupBy("age"), []Agg{Sum("name")}, ErrType},
		{"duplicate name", f.GroupBy("name"), []Agg{Sum("age"), Mean("age").As("age_sum")}, ErrDuplicate},
	}
	for _, tt := range tests {
		_, err := tt.g.Agg(tt.aggs...)
		if err == nil || (tt.err != nil && !errors.Is(err, tt.err)) {
			t.Errorf("%s: err = %v, want %v", tt.name, err, tt.err)
		}
	}
}

func TestJoin(t *testing.T) {
	left := mustNew(t,
		nullable(t, "id", []int64{1, 2, 3, 0}, []bool{true, true, true, false}),
		NewColumn("v", []string{"a", "b", "c", "n"}),
	)
	right := mustNew(t,
		nullable(t, "id", []int64{2, 4, 2, 0}, []bool{true, true, true, false}),
		NewColumn("v", []float64{20, 40, 21, 0}),
	)
	cols := []string{"id", "v", "v_right"}
	tests := []struct {
		how  JoinType
		want [][]any
	}{
		{Inner, [][]any{
			{int64(2), "b", 20.0},
			{int64(2), "b", 21.0},
		}},
		{Left, [][]any{
			{int64(1), "a", nil},
			{int64(2), "b", 20.0},
			{int64(2), "b", 21.0},
			{int64(3), "c", nil},
			{nil, "n", nil},
		}},
		{Right, [][]any{
			{int64(2), "b", 20.0},
			{int64(4), nil, 40.0},
			{int64(2), "b", 21.0},
			{nil, nil, 0.0},
		}},
		{Outer, [][]any{
			{int64(1), "a", nil},
			{int64(2), "b", 20.0},
			{int64(2), "b", 21.0},
			{int64(3), "c", nil},
			{nil, "n", nil},
			{int64(4), nil, 40.0},
			{nil, nil, 0.0},
		}},
	}
	for _, tt := range tests {
		got, err := left.Join(right, tt.how, "id")
		if err != nil {
			t.Fatal(err)
		}
		checkRows(t, got, cols, tt.want)
	}
}

func TestJoinMultiKey(t *testing.T) {
	left := mustNew(t,
		NewColumn("a", []string{"x", "x", "y"}),
		NewColumn("b", []int64{1, 2, 1}),
		NewColumn("l", []bool{true, false, true}),
	)
	right := mustNew(t,
		NewColumn("b", []int64{1, 1}),
		NewColumn("a", []string{"y", "x"}),
		NewColumn("r", []int64{10, 20}),
	)
	got, err := left.Join(right, Inner, "a", "b")
	if err != nil {
		t.Fatal(err)
	}
	checkRows(t, got, []string{"a", "b", "l", "r"}, [][]any{
		{"x", int64(1), true, int64(20)},
		{"y", int64(1), true, int64(10)},
	})

	bad := mustNew(t, NewColumn("a", []int64{1}), NewColumn("b", []int64{1}))
	if _, err := left.Join(bad, Inner, "a"); !errors.Is(err, ErrType) {
		t.Fatalf("type mismatch err = %v", err)
	}
	if _, err := left.Join(right, Inner); err == nil {
		t.Fatal("expected error for no keys")
	}
	if _, err := left.Join(right, Inner, "l"); !errors.Is(err, ErrNoColumn) {
		t.Fatalf("missing key err = %v", err)
	}
}

func TestDescribe(t *testing.T) {
	f := mustNew(t,
		NewColumn("s", []string{"a", "b", "c", "d"}),
		nullable(t, "x", []int64{1, 2, 3, 4}, []bool{true, true, true, true}),
		nullable(t, "y", []float64{0, 0, 0, 0}, []bool{false, false, false, false}),
	)
	d := f.Describe()
	checkRows(t, d, []string{"stat", "x", "y"}, [][]any{
		{"count", 4.0, 0.0},
		{"mean", 2.5, nil},
		{"std", math.Sqrt(5.0 / 3), nil},
		{"min", 1.0, nil},
		{"25%", 1.75, nil},
		{"50%", 2.5, nil},
		{"75%", 3.25, nil},
		{"max", 4.0, nil},
	})
}

func TestString(t *testing.T) {
	f := mustNew(t,
		NewColumn("name", []string{"ann", "a very long name that keeps going on"}),
		nullable(t, "x", []float64{1.5, 0}, []bool{true, false}),
		NewColumn("d", []time.Time{time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC), time.Date(2024, 3, 2, 0, 0, 0, 0, time.UTC)}),
	)
	want := "" +
		"                             name     x           d\n" +
		"0                             ann   1.5  2024-03-01\n" +
		"1  a very long name that keeps...  null  2024-03-02\n"
	if got := f.String(); got != want {
		t.Fatalf("String() =\n%s\nwant\n%s", got, want)
	}

	long := make([]int64, 100)
	for i := range long {
		long[i] = int64(i)
	}
	s := mustNew(t, NewColumn("n", long)).String()
	if !strings.Contains(s, "...") || !strings.Contains(s, "\n 99   99\n") || strings.Contains(s, "50") ||
		!strings.HasSuffix(s, "[100 rows x 1 columns]\n") {
		t.Fatalf("truncated String() =\n%s", s)
	}
}

func TestFromRecords(t *testing.T) {
	when := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
	f, err := FromRecords([]string{"i", "f", "s", "b", "t", "n"}, [][]any{
		{1, 1, "a", true, when, nil},
		{int32(2), 2.5, nil, false, nil, nil},
	})
	if err != nil {
		t.Fatal(err)
	}
	checkRows(t, f, []string{"i", "f", "s", "b", "t", "n"}, [][]any{
		{int64(1), 1.0, "a", true, when, nil},
		{int64(2), 2.5, nil, false, nil, nil},
	})
	types := []Type{Int64, Float64, String, Bool, Time, String}
	for i, name := range f.Columns() {
		if c, _ := f.Column(name); c.Type() != types[i] {
			t.Errorf("%s type = %s, want %s", name, c.Type(), types[i])
		}
	}

	tests := []struct {
		rows [][]any
		err  error
	}{
		{[][]any{{1, 2}}, ErrLength},
		{[][]any{{1}, {"x"}}, ErrType},
		{[][]any{{[]int{1}}}, ErrType},
		{[][]any{{uint64(math.MaxUint64)}}, ErrType},
	}
	for _, tt := range tests {
		if _, err := FromRecords([]string{"a"}, tt.rows); !errors.Is(err, tt.err) {
			t.Errorf("FromRecords(%v) err = %v, want %v", tt.rows, err, tt.err)
		}
	}
}

func TestFromStructs(t *testing.T) {
	type level string
	type rec struct {
		ID      int       `frame:"id"`
		Name    string    `frame:"name"`
		Score   *float64  `frame:"score"`
		Level   level     `frame:"level"`
		Active  bool      `frame:"active"`
		Joined  time.Time `frame:"joined"`
		Skip    string    `frame:"-"`
		private int
	}
	score := 9.5
	when := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
	rows := []rec{
		{ID: 1, Name: "ann", Score: &score, Level: "gold", Active: true, Joined: when, Skip: "x"},
		{ID: 2, Name: "bob", Level: "silver"},
	}
	f, err := FromStructs(rows)
	if err != nil {
		t.Fatal(err)
	}
	checkRows(t, f, []string{"id", "name", "score", "level", "active", "joined"}, [][]any{
		{int64(1), "ann", 9.5, "gold", true, when},
		{int64(2), "bob", nil, "silver", false, time.Time{}},
	})

	ptrs, err := FromStructs([]*rec{&rows[0]})
	if err != nil || ptrs.Len() != 1 {
		t.Fatalf("pointers = %v, %v", ptrs, err)
	}
	if _, err := FromStructs([]*rec{nil}); err == nil {
		t.Fatal("expected error for nil row")
	}
	if _, err := FromStructs([]int{1}); err == nil {
		t.Fatal("expected error for non-struct")
	}
	type bad struct{ M map[string]int }
	if _, err := FromStructs([]bad{{}}); err == nil {
		t.Fatal("expected error for unsupported field")
	}
}

func TestImmutability(t *testing.T) {
	data := []float64{3, 1, 2}
	col := NewColumn("x", data)
	f := mustNew(t, col, nullable(t, "y", []int64{1, 0, 3}, []bool{true, false, true}))
	before := f.String()

	data[0] = 100
	vals := col.Values()
	vals[1] = 100

	_, _ = f.Sort(Desc("x"))
	_, _ = f.FillNull("y", 9)
	_, _ = f.Rename("x", "z")
	_, _ = f.WithColumn(NewColumn("x", []float64{0, 0, 0}))
	_, _ = f.Drop("x")
	_, _ = f.Join(f, Outer, "x")
	_, _ = f.GroupBy("x").Agg(Sum("y"))
	_ = f.Filter(func(Row) bool { return false })
	_ = f.Describe()

	if got := f.String(); got != before {
		t.Fatalf("frame changed:\n%s\nwas\n%s", got, before)
	}
	if c, _ := f.Column("x"); c.Name() != "x" {
		t.Fatal("rename leaked into original column")
	}
}
