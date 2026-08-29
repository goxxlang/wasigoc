package main

import (
	"bytes"
	"fmt"
	"mime/multipart"
)

func main() {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	w.SetBoundary("BOUNDARY123")
	w.WriteField("name", "wasigo")
	w.WriteFile("upload", "hello.txt", []byte("hello world"))
	w.Close()

	fmt.Println(w.Boundary())

	r := multipart.NewReader(bytes.NewReader(buf.Bytes()), "BOUNDARY123")

	p1, err1 := r.NextPart()
	fmt.Println(err1 == nil)
	fmt.Println(p1.FormName())
	fmt.Println(p1.FileName())
	b1 := make([]byte, 32)
	n1, _ := p1.Read(b1)
	fmt.Println(string(b1[:n1]))

	p2, err2 := r.NextPart()
	fmt.Println(err2 == nil)
	fmt.Println(p2.FormName())
	fmt.Println(p2.FileName())
	b2 := make([]byte, 32)
	n2, _ := p2.Read(b2)
	fmt.Println(string(b2[:n2]))

	_, err3 := r.NextPart()
	fmt.Println(err3 == nil)
}
