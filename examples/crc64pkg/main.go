package main

import (
	"fmt"
	"hash/crc64"
)

func main() {
	isoTab := crc64.MakeTable(crc64.ISO)
	ecmaTab := crc64.MakeTable(crc64.ECMA)

	fmt.Println(crc64.Checksum([]byte("123456789"), isoTab) == 0xb90956c775a41001)
	fmt.Println(crc64.Checksum([]byte("123456789"), ecmaTab) == 0x995dc9bbdf1939fa)

	// Streaming Digest matches the one-shot Checksum, split across writes.
	d := crc64.New(ecmaTab)
	d.Write([]byte("12345"))
	d.Write([]byte("6789"))
	fmt.Println(d.Sum64() == 0x995dc9bbdf1939fa)

	d.Reset()
	d.Write([]byte("123456789"))
	fmt.Println(d.Sum64() == 0x995dc9bbdf1939fa)

	fmt.Println(d.Size())
	fmt.Println(d.BlockSize())

	sum := d.Sum(nil)
	fmt.Println(len(sum))
	fmt.Println(sum[0] == byte(0x99))
	fmt.Println(sum[7] == byte(0xfa))
}
