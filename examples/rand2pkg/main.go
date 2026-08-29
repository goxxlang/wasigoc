package main

import (
	"fmt"
	"math/rand/v2"
)

func main() {
	r := rand.New(rand.NewPCG(42, 7))
	fmt.Println(r.IntN(100))
	fmt.Println(r.IntN(100))
	fmt.Println(r.IntN(100))

	r2 := rand.New(rand.NewPCG(42, 7))
	fmt.Println(r2.IntN(100))

	f := r.Float64()
	fmt.Println(f >= 0 && f < 1)

	p := r.Perm(5)
	fmt.Println(len(p))
	seen := make(map[int]bool)
	dup := false
	for _, v := range p {
		if seen[v] {
			dup = true
		}
		seen[v] = true
	}
	fmt.Println(dup)

	fmt.Println(r.Int32N(10) >= 0)
	fmt.Println(r.Uint64N(10) < 10)
}
