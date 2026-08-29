package main

import (
	"bytes"
	"compress/lzw"
	"fmt"
	"io"
)

func roundTrip(data []byte, order lzw.Order, litWidth int) (string, bool) {
	var buf bytes.Buffer
	w := lzw.NewWriter(&buf, order, litWidth)
	w.Write(data)
	w.Close()

	r := lzw.NewReader(bytes.NewReader(buf.Bytes()), order, litWidth)
	out, err := io.ReadAll(r)
	if err != nil {
		return "", false
	}
	return fmt.Sprintf("%d->%d", len(data), buf.Len()), bytes.Equal(out, data)
}

func main() {
	text := []byte("TOBEORNOTTOBEORTOBEORNOT the quick brown fox jumps over the lazy dog the quick brown fox")

	s1, ok1 := roundTrip(text, lzw.LSB, 8)
	fmt.Println(ok1)
	fmt.Println(s1 != "")

	s2, ok2 := roundTrip(text, lzw.MSB, 8)
	fmt.Println(ok2)
	fmt.Println(s2 != "")

	repetitive := bytes.Repeat([]byte("ABAB"), 2000)
	_, ok3 := roundTrip(repetitive, lzw.LSB, 8)
	fmt.Println(ok3)

	_, ok4 := roundTrip(repetitive, lzw.MSB, 8)
	fmt.Println(ok4)

	empty := []byte{}
	_, ok5 := roundTrip(empty, lzw.LSB, 8)
	fmt.Println(ok5)

	single := []byte{42}
	_, ok6 := roundTrip(single, lzw.MSB, 8)
	fmt.Println(ok6)

	smallLit := bytes.Repeat([]byte{0, 1, 2, 3}, 500)
	_, ok7 := roundTrip(smallLit, lzw.LSB, 4)
	fmt.Println(ok7)
}
