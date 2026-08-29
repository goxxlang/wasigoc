package main

import (
	"fmt"
	"hash"
	"hash/adler32"
	"hash/crc32"
	"hash/crc64"
	"crypto/md5"
)

func sumHash(h hash.Hash, data []byte) []byte {
	h.Write(data)
	return h.Sum(nil)
}

func sumHash32(h hash.Hash32, data []byte) uint32 {
	h.Write(data)
	return h.Sum32()
}

func sumHash64(h hash.Hash64, data []byte) uint64 {
	h.Write(data)
	return h.Sum64()
}

func main() {
	data := []byte("wasigo")

	m := md5.New()
	sum := sumHash(m, data)
	fmt.Println(len(sum))
	fmt.Println(m.Size())

	a := adler32.New()
	fmt.Println(sumHash32(a, data))
	fmt.Println(a.BlockSize())

	c := crc32.NewIEEE()
	fmt.Println(sumHash32(c, data))

	c64 := crc64.New(crc64.MakeTable(crc64.ISO))
	fmt.Println(sumHash64(c64, data))

	var hh hash.Hash = md5.New()
	hh.Write(data)
	fmt.Println(len(hh.Sum(nil)))
}
