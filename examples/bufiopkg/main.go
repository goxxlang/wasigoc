package main

import (
	"bufio"
	"bytes"
	"fmt"
	"strings"
)

func main() {
	sc := bufio.NewScanner(strings.NewReader("line one\nline two\r\nline three"))
	n := 0
	for sc.Scan() {
		n++
		fmt.Println(n, sc.Text())
	}
	fmt.Println(sc.Err())

	var buf bytes.Buffer
	w := bufio.NewWriter(&buf)
	w.WriteString("hello ")
	w.WriteString("world")
	w.Flush()
	fmt.Println(buf.String())
}
