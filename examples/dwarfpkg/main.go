package main

import (
	"debug/dwarf"
	"debug/gosym"
	"fmt"
)

func main() {
	hdr := []byte{7, 0, 0, 0, 4, 0, 0, 0, 0, 0, 8}
	d, err := dwarf.New(hdr)
	fmt.Println(err == nil)
	fmt.Println(d.Version == 4)
	fmt.Println(d.AddrSize == 8)
	fmt.Println(d.UnitLength == 7)
	_, err2 := dwarf.New([]byte{1, 2})
	fmt.Println(err2 != nil)

	pc := []byte{0xfb, 0xff, 0xff, 0xff, 0, 0, 1, 8}
	t, err3 := gosym.NewTable(pc)
	fmt.Println(err3 == nil)
	fmt.Println(t.Magic == gosym.Magic12)
	fmt.Println(t.Quantum == 1)
	fmt.Println(t.Ptrsize == 8)
}
