package main

import (
	"bytes"
	"encoding/pem"
	"fmt"
)

func bytesEq(a []byte, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func main() {
	orig := []byte("hello pem world, this is a longer message to wrap across multiple base64 lines for real testing")
	block := &pem.Block{Type: "TEST DATA", Headers: map[string]string{}, Bytes: orig}
	encoded := pem.EncodeToMemory(block)

	decoded, rest := pem.Decode(encoded)
	fmt.Println(decoded != nil)
	fmt.Println(decoded.Type == "TEST DATA")
	fmt.Println(bytesEq(decoded.Bytes, orig))
	fmt.Println(len(rest) == 0)

	// A hand-written PEM block with exactly one header.
	handwritten := "-----BEGIN MESSAGE-----\nProc-Type: 4,ENCRYPTED\n\naGVsbG8=\n-----END MESSAGE-----\n"
	d2, rest2 := pem.Decode([]byte(handwritten))
	fmt.Println(d2 != nil)
	fmt.Println(d2.Type == "MESSAGE")
	fmt.Println(d2.Headers["Proc-Type"] == "4,ENCRYPTED")
	fmt.Println(string(d2.Bytes) == "hello")
	fmt.Println(len(rest2) == 0)

	// Round trip through Encode into a bytes.Buffer.
	var buf bytes.Buffer
	pem.Encode(&buf, &pem.Block{Type: "X", Headers: map[string]string{}, Bytes: []byte("abc")})
	d3, _ := pem.Decode(buf.Bytes())
	fmt.Println(d3 != nil)
	fmt.Println(string(d3.Bytes) == "abc")

	// No PEM markers at all.
	d4, rest4 := pem.Decode([]byte("just some text, no pem here"))
	fmt.Println(d4 == nil)
	fmt.Println(len(rest4) == len("just some text, no pem here"))

	// Trailing content after the block is preserved in `rest`.
	withTrailing := handwritten + "extra stuff after"
	_, rest5 := pem.Decode([]byte(withTrailing))
	fmt.Println(string(rest5) == "extra stuff after")
}
