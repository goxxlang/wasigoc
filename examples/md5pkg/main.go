package main

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
)

func main() {
	fmt.Println(hex.EncodeToString(md5.Sum([]byte(""))))
	fmt.Println(hex.EncodeToString(md5.Sum([]byte("abc"))))
	fmt.Println(hex.EncodeToString(md5.Sum([]byte("The quick brown fox jumps over the lazy dog"))))

	d := md5.New()
	d.Write([]byte("abc"))
	fmt.Println(hex.EncodeToString(d.Sum(nil)))

	d2 := md5.New()
	d2.Write([]byte("The quick brown "))
	d2.Write([]byte("fox jumps over the lazy dog"))
	fmt.Println(hex.EncodeToString(d2.Sum(nil)))

	d2.Reset()
	d2.Write([]byte("abc"))
	fmt.Println(hex.EncodeToString(d2.Sum(nil)))
	fmt.Println(d2.Size())
	fmt.Println(d2.BlockSize())
}
