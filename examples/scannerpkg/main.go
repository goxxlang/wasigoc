package main

import (
	"fmt"
	"go/scanner"
	"go/token"
)

func dump(src string) {
	s := scanner.New(src)
	for {
		_, tok, lit := s.Scan()
		if lit != "" {
			fmt.Println(token.TokenString(tok) + " " + lit)
		} else {
			fmt.Println(token.TokenString(tok))
		}
		if tok == token.EOF {
			break
		}
	}
}

func main() {
	dump("x := 1 + 2")

	fmt.Println("---")

	dump("func add(a, b int) int {\n\treturn a + b\n}\n")

	fmt.Println("---")

	dump("x := 1 // comment\ny := 2")

	fmt.Println("---")

	dump("x := 1 /* block\ncomment */\ny := 2")

	fmt.Println("---")

	dump("0xFF 0o17 0b1010 3.14 \"hi\" `raw` 'c'")
}
