package main

import (
	"fmt"
	"math"
	"path"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

func main() {
	fmt.Println(strings.ToUpper("hi"))
	n, err := strconv.Atoi("42")
	if err != nil {
		fmt.Println("err")
		return
	}
	fmt.Println(n)
	if strings.Contains("hello", "ell") {
		fmt.Println("yes")
	}
	fmt.Println(utf8.RuneCountInString("Go"))
	xs := []int{3, 1, 2}
	sort.Ints(xs)
	fmt.Println(xs[0], xs[1], xs[2])
	fmt.Println(path.Base("a/b/c.go"))
	if math.Abs(-3.0) == 3.0 {
		fmt.Println("abs")
	}
}
