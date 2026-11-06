package frame

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	maxRows   = 20 // frames longer than this are truncated when printed
	edgeRows  = 5  // rows shown at each end of a truncated frame
	maxString = 30
)

// String renders the frame as an aligned table. Long frames show only their
// first and last rows.
func (f *Frame) String() string {
	rows := make([]int, 0, f.n)
	truncated := f.n > maxRows
	if truncated {
		for i := range edgeRows {
			rows = append(rows, i)
		}
		rows = append(rows, -1)
		for i := f.n - edgeRows; i < f.n; i++ {
			rows = append(rows, i)
		}
	} else {
		for i := range f.n {
			rows = append(rows, i)
		}
	}

	table := make([][]string, len(f.cols)+1)
	table[0] = append(table[0], "")
	for _, i := range rows {
		table[0] = append(table[0], rowLabel(i))
	}
	for c, s := range f.cols {
		cell := formatter(s, rows)
		col := []string{s.Name()}
		for _, i := range rows {
			switch {
			case i < 0:
				col = append(col, "...")
			case s.IsNull(i):
				col = append(col, "null")
			default:
				col = append(col, cell(i))
			}
		}
		table[c+1] = col
	}

	widths := make([]int, len(table))
	for c, col := range table {
		for _, v := range col {
			widths[c] = max(widths[c], utf8.RuneCountInString(v))
		}
	}

	var b strings.Builder
	for r := range len(rows) + 1 {
		for c, col := range table {
			if c > 0 {
				b.WriteString("  ")
			}
			b.WriteString(strings.Repeat(" ", widths[c]-utf8.RuneCountInString(col[r])))
			b.WriteString(col[r])
		}
		b.WriteByte('\n')
	}
	if truncated || f.n == 0 {
		fmt.Fprintf(&b, "\n[%d rows x %d columns]\n", f.n, len(f.cols))
	}
	return b.String()
}

func rowLabel(i int) string {
	if i < 0 {
		return "..."
	}
	return strconv.Itoa(i)
}

// formatter returns a function that formats present values of s, chosen so
// the visible rows line up.
func formatter(s Series, rows []int) func(int) string {
	switch c := s.(type) {
	case *Column[float64]:
		prec := 0
		for _, i := range rows {
			if i >= 0 && !c.IsNull(i) && !math.IsInf(c.data[i], 0) && !math.IsNaN(c.data[i]) {
				str := strconv.FormatFloat(c.data[i], 'f', -1, 64)
				if dot := strings.IndexByte(str, '.'); dot >= 0 {
					prec = max(prec, min(len(str)-dot-1, 6))
				}
			}
		}
		return func(i int) string { return strconv.FormatFloat(c.data[i], 'f', prec, 64) }
	case *Column[time.Time]:
		layout := time.DateOnly
		for _, i := range rows {
			if i >= 0 && !c.IsNull(i) && !c.data[i].Equal(c.data[i].Truncate(24*time.Hour)) {
				layout = time.DateTime
				break
			}
		}
		return func(i int) string { return c.data[i].Format(layout) }
	case *Column[string]:
		return func(i int) string {
			v := c.data[i]
			if utf8.RuneCountInString(v) > maxString {
				v = string([]rune(v)[:maxString-3]) + "..."
			}
			return v
		}
	}
	return func(i int) string { return fmt.Sprint(s.Value(i)) }
}
