package main

import (
	"fmt"
	"image/color"
	"image/color/palette"
)

func main() {
	fmt.Println(len(palette.WebSafe))
	c0 := palette.WebSafe[0].(color.Rgba)
	fmt.Println(c0.R)
	fmt.Println(c0.G)
	fmt.Println(c0.B)
	last := palette.WebSafe[len(palette.WebSafe)-1].(color.Rgba)
	fmt.Println(last.R)
	fmt.Println(last.G)
	fmt.Println(last.B)
	mid := palette.WebSafe[35].(color.Rgba)
	fmt.Println(mid.R)
	fmt.Println(mid.G)
	fmt.Println(mid.B)
}
