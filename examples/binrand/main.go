package main

import (
	"encoding/binary"
	"fmt"
	"math/rand"
)

func main() {
	buf := make([]byte, 8)
	binary.LittleEndian.PutUint32(buf, 305419896)
	fmt.Println(binary.LittleEndian.Uint32(buf))
	binary.BigEndian.PutUint32(buf, 305419896)
	fmt.Println(binary.BigEndian.Uint32(buf))

	buf64 := make([]byte, 8)
	binary.LittleEndian.PutUint64(buf64, 12345678901234)
	fmt.Println(binary.LittleEndian.Uint64(buf64))

	out := binary.BigEndian.AppendUint16([]byte{}, 258)
	fmt.Println(out[0], out[1])

	r := rand.New(rand.NewSource(42))
	fmt.Println(r.Intn(100))
	fmt.Println(r.Intn(100))
	fmt.Println(r.Intn(100))

	r2 := rand.New(rand.NewSource(42))
	fmt.Println(r2.Intn(100))

	f := r.Float64()
	fmt.Println(f >= 0 && f < 1)

	p := r.Perm(5)
	fmt.Println(len(p))
	seen := make(map[int]bool)
	dup := false
	for _, v := range p {
		if seen[v] {
			dup = true
		}
		seen[v] = true
	}
	fmt.Println(dup)
}
