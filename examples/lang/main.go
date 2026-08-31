// Language surface that was missing: init, named results, fallthrough,
// type aliases, range over string, []byte conversions.
package main

import "fmt"

var boot = 0

func init() {
	boot = boot + 1
}

func init() {
	boot = boot + 2
}

type ID int

func pair() (x int, y int) {
	x = 3
	y = 4
	return
}

func classify(n int) string {
	switch n {
	case 1:
		return "one"
	case 2:
		fallthrough
	case 3:
		return "two-or-three"
	default:
		return "other"
	}
}

func main() {
	fmt.Println(boot)
	a, b := pair()
	fmt.Println(a + b)
	fmt.Println(classify(2))
	var id ID = ID(9)
	fmt.Println(int(id))
	s := "Go"
	for i, r := range s {
		fmt.Println(i, r)
	}
	fmt.Println(string([]byte(s)))
	const N = 4
	var buf [N]int
	fmt.Println(len(buf))
	var big [1 << 3]byte
	fmt.Println(len(big))
	sum := 0
	for i := range 3 {
		sum = sum + i
	}
	fmt.Println(sum)
}
