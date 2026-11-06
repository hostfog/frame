# frame

A pandas-style DataFrame for Go. Columns are typed (`int64`, `float64`, `string`, `bool`, `time.Time`) with a null bitmap, and every operation returns a new frame.

```sh
go get github.com/hostfog/frame
```

## Usage

```go
df, _ := frame.ReadCSV(file, frame.CSVOptions{})

big, _ := frame.Where(df, "sales", func(v float64) bool { return v > 100 })
byCity, _ := df.GroupBy("city").Agg(frame.Sum("sales").As("total"), frame.Mean("sales"))
byCity, _ = byCity.Sort(frame.Desc("total"))

joined, _ := byCity.Join(regions, frame.Left, "city")
fmt.Println(joined)
//      city  total  sales_mean  region
// 0  London    220         110   south
// 1   Leeds     80          80   north
```

Typed access goes through generics, so a wrong type is an error rather than a panic:

```go
sales, err := frame.Get[float64](df, "sales")
v, ok := sales.At(0) // ok is false for null
```

## Features

- **Build** from slices (`NewColumn`, `NewNullColumn`), structs (`FromStructs`, `frame:"name"` tags, pointer fields are nullable), records (`FromRecords`), CSV and JSON records.
- **CSV** type inference with `CSVOptions` for delimiter, explicit types, null markers and time layout.
- **Ops:** `Head`, `Tail`, `Select`, `Drop`, `Rename`, `Filter`, `Where`, `Sort` (multi-key, stable, nulls last), `WithColumn` + `Compute`, `FillNull`, `DropNull`, `Unique`.
- **GroupBy** with `Sum`, `Mean`, `Min`, `Max`, `Count`, `First`, `Last`, `Std`, renamed with `As`.
- **Join:** inner, left, right and outer hash joins on one or more keys. Null keys never match.
- **Describe** and pandas-like printing that truncates long frames.

## License

MIT
