package main

import (
	"fmt"
	"math/cmplx"
)

func main() {
	a := cmplx.New(3, 4)
	fmt.Println(cmplx.Abs(a) > 4.999 && cmplx.Abs(a) < 5.001)
	c := cmplx.Conj(a)
	fmt.Println(c.Imag == -4)
	s := cmplx.Add(a, cmplx.New(1, 1))
	fmt.Println(s.Real == 4)
	fmt.Println(s.Imag == 5)
	p := cmplx.Mul(a, cmplx.New(1, 0))
	fmt.Println(p.Real == 3)
	fmt.Println(p.Imag == 4)
	d := cmplx.Div(a, a)
	fmt.Println(d.Real > 0.999 && d.Real < 1.001)
	fmt.Println(d.Imag > -0.001 && d.Imag < 0.001)
}
