package main

import (
	"bytes"
	"fmt"
	"text/tabwriter"
)

func main() {
	var buf bytes.Buffer
	w := tabwriter.NewWriter(&buf, 1, 0, 1, ' ', 0)
	fmt.Fprint(w, "a\tb\tc\n")
	fmt.Fprint(w, "aaa\tbb\tc\n")
	fmt.Fprint(w, "aa\tbbb\tc\n")
	w.Flush()
	fmt.Println(buf.String() == "a   b   c\naaa bb  c\naa  bbb c\n")

	var buf2 bytes.Buffer
	w2 := tabwriter.NewWriter(&buf2, 1, 0, 1, ' ', tabwriter.AlignRight)
	fmt.Fprint(w2, "1\t22\n333\t4\n")
	w2.Flush()
	fmt.Println(buf2.String() == "   122\n 3334\n")

	// A larger padding and custom padchar.
	var buf3 bytes.Buffer
	w3 := tabwriter.NewWriter(&buf3, 0, 0, 2, byte('.'), 0)
	fmt.Fprint(w3, "x\ty\n")
	fmt.Fprint(w3, "xx\tyy\n")
	w3.Flush()
	fmt.Println(buf3.String() == "x...y\nxx..yy\n")
}
