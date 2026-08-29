package main

import (
	"encoding/ascii85"
	"fmt"
)

func main() {
	src := []byte("Man is distinguished, not only by his reason")
	dst := make([]byte, ascii85.MaxEncodedLen(len(src)))
	n := ascii85.Encode(dst, src)
	dst = dst[:n]
	fmt.Println(string(dst))

	back := make([]byte, len(src)+8)
	ndst, nsrc, err := ascii85.Decode(back, dst, true)
	fmt.Println(string(back[:ndst]))
	fmt.Println(nsrc == len(dst))
	fmt.Println(err == nil)

	zsrc := []byte{0, 0, 0, 0, 104, 101, 108, 108, 111}
	zdst := make([]byte, ascii85.MaxEncodedLen(len(zsrc)))
	zn := ascii85.Encode(zdst, zsrc)
	fmt.Println(string(zdst[:zn]))
}
