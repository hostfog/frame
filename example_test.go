package frame_test

import (
	"fmt"
	"os"
	"strings"

	"github.com/hostfog/frame"
)

func Example() {
	csv := `city,month,sales
London,1,120.5
Leeds,1,80
London,2,99.5
Leeds,2,NA
`
	df, err := frame.ReadCSV(strings.NewReader(csv), frame.CSVOptions{})
	if err != nil {
		panic(err)
	}

	totals, err := df.GroupBy("city").Agg(frame.Sum("sales").As("total"), frame.Count("sales"))
	if err != nil {
		panic(err)
	}
	totals, _ = totals.Sort(frame.Desc("total"))
	fmt.Print(totals)
	// Output:
	//      city  total  sales_count
	// 0  London    220            2
	// 1   Leeds     80            1
}

func ExampleFromStructs() {
	type order struct {
		ID    int     `frame:"id"`
		Item  string  `frame:"item"`
		Price float64 `frame:"price"`
		Qty   int     `frame:"qty"`
	}
	df, _ := frame.FromStructs([]order{
		{1, "tea", 2.5, 2},
		{2, "cake", 3.25, 1},
	})

	total := frame.Compute(df, "total", func(r frame.Row) (float64, bool) {
		price, ok1 := r.Float("price")
		qty, ok2 := r.Int("qty")
		return price * float64(qty), ok1 && ok2
	})
	df, _ = df.WithColumn(total)
	fmt.Print(df)
	// Output:
	//    id  item  price  qty  total
	// 0   1   tea   2.50    2   5.00
	// 1   2  cake   3.25    1   3.25
}

func ExampleFrame_Join() {
	users, _ := frame.New(
		frame.NewColumn("id", []int64{1, 2, 3}),
		frame.NewColumn("name", []string{"ann", "bob", "cat"}),
	)
	orders, _ := frame.New(
		frame.NewColumn("id", []int64{1, 1, 3}),
		frame.NewColumn("amount", []float64{10, 5, 7}),
	)

	joined, _ := users.Join(orders, frame.Left, "id")
	joined.WriteJSON(os.Stdout)
	// Output:
	// [{"id":1,"name":"ann","amount":10.0},{"id":1,"name":"ann","amount":5.0},{"id":2,"name":"bob","amount":null},{"id":3,"name":"cat","amount":7.0}]
}

func ExampleWhere() {
	df, _ := frame.New(
		frame.NewColumn("name", []string{"ann", "bob", "cat"}),
		frame.NewColumn("age", []int64{31, 17, 45}),
	)
	adults, _ := frame.Where(df, "age", func(age int64) bool { return age >= 18 })
	fmt.Print(adults)
	// Output:
	//    name  age
	// 0   ann   31
	// 1   cat   45
}
