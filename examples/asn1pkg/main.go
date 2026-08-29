package main

import (
	"encoding/asn1"
	"fmt"
)

func main() {
	b, err := asn1.Marshal(int64(42))
	fmt.Println(err == nil)
	var n int64
	err = asn1.Unmarshal(b, &n)
	fmt.Println(err == nil)
	fmt.Println(n == 42)

	bb, err2 := asn1.Marshal(true)
	fmt.Println(err2 == nil)
	var flag bool
	err2 = asn1.Unmarshal(bb, &flag)
	fmt.Println(err2 == nil)
	fmt.Println(flag)

	raw, err3 := asn1.Marshal([]byte{1, 2, 3})
	fmt.Println(err3 == nil)
	var out []byte
	err3 = asn1.Unmarshal(raw, &out)
	fmt.Println(err3 == nil)
	fmt.Println(len(out) == 3)
	fmt.Println(out[0] == 1)
}
