package main

import (
	"fmt"
	"unsafe"
)

func main() {
	p := unsafe.PointerFromInt(100)
	q := unsafe.Add(p, 8)
	fmt.Println(unsafe.IntFromPointer(q) == 108)
	fmt.Println(unsafe.IntFromPointer(p) == 100)
}
