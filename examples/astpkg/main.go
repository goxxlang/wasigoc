package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
)

func main() {
	src := "package main\n\nvar x int = 5\n\nconst pi = 3\n\nfunc run() int {\n\tvar y = 10\n\tp := &y\n\targ := []int{1, 2, 3}\n\tm := map[string]int{}\n\tsum := 0\n\tfor i, v := range arg {\n\t\tm[\"k\"] = i + v\n\t\tsum = sum + v\n\t}\n\tswitch y {\n\tcase 1:\n\t\treturn 1\n\tdefault:\n\t\treturn sum\n\t}\n\treturn *p\n}\n"
	f, err := parser.ParseFile(src)
	fmt.Println(err == nil)
	fmt.Println(len(f.List))

	vd := f.List[0]
	fmt.Println(vd.Kind == ast.VarSpec)
	fmt.Println(vd.Name)
	fmt.Println(vd.Type.Name)

	cd := f.List[1]
	fmt.Println(cd.Kind == ast.ConstSpec)
	fmt.Println(cd.Name)

	fd := f.List[2]
	fmt.Println(fd.Kind == ast.FuncDecl)
	body := fd.Body.List

	fmt.Println(body[0].Kind == ast.VarSpec)
	fmt.Println(body[1].Kind == ast.AssignStmt)
	fmt.Println(body[1].Rhs[0].Kind == ast.UnaryExpr)

	argDecl := body[2]
	fmt.Println(argDecl.Rhs[0].Kind == ast.CompositeLit)
	fmt.Println(argDecl.Rhs[0].Type.Kind == ast.ArrayType)
	fmt.Println(len(argDecl.Rhs[0].Args))

	mDecl := body[3]
	fmt.Println(mDecl.Rhs[0].Type.Kind == ast.MapType)

	rangeStmt := body[5]
	fmt.Println(rangeStmt.Kind == ast.RangeStmt)
	fmt.Println(len(rangeStmt.Lhs))

	switchStmt := body[6]
	fmt.Println(switchStmt.Kind == ast.SwitchStmt)
	fmt.Println(len(switchStmt.List))
	fmt.Println(switchStmt.List[0].Args[0].Lit)
	fmt.Println(len(switchStmt.List[1].Args))

	out := printer.Sprint(f)
	fmt.Println(out)
}
