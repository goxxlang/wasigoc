// A library file: `package geom` becomes `namespace geom`. Imported by
// examples/importpkg the way WASMVoodooCompile's examples/common is imported
// by examples/logging -- own generated header, own namespace, not flattened.
package geom

type Point struct {
	X int
	Y int
}

func NewPoint(x int, y int) Point {
	return Point{X: x, Y: y}
}

func (p Point) Sum() int {
	return p.X + p.Y
}
