// Exercises bytes.Reader/strings.Reader against io.ReadAll/io.Copy, and the
// byte/int8 fmt-formatting fix (Go prints these numerically, never as a
// C++ char).
package main

import (
	"bytes"
	"fmt"
	"io"
	"strings"
)

func main() {
	r := bytes.NewReader([]byte("hello bytes"))
	out, _ := io.ReadAll(r)
	fmt.Println(string(out))

	sr := strings.NewReader("hello strings")
	out2, _ := io.ReadAll(sr)
	fmt.Println(string(out2))

	var buf bytes.Buffer
	n, err := io.Copy(&buf, strings.NewReader("copied"))
	fmt.Println(n, err, buf.String())

	b, _ := bytes.NewReader([]byte("X")).ReadByte()
	fmt.Println(b)

	var i8 int8 = 65
	fmt.Println(i8)
	fmt.Printf("%d %c\n", b, b)
}
