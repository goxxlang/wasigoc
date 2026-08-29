// any, min/max/clear, method values, array slicing, embedded interfaces,
// bytes.Buffer, path/filepath, errors.Is, strconv.ParseFloat.
package main

import (
	"bytes"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
)

type Point struct {
	X int
	Y int
}

func (p Point) Sum() int {
	return p.X + p.Y
}

type Summer interface {
	Sum() int
}

type Namer interface {
	Name() string
}

type Both interface {
	Summer
	Namer
}

type NamedPoint struct {
	X int
	Y int
}

func (p NamedPoint) Sum() int {
	return p.X + p.Y
}

func (p NamedPoint) Name() string {
	return "np"
}

func takeAny(x any) int {
	v, ok := x.(int)
	if ok {
		return v
	}
	return -1
}

func main() {
	var x any = 7
	fmt.Println(takeAny(x))
	fmt.Println(min(3, 1, 2))
	fmt.Println(max(3, 1, 2))

	s := []int{1, 2, 3}
	clear(s)
	fmt.Println(s[0], s[1], s[2], len(s))
	m := map[string]int{"a": 1}
	clear(m)
	fmt.Println(len(m))

	p := Point{X: 2, Y: 3}
	f := p.Sum
	fmt.Println(f())

	var arr [4]int
	arr[0] = 9
	arr[1] = 8
	arr[2] = 7
	arr[3] = 6
	sl := arr[1:3]
	fmt.Println(sl[0], sl[1], len(sl))

	var b Both = NamedPoint{X: 1, Y: 4}
	fmt.Println(b.Sum(), b.Name())

	var buf bytes.Buffer
	buf.WriteString("hi")
	fmt.Println(buf.String())
	fmt.Println(filepath.Base("a/b/c.go"))

	e1 := errors.New("boom")
	e2 := errors.New("boom")
	if errors.Is(e1, e2) {
		fmt.Println("is")
	}

	fv, err := strconv.ParseFloat("3")
	if err == nil && fv == 3.0 {
		fmt.Println("pf")
	}
}