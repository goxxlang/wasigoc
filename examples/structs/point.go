package main

import "fmt"

type Point struct {
	X int
	Y int
}

func (p *Point) Scale(f int) {
	p.X = p.X * f
	p.Y = p.Y * f
}

func (p Point) Sum() int {
	return p.X + p.Y
}

func NewPoint(x int, y int) Point {
	return Point{X: x, Y: y}
}

func main() {
	p := NewPoint(2, 3)
	fmt.Println("sum before scale:", p.Sum())

	p.Scale(5)
	fmt.Println("x:", p.X, "y:", p.Y)
	fmt.Println("sum after scale:", p.Sum())

	origin := Point{}
	fmt.Println("origin sum:", origin.Sum())
}
