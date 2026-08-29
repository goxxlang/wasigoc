// Nested package shape/palette importing its parent package -- WASMVoodooCompile
// include-graph shape: palette_gen.hpp #includes shape_gen.hpp (direct import
// only), and the entry TU includes only its own direct imports.
package palette

import "example.com/app/shape"

func Red() shape.Point {
	return shape.Point{X: 1, Y: 0}
}
