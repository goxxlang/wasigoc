// Bounded subset of image/color: the `Color` interface, the four most
// common concrete color types (Rgba/NRGBA/Gray/CMYK -- no Gray16/RGBA64/
// NRGBA64/Alpha/Alpha16/YCbCr), and their `Model`s. No `Palette` (that's
// `image/color/palette`, a separate package). `Model`'s wrapper shape
// (`modelFunc` holding a plain `func(Color) Color`) matches real Go's own
// actual implementation exactly -- a bare Go func can't have methods, so
// real Go needs the same struct-wrapping-a-function trick this package
// uses, not a simplification invented here.
//
// Real Go names the first concrete type `RGBA`, with a method also named
// `RGBA()` (to satisfy `Color`) -- legal in real Go (a method and its own
// type CAN share a name there), but not reproducible here: every method
// this compiler generates is emitted as a same-named C++ member function
// *inside* its receiver's struct body, and C++ always parses a member
// function sharing its enclosing class's exact name as a constructor
// declaration, never an ordinary method -- `struct RGBA { ... RGBA RGBA()
// {...} ... }` fails to compile. Worse, fixing this generally (mangling
// only a colliding method's C++ name) doesn't work either: `Color`'s
// vtable trampoline (`EmitIfaceVTableLambda`) is emitted ONCE as a
// `template<class T>` shared by every concrete type that ever satisfies
// `Color`, calling `static_cast<T*>(__self)->RGBA(...)` -- a single
// generic template body can't conditionally call a different C++ method
// name depending on which concrete T it gets instantiated with, so a
// per-type mangle would need a per-type template specialization
// everywhere `Color` gets adapted, a much bigger change. Renamed the
// STRUCT instead (to `Rgba`, case-only so it stays recognizable) --
// same "rename the Go type, don't teach the compiler to mangle
// same-named methods" precedent already established for `hash/fnv`'s
// `Digest32`/`Digest64` (see the compiler-bugs writeup), just with no
// alternative name available here since callers really do need `Color`
// satisfied by direct construction, not a renamed method.
package color

type Color interface {
	RGBA() (r uint32, g uint32, b uint32, a uint32)
}

type Rgba struct {
	R uint8
	G uint8
	B uint8
	A uint8
}

func (c Rgba) RGBA() (uint32, uint32, uint32, uint32) {
	r := uint32(c.R)
	r = r | (r << 8)
	g := uint32(c.G)
	g = g | (g << 8)
	b := uint32(c.B)
	b = b | (b << 8)
	a := uint32(c.A)
	a = a | (a << 8)
	return r, g, b, a
}

type NRGBA struct {
	R uint8
	G uint8
	B uint8
	A uint8
}

func (c NRGBA) RGBA() (uint32, uint32, uint32, uint32) {
	a := uint32(c.A)
	a = a | (a << 8)

	r := uint32(c.R)
	r = r | (r << 8)
	r = r * a / 0xffff

	g := uint32(c.G)
	g = g | (g << 8)
	g = g * a / 0xffff

	b := uint32(c.B)
	b = b | (b << 8)
	b = b * a / 0xffff

	return r, g, b, a
}

type Gray struct {
	Y uint8
}

func (c Gray) RGBA() (uint32, uint32, uint32, uint32) {
	y := uint32(c.Y)
	y = y | (y << 8)
	return y, y, y, 0xffff
}

type CMYK struct {
	C uint8
	M uint8
	Y uint8
	K uint8
}

func (c CMYK) RGBA() (uint32, uint32, uint32, uint32) {
	w := uint32(0xffff) - uint32(c.K)*0x101
	r := (uint32(0xffff) - uint32(c.C)*0x101) * w / 0xffff
	g := (uint32(0xffff) - uint32(c.M)*0x101) * w / 0xffff
	b := (uint32(0xffff) - uint32(c.Y)*0x101) * w / 0xffff
	return r, g, b, 0xffff
}

type Model interface {
	Convert(c Color) Color
}

type modelFunc struct {
	f func(c Color) Color
}

func (m *modelFunc) Convert(c Color) Color {
	return m.f(c)
}

func ModelFunc(f func(c Color) Color) Model {
	return &modelFunc{f: f}
}

func toRGBA(c Color) Color {
	r, g, b, a := c.RGBA()
	return Rgba{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: uint8(a >> 8)}
}

func toNRGBA(c Color) Color {
	r, g, b, a := c.RGBA()
	if a == 0 {
		return NRGBA{}
	}
	r = r * 0xffff / a
	g = g * 0xffff / a
	b = b * 0xffff / a
	return NRGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: uint8(a >> 8)}
}

func toGray(c Color) Color {
	r, g, b, _ := c.RGBA()
	y := (19595*r + 38470*g + 7471*b + 1<<15) >> 24
	return Gray{Y: uint8(y)}
}

func toCMYK(c Color) Color {
	r, g, b, _ := c.RGBA()
	rr := uint8(r >> 8)
	gg := uint8(g >> 8)
	bb := uint8(b >> 8)
	w := rr
	if gg > w {
		w = gg
	}
	if bb > w {
		w = bb
	}
	if w == 0 {
		return CMYK{0, 0, 0, 255}
	}
	c1 := (uint32(w) - uint32(rr)) * 255 / uint32(w)
	m1 := (uint32(w) - uint32(gg)) * 255 / uint32(w)
	y1 := (uint32(w) - uint32(bb)) * 255 / uint32(w)
	return CMYK{C: uint8(c1), M: uint8(m1), Y: uint8(y1), K: 255 - w}
}

var RGBAModel = ModelFunc(toRGBA)
var NRGBAModel = ModelFunc(toNRGBA)
var GrayModel = ModelFunc(toGray)
var CMYKModel = ModelFunc(toCMYK)

var Black = Gray{Y: 0}
var White = Gray{Y: 255}
var Opaque = Alpha{A: 255}
var Transparent = Alpha{A: 0}

type Alpha struct {
	A uint8
}

func (c Alpha) RGBA() (uint32, uint32, uint32, uint32) {
	a := uint32(c.A)
	a = a | (a << 8)
	return a, a, a, a
}
