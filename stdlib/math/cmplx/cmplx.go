// math/cmplx over Go++ complex128 (1+2i, complex/real/imag builtins).
package cmplx

import "math"

func Abs(c complex128) float64 {
	re := real(c)
	im := imag(c)
	return math.Sqrt(re*re + im*im)
}

func Conj(c complex128) complex128 {
	return complex(real(c), -imag(c))
}

func Add(a complex128, b complex128) complex128 {
	return a + b
}

func Sub(a complex128, b complex128) complex128 {
	return a - b
}

func Mul(a complex128, b complex128) complex128 {
	return a * b
}

func Inv(c complex128) complex128 {
	return 1 / c
}

func Div(a complex128, b complex128) complex128 {
	return a / b
}
