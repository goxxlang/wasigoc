// Bounded image/color/palette: real Go's package has exactly two data
// tables, `WebSafe` (216 colors) and `Plan9` (256 colors, a specific
// quantization table with no simple closed-form generator). `WebSafe` is
// built here via the same three-nested-loop construction that DEFINES the
// web-safe palette in the first place (every R/G/B combination from the
// 6-step ramp {0,51,102,153,204,255}) -- not transcribed by hand, so it's
// provably the standard table by construction rather than by copying.
// `Plan9` is NOT implemented (todo): it has no such formula, only a fixed
// 256-entry table, and transcribing 256 RGB triples by hand is exactly the
// kind of error-prone busywork this project's other packages avoid (see
// e.g. hash algorithms' constant tables being cross-checked against an
// authoritative source rather than typed from memory).
package palette

import "image/color"

var WebSafe []color.Color

func init() {
	ramp := []byte{0, 51, 102, 153, 204, 255}
	ri := 0
	for ri < 6 {
		gi := 0
		for gi < 6 {
			bi := 0
			for bi < 6 {
				WebSafe = append(WebSafe, color.Rgba{R: ramp[ri], G: ramp[gi], B: ramp[bi], A: 255})
				bi = bi + 1
			}
			gi = gi + 1
		}
		ri = ri + 1
	}
}
