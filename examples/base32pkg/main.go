package main

import (
	"encoding/base32"
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
	// RFC 4648 section 10 test vectors.
	fmt.Println(base32.StdEncoding.EncodeToString([]byte("")) == "")
	fmt.Println(base32.StdEncoding.EncodeToString([]byte("f")) == "MY======")
	fmt.Println(base32.StdEncoding.EncodeToString([]byte("fo")) == "MZXQ====")
	fmt.Println(base32.StdEncoding.EncodeToString([]byte("foo")) == "MZXW6===")
	fmt.Println(base32.StdEncoding.EncodeToString([]byte("foob")) == "MZXW6YQ=")
	fmt.Println(base32.StdEncoding.EncodeToString([]byte("fooba")) == "MZXW6YTB")
	fmt.Println(base32.StdEncoding.EncodeToString([]byte("foobar")) == "MZXW6YTBOI======")

	d1, err1 := base32.StdEncoding.DecodeString("MZXW6YTBOI======")
	fmt.Println(err1 == nil)
	fmt.Println(bytesEq(d1, []byte("foobar")))

	d2, err2 := base32.StdEncoding.DecodeString("MZXW6YQ=")
	fmt.Println(err2 == nil)
	fmt.Println(bytesEq(d2, []byte("foob")))

	_, err3 := base32.StdEncoding.DecodeString("!!!!")
	fmt.Println(err3 != nil)

	// HexEncoding, same test vectors, different alphabet.
	fmt.Println(base32.HexEncoding.EncodeToString([]byte("foobar")) == "CPNMUOJ1E8======")
	d3, err4 := base32.HexEncoding.DecodeString("CPNMUOJ1E8======")
	fmt.Println(err4 == nil)
	fmt.Println(bytesEq(d3, []byte("foobar")))

	// Round trip a longer, non-ASCII-boundary-friendly value.
	orig := []byte("The quick brown fox jumps over the lazy dog")
	enc := base32.StdEncoding.EncodeToString(orig)
	dec, err5 := base32.StdEncoding.DecodeString(enc)
	fmt.Println(err5 == nil)
	fmt.Println(bytesEq(dec, orig))
}
