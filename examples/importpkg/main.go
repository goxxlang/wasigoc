// Entry file that imports the geom directory. All *.go files in that
// directory (point.go + origin.go) become namespace geom and geom_gen.hpp.
package main

import (
	"fmt"
	"../geom"
)

func main() {
	p := geom.NewPoint(2, 3)
	o := geom.Origin()
	fmt.Println("sum:", p.Sum()+o.Sum())
}
