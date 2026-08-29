// Multi-result interface method: io.Writer.Write returns (int, error).
package main

import (
	"fmt"
	"io"
)

type Buf struct {
	N int
}

func (b *Buf) Write(p []byte) (n int, err error) {
	b.N = b.N + len(p)
	return len(p), nil
}

func add(w io.Writer, s string) {
	n, err := w.Write([]byte(s))
	if err != nil {
		return
	}
	fmt.Println(n)
}

func main() {
	var b Buf
	add(&b, "hi")
	fmt.Println(b.N)
}
