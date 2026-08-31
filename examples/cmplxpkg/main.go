package main

import (
	"fmt"
	"math/cmplx"
)

func main() {
	a := 3 + 4i
	fmt.Println(cmplx.Abs(a) > 4.999 && cmplx.Abs(a) < 5.001)
	c := cmplx.Conj(a)
	fmt.Println(imag(c) == -4)
	s := cmplx.Add(a, 1+1i)
	fmt.Println(real(s) == 4)
	fmt.Println(imag(s) == 5)
	p := a * 1
	fmt.Println(real(p) == 3)
	fmt.Println(imag(p) == 4)
	d := a / a
	fmt.Println(real(d) > 0.999 && real(d) < 1.001)
	fmt.Println(imag(d) > -0.001 && imag(d) < 0.001)
	var f float32 = 3
	var g float32 = 4
	z := complex(f, g)
	fmt.Println(real(z) == 3)
}
