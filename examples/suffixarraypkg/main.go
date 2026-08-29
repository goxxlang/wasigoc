package main

import (
	"fmt"
	"index/suffixarray"
	"sort"
)

func main() {
	x := suffixarray.New([]byte("banana"))

	idx := x.Lookup([]byte("ana"), -1)
	sort.Ints(idx)
	fmt.Println(len(idx))
	fmt.Println(idx[0], idx[1])

	one := x.Lookup([]byte("ana"), 1)
	fmt.Println(len(one))

	none := x.Lookup([]byte("xyz"), -1)
	fmt.Println(len(none))

	all := x.Lookup([]byte(""), -1)
	fmt.Println(len(all))

	fmt.Println(string(x.Bytes()))
}
