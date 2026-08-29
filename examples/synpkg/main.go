package main

import (
	"fmt"
	"regexp/syntax"
)

func main() {
	re1, err1 := syntax.Parse("abc", syntax.Perl)
	fmt.Println(err1 == nil)
	fmt.Println(re1.Op == syntax.OpConcat)
	fmt.Println(len(re1.Sub))
	fmt.Println(string(re1.Sub[0].Rune[0]), string(re1.Sub[1].Rune[0]), string(re1.Sub[2].Rune[0]))

	re2, _ := syntax.Parse("a*", syntax.Perl)
	fmt.Println(re2.Op == syntax.OpStar)
	fmt.Println(re2.Sub[0].Op == syntax.OpLiteral)

	re3, _ := syntax.Parse("a|b", syntax.Perl)
	fmt.Println(re3.Op == syntax.OpAlternate)
	fmt.Println(len(re3.Sub))

	re4, _ := syntax.Parse("[a-c]", syntax.Perl)
	fmt.Println(re4.Op == syntax.OpCharClass)
	fmt.Println(re4.Rune[0], re4.Rune[1])

	re5, _ := syntax.Parse("[^a-c]", syntax.Perl)
	fmt.Println(re5.Rune[0], re5.Rune[1])
	fmt.Println(len(re5.Rune))

	re6, _ := syntax.Parse("(ab)", syntax.Perl)
	fmt.Println(re6.Op == syntax.OpCapture)
	fmt.Println(re6.Cap)

	re7, _ := syntax.Parse("\\d+", syntax.Perl)
	fmt.Println(re7.Op == syntax.OpPlus)
	fmt.Println(re7.Sub[0].Rune[0], re7.Sub[0].Rune[1])

	_, err8 := syntax.Parse("(ab", syntax.Perl)
	fmt.Println(err8 == nil)
}
