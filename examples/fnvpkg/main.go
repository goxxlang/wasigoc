package main

import (
	"fmt"
	"hash/fnv"
)

func main() {
	h32 := fnv.New32()
	h32.Write([]byte("hello"))
	fmt.Println(h32.Sum32())

	h32a := fnv.New32a()
	h32a.Write([]byte("hello"))
	fmt.Println(h32a.Sum32())

	h64 := fnv.New64()
	h64.Write([]byte("hello"))
	fmt.Println(h64.Sum64())

	h64a := fnv.New64a()
	h64a.Write([]byte("hello"))
	fmt.Println(h64a.Sum64())

	h32.Reset()
	fmt.Println(h32.Sum32())
	fmt.Println(h32.Size())
	fmt.Println(h32.BlockSize())
}
