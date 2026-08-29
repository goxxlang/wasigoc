package main

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
)

func main() {
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)

	f1, cerr1 := w.Create("a.txt")
	fmt.Println(cerr1 == nil)
	f1.Write([]byte("hello zip world hello zip world hello zip world"))

	f2, cerr2 := w.Create("b.txt")
	fmt.Println(cerr2 == nil)
	f2.Write([]byte("second entry"))

	fmt.Println(w.Close() == nil)

	r, rerr := zip.NewReader(buf.Bytes())
	fmt.Println(rerr == nil)
	fmt.Println(len(r.File) == 2)
	fmt.Println(r.File[0].Name == "a.txt")
	fmt.Println(r.File[1].Name == "b.txt")

	rc1, oerr1 := r.File[0].Open()
	fmt.Println(oerr1 == nil)
	data1, derr1 := io.ReadAll(rc1)
	fmt.Println(derr1 == nil)
	fmt.Println(string(data1) == "hello zip world hello zip world hello zip world")

	rc2, oerr2 := r.File[1].Open()
	fmt.Println(oerr2 == nil)
	data2, _ := io.ReadAll(rc2)
	fmt.Println(string(data2) == "second entry")

	_, badErr := zip.NewReader([]byte("not a zip file"))
	fmt.Println(badErr != nil)
}
