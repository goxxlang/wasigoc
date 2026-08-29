package main

import (
	"fmt"
	"go/format"
)

func main() {
	src := "package main\n\nfunc add(a int,b int) int {\nif a>b{\nreturn a\n}\nreturn b\n}\n"
	out, err := format.Source(src)
	fmt.Println(err == nil)
	fmt.Println(out)

	_, err2 := format.Source("package main\n\nfunc bad(\n")
	fmt.Println(err2 != nil)
}
