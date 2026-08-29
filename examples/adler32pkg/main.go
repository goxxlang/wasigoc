package main

import (
	"fmt"
	"hash/adler32"
)

func main() {
	fmt.Println(adler32.Checksum([]byte("")))
	fmt.Println(adler32.Checksum([]byte("Wikipedia")))
	fmt.Println(adler32.Checksum([]byte("hello")))

	d := adler32.New()
	d.Write([]byte("Wikipedia"))
	fmt.Println(d.Sum32())
	fmt.Println(d.Size())
	fmt.Println(d.BlockSize())
}
