package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/types"
)

func check(c *types.Checker, src string) {
	e, err := parser.ParseExpr(src)
	if err != nil {
		fmt.Println("parse error")
		return
	}
	t, err2 := c.CheckExpr(e)
	if err2 != nil {
		fmt.Println("error")
		return
	}
	fmt.Println(t.String())
}

func main() {
	p1 := types.PointerTo(types.IntType)
	p2 := types.PointerTo(types.IntType)
	fmt.Println(types.Identical(p1, p2))
	fmt.Println(p1.String())

	s1 := types.SliceOf(types.StringType)
	s2 := types.SliceOf(types.StringType)
	fmt.Println(types.Identical(s1, s2))
	fmt.Println(s1.String())

	fmt.Println(types.Identical(types.IntType, types.Float64Type))

	c := types.NewChecker()
	check(c, "1 + 2")
	check(c, "\"a\" + \"b\"")
	check(c, "1 < 2")
	check(c, "1 + \"a\"")

	c.Env["x"] = types.IntType
	check(c, "x + 1")
	check(c, "y + 1")

	c.Env["p"] = types.PointerTo(types.IntType)
	pt, _ := parser.ParseExpr("p")
	rt, _ := c.CheckExpr(pt)
	fmt.Println(rt.String())
	fmt.Println(types.Identical(rt, types.PointerTo(types.IntType)))

	// CheckStmt: a whole function body, not just one expression.
	src := "package main\n\nfunc run() {\n\tvar x int = 5\n\tvar y = 3.5\n\tx = x + 1\n\tif x > 0 {\n\t\ty = 1.0\n\t}\n\tfor i := 0; i < 10; i++ {\n\t\tx = x + i\n\t}\n}\n"
	f, ferr := parser.ParseFile(src)
	fmt.Println(ferr == nil)

	body := f.List[0].Body.List
	fmt.Println(len(body))

	c2 := types.NewChecker()
	for i := 0; i < len(body); i++ {
		cerr := c2.CheckStmt(body[i])
		fmt.Println(cerr == nil)
	}
	fmt.Println(c2.Env["x"].String())
	fmt.Println(c2.Env["y"].String())

	// A real type mismatch should be caught, not silently accepted.
	badSrc := "package main\n\nfunc bad() {\n\tvar z int = 5\n\tz = \"oops\"\n}\n"
	f2, _ := parser.ParseFile(badSrc)
	c3 := types.NewChecker()
	body2 := f2.List[0].Body.List
	err1 := c3.CheckStmt(body2[0])
	fmt.Println(err1 == nil)
	err2 := c3.CheckStmt(body2[1])
	fmt.Println(err2 != nil)

	// A non-bool if-condition should be caught too.
	condSrc := "package main\n\nfunc badcond() {\n\tvar n int = 1\n\tif n {\n\t}\n}\n"
	f3, _ := parser.ParseFile(condSrc)
	c4 := types.NewChecker()
	body3 := f3.List[0].Body.List
	c4.CheckStmt(body3[0])
	err3 := c4.CheckStmt(body3[1])
	fmt.Println(err3 != nil)

	// Maps, indexing, composite literals, range-for, and switch.
	m1 := types.MapOf(types.StringType, types.IntType)
	m2 := types.MapOf(types.StringType, types.IntType)
	fmt.Println(types.Identical(m1, m2))
	fmt.Println(m1.String())

	c5 := types.NewChecker()
	c5.Env["arr"] = types.SliceOf(types.IntType)
	e, _ := parser.ParseExpr("arr[0]")
	t, err4 := c5.CheckExpr(e)
	fmt.Println(err4 == nil)
	fmt.Println(t.String())

	e2, _ := parser.ParseExpr("[]int{1, 2, 3}")
	t2, err5 := c5.CheckExpr(e2)
	fmt.Println(err5 == nil)
	fmt.Println(t2.String())

	e3, _ := parser.ParseExpr("[]int{1, \"x\"}")
	_, err6 := c5.CheckExpr(e3)
	fmt.Println(err6 != nil)

	rangeSrc := "package main\n\nfunc run() {\n\targ := []int{1, 2, 3}\n\tsum := 0\n\tfor i, v := range arg {\n\t\tsum = sum + v\n\t\t_ = i\n\t}\n\tswitch sum {\n\tcase 6:\n\t\tsum = sum + 1\n\tdefault:\n\t\tsum = 0\n\t}\n\tswitch {\n\tcase sum > 0:\n\t\tsum = 1\n\t}\n}\n"
	f4, ferr2 := parser.ParseFile(rangeSrc)
	fmt.Println(ferr2 == nil)
	body4 := f4.List[0].Body.List
	fmt.Println(len(body4))

	c6 := types.NewChecker()
	for i := 0; i < len(body4); i++ {
		serr := c6.CheckStmt(body4[i])
		fmt.Println(serr == nil)
	}

	badSwitchSrc := "package main\n\nfunc bad() {\n\targ := []int{1, 2, 3}\n\tswitch arg[0] {\n\tcase \"x\":\n\t}\n}\n"
	f5, _ := parser.ParseFile(badSwitchSrc)
	c7 := types.NewChecker()
	body5 := f5.List[0].Body.List
	c7.CheckStmt(body5[0])
	err7 := c7.CheckStmt(body5[1])
	fmt.Println(err7 != nil)

	// Object-type identity: defined types intern by name, not underlying.
	d1 := types.Named("Duration", types.IntType)
	d2 := types.Named("Duration", types.IntType)
	fmt.Println(types.Identical(d1, d2))
	fmt.Println(types.Identical(d1, types.IntType))
	fmt.Println(d1.String())

	in := []*types.Type{}
	in = append(in, types.SliceOf(types.IntType))
	out := []*types.Type{}
	out = append(out, types.IntType)
	readSig := types.FuncOf(in, out)
	readMs := []*types.Method{}
	readMs = append(readMs, types.NewMethod("Read", readSig))
	i1 := types.InterfaceOf(readMs)
	readMs2 := []*types.Method{}
	readMs2 = append(readMs2, types.NewMethod("Read", readSig))
	i2 := types.InterfaceOf(readMs2)
	fmt.Println(types.Identical(i1, i2))
	fmt.Println(i1.String())
	writeMs := []*types.Method{}
	writeMs = append(writeMs, types.NewMethod("Write", readSig))
	i3 := types.InterfaceOf(writeMs)
	fmt.Println(types.Identical(i1, i3))

	setInt := types.Instantiate("Set", types.IntType, types.SliceOf(types.IntType))
	fmt.Println(types.Identical(setInt, types.SliceOf(types.IntType)))
	fmt.Println(setInt.String())

	otiSrc := "package main\n\ntype Duration int\n\ntype Set[T any] []T\n\ntype Reader interface {\n\tRead([]int) int\n}\n\nfunc (d Duration) String() string {\n\treturn \"x\"\n}\n\nfunc run() {\n\tvar d Duration\n\ts := d.String()\n\t_ = s\n\tvar xs Set[int]\n\t_ = xs\n}\n"
	fOti, otiErr := parser.ParseFile(otiSrc)
	fmt.Println(otiErr == nil)
	fmt.Println(len(fOti.List))
	fmt.Println(fOti.List[0].Kind == ast.TypeSpec)
	fmt.Println(fOti.List[1].Name == "Set")
	fmt.Println(len(fOti.List[1].Params))
	fmt.Println(fOti.List[2].Type.Kind == ast.InterfaceType)
	fmt.Println(fOti.List[3].X != nil)

	cOti := types.NewChecker()
	fmt.Println(cOti.CheckFile(fOti) == nil)
	fmt.Println(cOti.Types["Duration"].String())
	fmt.Println(types.Identical(cOti.Types["Duration"], types.IntType))
	fmt.Println(types.Identical(cOti.Types["Reader"], i1))

	bodyOti := fOti.List[4].Body.List
	fmt.Println(len(bodyOti))
	fmt.Println(cOti.CheckStmt(bodyOti[0]) == nil)
	fmt.Println(cOti.CheckStmt(bodyOti[1]) == nil)
	fmt.Println(cOti.CheckStmt(bodyOti[2]) == nil)
	fmt.Println(cOti.CheckStmt(bodyOti[3]) == nil)
	fmt.Println(cOti.CheckStmt(bodyOti[4]) == nil)
	fmt.Println(cOti.Env["s"].String())
	fmt.Println(cOti.Env["xs"].String())
	fmt.Println(types.Identical(cOti.Env["xs"], types.SliceOf(types.IntType)))

	yin := []*types.Type{}
	yin = append(yin, types.IntType)
	yout := []*types.Type{}
	yout = append(yout, types.BoolType)
	yield := types.FuncOf(yin, yout)
	sin := []*types.Type{}
	sin = append(sin, yield)
	seq := types.FuncOf(sin, nil)
	cSeq := types.NewChecker()
	cSeq.Env["seq"] = seq
	rangeFnSrc := "package main\n\nfunc run() {\n\tfor v := range seq {\n\t\t_ = v\n\t}\n}\n"
	fSeq, _ := parser.ParseFile(rangeFnSrc)
	fmt.Println(cSeq.CheckStmt(fSeq.List[0].Body.List[0]) == nil)
	fmt.Println(cSeq.Env["v"].String())
}
