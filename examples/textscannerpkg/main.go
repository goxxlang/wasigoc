package main

import (
	"fmt"
	"strings"
	"text/scanner"
)

func main() {
	src := "foo 123 3.14 \"hi there\" // a comment\nbar_2 `raw text` 'x' + /* block\ncomment */ baz"
	var s scanner.Scanner
	s.Init(strings.NewReader(src))

	for {
		tok := s.Scan()
		if tok == scanner.EOF {
			break
		}
		switch tok {
		case scanner.Ident:
			fmt.Println("ident", s.TokenText())
		case scanner.Int:
			fmt.Println("int", s.TokenText())
		case scanner.Float:
			fmt.Println("float", s.TokenText())
		case scanner.String:
			fmt.Println("string", s.TokenText())
		case scanner.RawString:
			fmt.Println("rawstring", s.TokenText())
		case scanner.Char:
			fmt.Println("char", s.TokenText())
		default:
			fmt.Println("rune", s.TokenText())
		}
	}
}
