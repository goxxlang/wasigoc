// Labeled break, goto, bitwise assign, struct ==, Builder, Sprint.
package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

type Pair struct {
	A int
	B int
}

func sum(xs ...int) int {
	n := 0
	for _, x := range xs {
		n += x
	}
	return n
}

func main() {
	n := 0
Outer:
	for i := 0; i < 5; i++ {
		for j := 0; j < 5; j++ {
			if i == 2 && j == 1 {
				break Outer
			}
			n++
		}
	}
	fmt.Println(n)

	x := 1
	goto Skip
	x = 99
Skip:
	fmt.Println(x)

	flags := 7
	flags &^= 2
	fmt.Println(flags)

	p1 := Pair{A: 1, B: 2}
	p2 := Pair{A: 1, B: 2}
	if p1 == p2 {
		fmt.Println("eq")
	}

	more := []int{4, 5}
	fmt.Println(sum(1, 2, 3))
	fmt.Println(sum(more...))

	var b strings.Builder
	b.WriteString("hi")
	fmt.Println(b.String())
	fmt.Println(strconv.FormatBool(true))
	fmt.Println(fmt.Sprint("s", 1))
	time.Sleep(time.Millisecond)
}
