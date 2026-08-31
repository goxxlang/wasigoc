package main

import "fmt"

type Duration int64

func (d Duration) String() string {
	if d == 0 {
		return "0"
	}
	return "d"
}

type Set[T any] struct {
	n int
}

func (s *Set[T]) Add() {
	s.n = s.n + 1
}

type Box struct {
	v int
}

func (b Box) N() int {
	return b.v
}

func take(r interface{ N() int }) int {
	return r.N()
}

func seq(yield func(int) bool) {
	if !yield(1) {
		return
	}
	yield(2)
}

func main() {
	d := Duration(0)
	fmt.Println(d.String())
	var s Set[int]
	s.Add()
	s.Add()
	fmt.Println(s.n)
	fmt.Println(take(Box{v: 7}))
	sum := 0
	for v := range seq {
		sum = sum + v
	}
	fmt.Println(sum)
}
