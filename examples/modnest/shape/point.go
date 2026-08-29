// Nested package under the example.com/app module (go.mod). C++ namespace
// is still `shape` -- Go source says shape.Point, not example.com.app.shape.
package shape

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
