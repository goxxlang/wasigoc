package main

import (
	"fmt"
	"go/constant"
	"go/token"
)

func main() {
	a := constant.MakeInt64(7)
	b := constant.MakeInt64(3)

	sum := constant.BinaryOp(a, token.ADD, b)
	fmt.Println(constant.KindString(sum.Kind()))
	iv, ok := constant.Int64Val(sum)
	fmt.Println(iv, ok)

	quo := constant.BinaryOp(a, token.QUO, b)
	qv, _ := constant.Int64Val(quo)
	fmt.Println(qv)

	f := constant.MakeFloat64(2.5)
	fsum := constant.BinaryOp(a, token.ADD, f)
	fv, _ := constant.Float64Val(fsum)
	fmt.Println(fv)

	neg := constant.UnaryOp(token.SUB, a, 0)
	nv, _ := constant.Int64Val(neg)
	fmt.Println(nv)

	fmt.Println(constant.Compare(a, token.GTR, b))
	fmt.Println(constant.Compare(a, token.EQL, b))

	s1 := constant.MakeString("foo")
	s2 := constant.MakeString("bar")
	cat := constant.BinaryOp(s1, token.ADD, s2)
	fmt.Println(constant.StringVal(cat))

	t := constant.MakeBool(true)
	f2 := constant.MakeBool(false)
	fmt.Println(constant.BoolVal(constant.BinaryOp(t, token.LAND, f2)))
	fmt.Println(constant.BoolVal(constant.BinaryOp(t, token.LOR, f2)))

	fmt.Println(constant.Sign(constant.MakeInt64(-5)))
	fmt.Println(constant.Sign(constant.MakeInt64(0)))
	fmt.Println(constant.Sign(constant.MakeInt64(5)))

	unk := constant.BinaryOp(a, token.QUO, constant.MakeInt64(0))
	fmt.Println(constant.KindString(unk.Kind()))
}
