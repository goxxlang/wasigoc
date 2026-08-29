package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
)

func main() {
	e, err := parser.ParseExpr("1 + 2 * 3")
	fmt.Println(err == nil)
	fmt.Println(e.Kind == ast.BinaryExpr)
	fmt.Println(e.Op == token.ADD)
	fmt.Println(e.Y.Kind == ast.BinaryExpr)
	fmt.Println(e.Y.Op == token.MUL)

	e2, err2 := parser.ParseExpr("fmt.Println(a, b+1)")
	fmt.Println(err2 == nil)
	fmt.Println(e2.Kind == ast.CallExpr)
	fmt.Println(e2.X.Kind == ast.SelectorExpr)
	fmt.Println(e2.X.Name)
	fmt.Println(len(e2.Args))

	_, err3 := parser.ParseExpr("1 +")
	fmt.Println(err3 != nil)

	src := "package main\n\nfunc add(a int, b int) int {\n\tif a > b {\n\t\treturn a\n\t}\n\tsum := a + b\n\tfor sum > 0 {\n\t\tsum--\n\t}\n\treturn sum\n}\n"
	f, err4 := parser.ParseFile(src)
	fmt.Println(err4 == nil)
	fmt.Println(f.Kind == ast.File)
	fmt.Println(f.Name)
	fmt.Println(len(f.List))

	fd := f.List[0]
	fmt.Println(fd.Kind == ast.FuncDecl)
	fmt.Println(fd.Name)
	fmt.Println(len(fd.Params))
	fmt.Println(fd.Params[0].Name)
	fmt.Println(fd.Params[0].Type.Name)
	fmt.Println(len(fd.Results))
	fmt.Println(fd.Results[0].Type.Name)
	fmt.Println(len(fd.Body.List))

	stmt0 := fd.Body.List[0]
	fmt.Println(stmt0.Kind == ast.IfStmt)
	stmt1 := fd.Body.List[1]
	fmt.Println(stmt1.Kind == ast.AssignStmt)
	fmt.Println(stmt1.Op == token.DEFINE)
	stmt2 := fd.Body.List[2]
	fmt.Println(stmt2.Kind == ast.ForStmt)
	stmt3 := fd.Body.List[3]
	fmt.Println(stmt3.Kind == ast.ReturnStmt)
}
