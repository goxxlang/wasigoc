// Rosetta: Go++ constructs that do not line up 1:1 with C++, mapped onto
// WASM-friendly C++ (cooperative goroutines, RAII defer, bounds-checked
// slices, real error-vs-nil). See src/runtime.hpp and docs/language.md.
package main

import "fmt"

const (
	Red = iota
	Green
	Blue
)

func worker(ch chan int) {
	ch <- 7
}

func boom() {
	defer func() {
		r := recover()
		if r != nil {
			fmt.Println("recovered", r)
		}
	}()
	panic("boom")
}

func main() {
	ch := make(chan int)
	go worker(ch)
	fmt.Println(<-ch)

	fmt.Println("red", Red, "green", Green, "blue", Blue)

	nums := []int{1, 2, 3}
	total := 0
	for _, n := range nums {
		total += n
	}
	fmt.Println("sum", total)

	boom()
}
