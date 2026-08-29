package main

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
)

func main() {
	fmt.Println(hex.EncodeToString(sha1.Sum([]byte(""))))
	fmt.Println(hex.EncodeToString(sha1.Sum([]byte("abc"))))
	fmt.Println(hex.EncodeToString(sha1.Sum([]byte("The quick brown fox jumps over the lazy dog"))))

	d := sha1.New()
	d.Write([]byte("abc"))
	fmt.Println(hex.EncodeToString(d.Sum(nil)))

	d2 := sha1.New()
	d2.Write([]byte("The quick brown "))
	d2.Write([]byte("fox jumps over the lazy dog"))
	fmt.Println(hex.EncodeToString(d2.Sum(nil)))

	d2.Reset()
	d2.Write([]byte("abc"))
	fmt.Println(hex.EncodeToString(d2.Sum(nil)))
	fmt.Println(d2.Size())
	fmt.Println(d2.BlockSize())
}
