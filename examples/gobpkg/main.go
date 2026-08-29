package main

import (
	"encoding/gob"
	"fmt"
)

func main() {
	e := gob.NewEncoder()
	e.Encode("hi")
	e.Encode(int64(7))
	e.Encode([]byte{9})
	d := gob.NewDecoder(e.Bytes())
	var s string
	d.Decode(&s)
	fmt.Println(s == "hi")
	var n int64
	d.Decode(&n)
	fmt.Println(n == 7)
	var b []byte
	d.Decode(&b)
	fmt.Println(len(b) == 1)
	fmt.Println(b[0] == 9)
}
