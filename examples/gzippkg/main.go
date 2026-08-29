package main

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
)

func roundTrip(data []byte) (int, bool) {
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	w.Write(data)
	w.Close()

	r, err := gzip.NewReader(bytes.NewReader(buf.Bytes()))
	if err != nil {
		return 0, false
	}
	out, rerr := io.ReadAll(r)
	if rerr != nil {
		return 0, false
	}
	return buf.Len(), bytes.Equal(out, data)
}

func main() {
	_, ok1 := roundTrip([]byte("hello gzip world hello gzip world hello gzip world"))
	fmt.Println(ok1)

	_, ok2 := roundTrip([]byte(""))
	fmt.Println(ok2)

	_, badErr := gzip.NewReader(bytes.NewReader([]byte{1, 2, 3}))
	fmt.Println(badErr != nil)

	text := bytes.Repeat([]byte("gzip round trip test data "), 40)
	n4, ok4 := roundTrip(text)
	fmt.Println(ok4)
	fmt.Println(n4 < len(text))
}
