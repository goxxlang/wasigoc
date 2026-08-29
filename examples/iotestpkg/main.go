package main

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"testing/iotest"
)

func main() {
	r1 := iotest.OneByteReader(strings.NewReader("abc"))
	buf := make([]byte, 10)
	n1, _ := r1.Read(buf)
	fmt.Println(n1 == 1)

	r2 := iotest.HalfReader(strings.NewReader("abcdefgh"))
	buf2 := make([]byte, 8)
	n2, _ := r2.Read(buf2)
	fmt.Println(n2 == 4)

	r3 := iotest.ErrReader(errors.New("boom"))
	_, err3 := r3.Read(buf)
	fmt.Println(err3.Error() == "boom")

	r4 := iotest.DataErrReader(strings.NewReader("xyz"))
	out, ferr := io.ReadAll(r4)
	fmt.Println(string(out) == "xyz")
	fmt.Println(ferr == nil)

	buf5 := make([]byte, 1)
	r5 := iotest.DataErrReader(strings.NewReader("Q"))
	n5, err5 := r5.Read(buf5)
	fmt.Println(n5 == 1)
	fmt.Println(err5 == io.EOF)
}
