// Named interface assignment and type assert. Concrete values are boxed by
// adapt(); x.(T) uses type_key rather than RTTI (wasi-sdk noeh has none).
package main

import "fmt"

type Point struct {
	X int
	Y int
}

func (p Point) Sum() int {
	return p.X + p.Y
}

type Adder interface {
	Sum() int
}

func printSum(a Adder) {
	fmt.Println("sum:", a.Sum())
}

func main() {
	p := Point{X: 2, Y: 3}
	printSum(p)
	var a Adder = p
	v, ok := a.(Point)
	if ok {
		fmt.Println("x:", v.X)
	}
	_, ok2 := a.(string)
	if !ok2 {
		fmt.Println("not string")
	}
}
