package main

import (
	"fmt"
	"hash/crc32"
)

func main() {
	fmt.Println(crc32.ChecksumIEEE([]byte("")))
	fmt.Println(crc32.ChecksumIEEE([]byte("hello")))
	fmt.Println(crc32.ChecksumIEEE([]byte("The quick brown fox jumps over the lazy dog")))

	d := crc32.NewIEEE()
	d.Write([]byte("hello"))
	fmt.Println(d.Sum32())
	d.Reset()
	d.Write([]byte(""))
	fmt.Println(d.Sum32())
	fmt.Println(d.Size())
	fmt.Println(d.BlockSize())
}
