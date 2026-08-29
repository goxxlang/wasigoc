// Type switch, import alias, append unpack, arrays, &^, os.Args.
package main

import (
	"fmt"
	str "strings"
	"os"
)

type Adder interface {
	Sum() int
}

type Point struct {
	X int
	Y int
}

func (p Point) Sum() int {
	return p.X + p.Y
}

func describe(a Adder) string {
	switch v := a.(type) {
	case Point:
		return str.ToUpper("pt")
	case nil:
		return "nil"
	default:
		_ = v
		return "other"
	}
}

func main() {
	p := Point{X: 1, Y: 2}
	fmt.Println(describe(p))
	more := []int{4, 5}
	nums := []int{1, 2, 3}
	nums = append(nums, more...)
	fmt.Println(len(nums))
	var arr [2]int
	arr[0] = 7
	arr[1] = 8
	sum := 0
	for _, n := range arr {
		sum += n
	}
	fmt.Println(sum)
	x := 5
	fmt.Println(x &^ 1)
	if len(os.Args) > 0 {
		fmt.Println("args")
	}
	b := make([]byte, 2)
	copy(b, "hi")
	fmt.Println(string(b))
}
