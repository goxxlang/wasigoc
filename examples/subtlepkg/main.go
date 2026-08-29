package main

import (
	"crypto/subtle"
	"fmt"
)

func main() {
	fmt.Println(subtle.ConstantTimeCompare([]byte("hello"), []byte("hello")) == 1)
	fmt.Println(subtle.ConstantTimeCompare([]byte("hello"), []byte("world")) == 0)
	fmt.Println(subtle.ConstantTimeCompare([]byte("hi"), []byte("hello")) == 0)

	fmt.Println(subtle.ConstantTimeByteEq(byte(5), byte(5)) == 1)
	fmt.Println(subtle.ConstantTimeByteEq(byte(5), byte(6)) == 0)

	fmt.Println(subtle.ConstantTimeEq(int32(42), int32(42)) == 1)
	fmt.Println(subtle.ConstantTimeEq(int32(42), int32(-42)) == 0)

	fmt.Println(subtle.ConstantTimeSelect(1, 10, 20) == 10)
	fmt.Println(subtle.ConstantTimeSelect(0, 10, 20) == 20)

	fmt.Println(subtle.ConstantTimeLessOrEq(3, 5) == 1)
	fmt.Println(subtle.ConstantTimeLessOrEq(5, 5) == 1)
	fmt.Println(subtle.ConstantTimeLessOrEq(6, 5) == 0)

	x := []byte{1, 2, 3}
	y := []byte{9, 9, 9}
	subtle.ConstantTimeCopy(1, x, y)
	fmt.Println(x[0] == 9 && x[1] == 9 && x[2] == 9)

	x2 := []byte{1, 2, 3}
	subtle.ConstantTimeCopy(0, x2, y)
	fmt.Println(x2[0] == 1 && x2[1] == 2 && x2[2] == 3)
}
