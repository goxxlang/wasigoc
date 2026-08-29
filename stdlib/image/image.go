// Bounded subset of image: `Point`/`Rectangle` (real Go's own shape, half-
// open on the Max corner), the `Image` interface, and ONE concrete pixel
// buffer type, `RGBA` (byte-per-channel, `Pix`/`Stride`/`Rect`, same
// layout real Go's own `image.RGBA` uses). No `NRGBA`/`Gray`/`Paletted`/
// etc. concrete types, no `Decode`/format registry, no `Uniform`/`Alpha`
// masks, no `Draw`. Real Go's `image.RGBA` and `image/color.RGBA` share a
// name (different packages, no collision there) -- this project's own
// `image/color.Rgba` is already the case-only rename its own tracker line
// explains; nothing about that renaming affects this package.
package image

import "image/color"

type Point struct {
	X int
	Y int
}

func Pt(x int, y int) Point {
	return Point{X: x, Y: y}
}

func (p Point) Add(q Point) Point {
	return Point{X: p.X + q.X, Y: p.Y + q.Y}
}

func (p Point) Sub(q Point) Point {
	return Point{X: p.X - q.X, Y: p.Y - q.Y}
}

func (p Point) In(r Rectangle) bool {
	return r.Min.X <= p.X && p.X < r.Max.X && r.Min.Y <= p.Y && p.Y < r.Max.Y
}

func (p Point) Eq(q Point) bool {
	return p.X == q.X && p.Y == q.Y
}

type Rectangle struct {
	Min Point
	Max Point
}

func Rect(x0 int, y0 int, x1 int, y1 int) Rectangle {
	return Rectangle{Min: Point{X: x0, Y: y0}, Max: Point{X: x1, Y: y1}}
}

func (r Rectangle) Dx() int {
	return r.Max.X - r.Min.X
}

func (r Rectangle) Dy() int {
	return r.Max.Y - r.Min.Y
}

func (r Rectangle) Empty() bool {
	return r.Min.X >= r.Max.X || r.Min.Y >= r.Max.Y
}

func (r Rectangle) Eq(s Rectangle) bool {
	return r.Min.Eq(s.Min) && r.Max.Eq(s.Max) || (r.Empty() && s.Empty())
}

func (r Rectangle) In(s Rectangle) bool {
	if r.Empty() {
		return true
	}
	return s.Min.X <= r.Min.X && r.Max.X <= s.Max.X && s.Min.Y <= r.Min.Y && r.Max.Y <= s.Max.Y
}

func (r Rectangle) Add(p Point) Rectangle {
	return Rectangle{Min: r.Min.Add(p), Max: r.Max.Add(p)}
}

func (r Rectangle) Sub(p Point) Rectangle {
	return Rectangle{Min: r.Min.Sub(p), Max: r.Max.Sub(p)}
}

func maxInt(a int, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a int, b int) int {
	if a < b {
		return a
	}
	return b
}

func (r Rectangle) Intersect(s Rectangle) Rectangle {
	x0 := maxInt(r.Min.X, s.Min.X)
	y0 := maxInt(r.Min.Y, s.Min.Y)
	x1 := minInt(r.Max.X, s.Max.X)
	y1 := minInt(r.Max.Y, s.Max.Y)
	if x0 >= x1 || y0 >= y1 {
		return Rectangle{}
	}
	return Rectangle{Min: Point{X: x0, Y: y0}, Max: Point{X: x1, Y: y1}}
}

func (r Rectangle) Union(s Rectangle) Rectangle {
	if r.Empty() {
		return s
	}
	if s.Empty() {
		return r
	}
	x0 := minInt(r.Min.X, s.Min.X)
	y0 := minInt(r.Min.Y, s.Min.Y)
	x1 := maxInt(r.Max.X, s.Max.X)
	y1 := maxInt(r.Max.Y, s.Max.Y)
	return Rectangle{Min: Point{X: x0, Y: y0}, Max: Point{X: x1, Y: y1}}
}

type Image interface {
	ColorModel() color.Model
	Bounds() Rectangle
	At(x int, y int) color.Color
}

type RGBA struct {
	Pix    []byte
	Stride int
	Rect   Rectangle
}

func NewRGBA(r Rectangle) *RGBA {
	w := r.Dx()
	h := r.Dy()
	return &RGBA{Pix: make([]byte, 4*w*h), Stride: 4 * w, Rect: r}
}

func (p *RGBA) ColorModel() color.Model {
	return color.RGBAModel
}

func (p *RGBA) Bounds() Rectangle {
	return p.Rect
}

func (p *RGBA) PixOffset(x int, y int) int {
	return (y-p.Rect.Min.Y)*p.Stride + (x-p.Rect.Min.X)*4
}

func (p *RGBA) At(x int, y int) color.Color {
	if !(Point{X: x, Y: y}.In(p.Rect)) {
		return color.Rgba{}
	}
	i := p.PixOffset(x, y)
	return color.Rgba{R: p.Pix[i], G: p.Pix[i+1], B: p.Pix[i+2], A: p.Pix[i+3]}
}

func (p *RGBA) Set(x int, y int, c color.Color) {
	if !(Point{X: x, Y: y}.In(p.Rect)) {
		return
	}
	i := p.PixOffset(x, y)
	c1, _ := color.RGBAModel.Convert(c).(color.Rgba)
	p.Pix[i] = c1.R
	p.Pix[i+1] = c1.G
	p.Pix[i+2] = c1.B
	p.Pix[i+3] = c1.A
}
