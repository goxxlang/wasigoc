// go.mod module example.com/app: import paths resolve at the module root,
// including a nested package (shape/palette) that itself imports shape.
package main

import (
	"example.com/app/shape"
	"example.com/app/shape/palette"
	"fmt"
)

func main() {
	p := shape.NewPoint(2, 3)
	fmt.Println("sum:", p.Sum())
	r := palette.Red()
	fmt.Println("red:", r.X)
}