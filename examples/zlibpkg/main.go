package main

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"io"
)

func roundTrip(data []byte) (int, bool) {
	var buf bytes.Buffer
	w := zlib.NewWriter(&buf)
	w.Write(data)
	w.Close()

	r, err := zlib.NewReader(bytes.NewReader(buf.Bytes()))
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
	_, ok1 := roundTrip([]byte("hello zlib world hello zlib world hello zlib world"))
	fmt.Println(ok1)

	_, ok2 := roundTrip([]byte(""))
	fmt.Println(ok2)

	var buf bytes.Buffer
	w := zlib.NewWriter(&buf)
	w.Write([]byte("corrupt me"))
	w.Close()
	b := buf.Bytes()
	b[len(b)-1] = b[len(b)-1] ^ 255
	r, _ := zlib.NewReader(bytes.NewReader(b))
	_, cerr := io.ReadAll(r)
	fmt.Println(cerr != nil)

	_, badErr := zlib.NewReader(bytes.NewReader([]byte{1, 2, 3}))
	fmt.Println(badErr != nil)
}
