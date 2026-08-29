package main

import (
	"bytes"
	"compress/flate"
	"fmt"
	"io"
)

func roundTrip(data []byte) (int, bool) {
	var buf bytes.Buffer
	w, _ := flate.NewWriter(&buf, flate.DefaultCompression)
	w.Write(data)
	w.Close()

	r := flate.NewReader(bytes.NewReader(buf.Bytes()))
	out, err := io.ReadAll(r)
	if err != nil {
		return 0, false
	}
	return buf.Len(), bytes.Equal(out, data)
}

func main() {
	_, ok1 := roundTrip([]byte(""))
	fmt.Println(ok1)

	_, ok2 := roundTrip([]byte("a"))
	fmt.Println(ok2)

	n3, ok3 := roundTrip([]byte("hello, hello, hello, hello world world world"))
	fmt.Println(ok3)
	fmt.Println(n3 > 0)

	repetitive := bytes.Repeat([]byte("AB"), 2000)
	n4, ok4 := roundTrip(repetitive)
	fmt.Println(ok4)
	fmt.Println(n4 < len(repetitive))

	text := bytes.Repeat([]byte("the quick brown fox jumps over the lazy dog. "), 50)
	n5, ok5 := roundTrip(text)
	fmt.Println(ok5)
	fmt.Println(n5 < len(text))

	single := []byte{42}
	_, ok6 := roundTrip(single)
	fmt.Println(ok6)
}
