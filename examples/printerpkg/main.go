package main

import (
	"fmt"
	"go/parser"
	"go/printer"
)

func main() {
	e, _ := parser.ParseExpr("1+2*3")
	fmt.Println(printer.Sprint(e))

	e2, _ := parser.ParseExpr("fmt.Println(a,b+1)")
	fmt.Println(printer.Sprint(e2))

	src := "package main\n\nfunc add(a int, b int) int {\nif a > b {\nreturn a\n}\nsum := a + b\nfor sum > 0 {\nsum--\n}\nreturn sum\n}\n"
	f, err := parser.ParseFile(src)
	fmt.Println(err == nil)
	fmt.Println(printer.Sprint(f))
}
