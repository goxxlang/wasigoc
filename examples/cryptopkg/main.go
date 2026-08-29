package main

import (
	"crypto"
	"fmt"
)

func main() {
	fmt.Println(crypto.SHA256.Size() == 32)
	fmt.Println(crypto.SHA256.String() == "SHA-256")
	fmt.Println(crypto.SHA256.Available())
	fmt.Println(crypto.MD5.Size() == 16)
	fmt.Println(crypto.SHA1.Size() == 20)
	fmt.Println(crypto.SHA512.Size() == 64)
	fmt.Println(crypto.SHA3_256.Size() == 32)
}
