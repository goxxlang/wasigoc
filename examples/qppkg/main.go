package main

import (
	"bytes"
	"fmt"
	"io"
	"mime/quotedprintable"
)

func main() {
	var buf bytes.Buffer
	w := quotedprintable.NewWriter(&buf)
	w.Write([]byte("Hi = 100% cool\t\ntrailing \n"))
	w.Close()
	fmt.Println(buf.String())

	r := quotedprintable.NewReader(bytes.NewReader(buf.Bytes()))
	out, _ := io.ReadAll(r)
	fmt.Println(string(out))

	r2 := quotedprintable.NewReader(bytes.NewReader([]byte("If you believe that truth=3Dbeauty, then surely=20\r\nmathematics is the most beautiful branch of philosophy.")))
	out2, _ := io.ReadAll(r2)
	fmt.Println(string(out2))
}
