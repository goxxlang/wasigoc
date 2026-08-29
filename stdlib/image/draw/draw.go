// Bounded image/draw: `Draw` only, no `Drawer`/`FloydSteinberg`/`Quantizer`.
// `dst` is a concrete `*image.RGBA`, not real Go's `draw.Image` interface
// (`image.Image` + `Set`) -- this project's `image` package has exactly
// one concrete pixel-buffer type anyway, and every real-world `dst` is one
// in practice too. `src` stays the generic `image.Image` interface, so any
// caller-implemented image works as a source, same "read via interface"
// precedent as io/fs's `FS`. `Over` does real per-pixel alpha compositing
// on ALREADY-premultiplied values -- matching this project's `color.Rgba.
// RGBA()`, which (like real Go's own `color.RGBA`) returns its fields
// widened to 16 bits WITHOUT multiplying by alpha, because the type's own
// contract is that R/G/B are stored pre-multiplied by the caller already.
package draw

import (
	"image"
	"image/color"
)

type Op int

const Over Op = 0
const Src Op = 1

func Draw(dst *image.RGBA, r image.Rectangle, src image.Image, sp image.Point, op Op) {
	dr := r.Intersect(dst.Bounds())
	width := dr.Dx()
	height := dr.Dy()
	if width <= 0 || height <= 0 {
		return
	}
	y := 0
	for y < height {
		x := 0
		for x < width {
			sc := src.At(sp.X+x, sp.Y+y)
			dx := dr.Min.X + x
			dy := dr.Min.Y + y
			if op == Src {
				dst.Set(dx, dy, sc)
			} else {
				sr, sg, sb, sa := sc.RGBA()
				if sa == 65535 {
					dst.Set(dx, dy, sc)
				} else if sa != 0 {
					dc := dst.At(dx, dy)
					dr2, dg2, db2, da2 := dc.RGBA()
					inv := uint32(65535) - sa
					outR := sr + (dr2*inv)/65535
					outG := sg + (dg2*inv)/65535
					outB := sb + (db2*inv)/65535
					outA := sa + (da2*inv)/65535
					dst.Set(dx, dy, color.Rgba{R: uint8(outR >> 8), G: uint8(outG >> 8), B: uint8(outB >> 8), A: uint8(outA >> 8)})
				}
			}
			x = x + 1
		}
		y = y + 1
	}
}
