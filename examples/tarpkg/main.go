package main

import (
	"archive/tar"
	"bytes"
	"fmt"
)

func main() {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)

	h1 := &tar.Header{Name: "hello.txt", Mode: 420, Size: int64(len("hello tar world")), Typeflag: '0'}
	tw.WriteHeader(h1)
	tw.Write([]byte("hello tar world"))

	h2 := &tar.Header{Name: "second.txt", Mode: 420, Size: int64(len("a second, shorter file")), Typeflag: '0'}
	tw.WriteHeader(h2)
	tw.Write([]byte("a second, shorter file"))

	tw.Close()

	// Round trip through our own Reader.
	tr := tar.NewReader(bytes.NewReader(buf.Bytes()))

	rh1, err1 := tr.Next()
	fmt.Println(err1 == nil)
	fmt.Println(rh1.Name == "hello.txt")
	fmt.Println(rh1.Size == int64(len("hello tar world")))
	fmt.Println(rh1.Typeflag == byte('0'))

	data1 := make([]byte, rh1.Size)
	n1, rerr1 := tr.Read(data1)
	fmt.Println(rerr1 == nil)
	fmt.Println(n1 == len(data1))
	fmt.Println(string(data1) == "hello tar world")

	rh2, err2 := tr.Next()
	fmt.Println(err2 == nil)
	fmt.Println(rh2.Name == "second.txt")

	data2 := make([]byte, rh2.Size)
	tr.Read(data2)
	fmt.Println(string(data2) == "a second, shorter file")

	// After the last entry, Next() reports EOF.
	_, err3 := tr.Next()
	fmt.Println(err3 != nil)

	// Total buffer length is a multiple of 512 (block-aligned).
	fmt.Println(len(buf.Bytes())%512 == 0)
}
