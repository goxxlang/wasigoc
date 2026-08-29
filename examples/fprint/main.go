package main

import (
	"bytes"
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stdout, "hi", 1, true)
	fmt.Fprint(os.Stdout, "a", "b")
	fmt.Fprintln(os.Stdout)
	fmt.Fprintf(os.Stdout, "%d-%s-%v\n", 5, "x", false)
	fmt.Fprintln(os.Stderr, "err line")

	var b bytes.Buffer
	fmt.Fprintln(&b, "buffered", 42)
	fmt.Fprintf(&b, "%d", 7)
	fmt.Println(b.String())
}
