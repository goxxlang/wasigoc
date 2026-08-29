// Second file in package geom: directory imports merge every *.go
// (except *_test.go) into one namespace / one generated header.
package geom

func Origin() Point {
	return Point{X: 0, Y: 0}
}
