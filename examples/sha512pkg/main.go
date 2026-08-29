package main

import (
	"crypto/sha512"
	"encoding/hex"
	"fmt"
)

func main() {
	fmt.Println(hex.EncodeToString(sha512.Sum([]byte(""))) ==
		"cf83e1357eefb8bdf1542850d66d8007d620e4050b5715dc83f4a921d36ce9ce47d0d13c5d85f2b0ff8318d2877eec2f63b931bd47417a81a538327af927da3e")
	fmt.Println(hex.EncodeToString(sha512.Sum([]byte("abc"))) ==
		"ddaf35a193617abacc417349ae20413112e6fa4e89a97ea20a9eeee64b55d39a2192992a274fc1a836ba3c23a3feebbd454d4423643ce80e2a9ac94fa54ca49f")
	fmt.Println(hex.EncodeToString(sha512.Sum([]byte("The quick brown fox jumps over the lazy dog"))) ==
		"07e547d9586f6a73f73fbac0435ed76951218fb7d0c8d788a309d785436bbb642e93a252a954f23912547d1e8a3b5ed6e1bfd7097821233fa0538f3db854fee6")

	// Streaming Digest, split across two Write calls, matches the
	// one-shot result.
	d := sha512.New()
	d.Write([]byte("The quick brown "))
	d.Write([]byte("fox jumps over the lazy dog"))
	fmt.Println(hex.EncodeToString(d.Sum(nil)) ==
		"07e547d9586f6a73f73fbac0435ed76951218fb7d0c8d788a309d785436bbb642e93a252a954f23912547d1e8a3b5ed6e1bfd7097821233fa0538f3db854fee6")

	d.Reset()
	d.Write([]byte("abc"))
	fmt.Println(hex.EncodeToString(d.Sum(nil)) ==
		"ddaf35a193617abacc417349ae20413112e6fa4e89a97ea20a9eeee64b55d39a2192992a274fc1a836ba3c23a3feebbd454d4423643ce80e2a9ac94fa54ca49f")

	fmt.Println(d.Size())
	fmt.Println(d.BlockSize())
}
