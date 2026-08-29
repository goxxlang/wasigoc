package main

import (
	"fmt"
	"structs"
)

type Point struct {
	Layout structs.HostLayout
	X      int
	Y      int
}

func main() {
	p := Point{X: 3, Y: 4}
	fmt.Println(p.X == 3)
	fmt.Println(p.Y == 4)
}
