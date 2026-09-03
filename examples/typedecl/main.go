// Three type-declaration/expression corner cases found compiling
// ~/project_lovelace's wasm decompiler through wasigoc and fixed here:
// a named array/slice type's `[...]` was always parsed as a generic
// type-parameter list, grouped struct field names (`A, B T`) weren't
// supported at all, and a generic struct instantiated directly as a
// composite literal (`Pair[int]{...}`, as opposed to a `var` declaration)
// both failed to parse and, once parsing was fixed, failed to compile
// (CTAD with no constructor argument to deduce from).
package main

import "fmt"

const maxSize = 4

type ByteSlice []byte
type Block [64]int32
type Named [maxSize]byte

type Pair[T any] struct {
	A, B T
}

type Triple[T any] struct {
	X, Y, Z T
}

type Map2[K comparable, V any] struct {
	m map[K]V
}

func main() {
	var bs ByteSlice = ByteSlice{1, 2, 3}
	fmt.Println(len(bs))

	var b Block
	b[0] = 42
	fmt.Println(b[0])

	var n Named
	n[1] = 7
	fmt.Println(n[1], len(n))

	p := Pair[int]{A: 1, B: 2}
	fmt.Println(p.A + p.B)

	t := Triple[string]{X: "a", Y: "b", Z: "c"}
	fmt.Println(t.X + t.Y + t.Z)

	m2 := Map2[string, int]{m: map[string]int{"x": 5}}
	fmt.Println(m2.m["x"])

	nums := []int{10, 20, 30}
	fmt.Println(nums[1])
}
