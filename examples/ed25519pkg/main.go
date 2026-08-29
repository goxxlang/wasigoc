package main

import (
	"crypto/ed25519"
	"fmt"
)

func main() {
	fmt.Println(ed25519.PublicKeySize == 32)
	fmt.Println(ed25519.PrivateKeySize == 32)
	fmt.Println(ed25519.SignatureSize == 64)
	fmt.Println(ed25519.Verify(nil, nil, nil) == false)
	short := make([]byte, 32)
	fmt.Println(ed25519.Verify(short, []byte("x"), short) == false)
}
