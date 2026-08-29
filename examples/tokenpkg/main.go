package main

import (
	"fmt"
	"go/token"
)

func main() {
	fmt.Println(token.TokenString(token.IDENT))
	fmt.Println(token.TokenString(token.ADD))
	fmt.Println(token.TokenString(token.FUNC))
	fmt.Println(token.TokenString(token.ARROW))

	fmt.Println(token.Lookup("func") == token.FUNC)
	fmt.Println(token.Lookup("hello") == token.IDENT)
	fmt.Println(token.Lookup("return") == token.RETURN)

	fmt.Println(token.IsKeyword(token.FUNC))
	fmt.Println(token.IsKeyword(token.IDENT))
	fmt.Println(token.IsOperator(token.ADD))
	fmt.Println(token.IsLiteral(token.STRING))

	fmt.Println(token.PosIsValid(token.NoPos))
	fmt.Println(token.PosIsValid(token.Pos(5)))
}
