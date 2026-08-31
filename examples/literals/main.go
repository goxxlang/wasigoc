// Integer bases, leading-dot floats, Go string/rune escapes, uintptr.
package main

import "fmt"

func main() {
	fmt.Println(0xFF)
	fmt.Println(0o10)
	fmt.Println(0b1010)
	fmt.Println(010)
	fmt.Println(.5)
	fmt.Println("\u0041")
	fmt.Println("\U00000041")
	fmt.Println(int('\a'))
	fmt.Println(int('\u03A9'))
	fmt.Println(int('\n'))
	var p uintptr = uintptr(7)
	fmt.Println(int(p))
	fmt.Println(len("\U0001F600"))
	fmt.Println(int(0x1p3))
	fmt.Println(0x1.fp+3)
	fmt.Println(int(1.))
	fmt.Println(int(1.e2))
}
