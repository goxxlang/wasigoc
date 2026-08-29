package main

import (
	"fmt"
	"go/doc"
	"go/parser"
)

const src = `// Package widget does widget things.
package widget

// Frob does a frob.
func Frob() {}

// Count is documented.
var Count = 0

const Limit = 10

// separated by a blank line above, so NOT this decl's doc.

func Nope() {}
`

func main() {
	file, comments, err := parser.ParseFileWithComments(src)
	fmt.Println(err == nil)

	pkg := doc.New(file, comments, src)
	fmt.Println(pkg.Name == "widget")
	fmt.Println(pkg.Doc == "Package widget does widget things.\n")
	fmt.Println(len(pkg.Funcs) == 2)
	fmt.Println(pkg.Funcs[0].Name == "Frob")
	fmt.Println(pkg.Funcs[0].Doc == "Frob does a frob.\n")
	fmt.Println(pkg.Funcs[1].Name == "Nope")
	fmt.Println(pkg.Funcs[1].Doc == "")
	fmt.Println(len(pkg.Vars) == 1)
	fmt.Println(pkg.Vars[0].Name == "Count")
	fmt.Println(pkg.Vars[0].Doc == "Count is documented.\n")
	fmt.Println(len(pkg.Consts) == 1)
	fmt.Println(pkg.Consts[0].Name == "Limit")
	fmt.Println(pkg.Consts[0].Doc == "")
}
