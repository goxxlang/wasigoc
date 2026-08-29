// Bounded math/cmplx: a Complex struct {Real, Imag float64} rather than
// the language's complex128 -- this compiler has no complex number
// type at all (see the language-gap audit). Callers write
// cmplx.New(re, im), not 1+2i. Abs/Add/Sub/Mul/Conj/Inv; no polar
// trig (Sin/Cos/Tan of a complex) and no Pow.
package cmplx

import "math"

type Complex struct {
	Real float64
	Imag float64
}

func New(r float64, i float64) Complex {
	return Complex{Real: r, Imag: i}
}

func Abs(c Complex) float64 {
	return math.Sqrt(c.Real*c.Real + c.Imag*c.Imag)
}

func Conj(c Complex) Complex {
	return Complex{Real: c.Real, Imag: -c.Imag}
}

func Add(a Complex, b Complex) Complex {
	return Complex{Real: a.Real + b.Real, Imag: a.Imag + b.Imag}
}

func Sub(a Complex, b Complex) Complex {
	return Complex{Real: a.Real - b.Real, Imag: a.Imag - b.Imag}
}

func Mul(a Complex, b Complex) Complex {
	return Complex{
		Real: a.Real*b.Real - a.Imag*b.Imag,
		Imag: a.Real*b.Imag + a.Imag*b.Real,
	}
}

func Inv(c Complex) Complex {
	d := c.Real*c.Real + c.Imag*c.Imag
	return Complex{Real: c.Real / d, Imag: -c.Imag / d}
}

func Div(a Complex, b Complex) Complex {
	return Mul(a, Inv(b))
}
