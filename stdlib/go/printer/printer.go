// Tiny subset of go/printer: prints a go/ast.Node back out as Go source
// text. Sprint(n) instead of real Go's Fprint(w, fset, node) -- no
// io.Writer/FileSet threading, just a string in, a string out, matching
// this project's usual simplification (see stdlib/time's FormatDuration
// for the same reasoning). Covers exactly the Node shapes go/parser
// produces; anything outside that (a hand-built Node with an
// unrecognized Kind) prints as a placeholder rather than erroring, since
// there's no error return to give the caller.
package printer

import (
	"go/ast"
	"go/token"
	"strconv"
)

type printer struct {
	buf    []byte
	indent int
}

func (p *printer) writeString(s string) {
	p.buf = append(p.buf, s...)
}

func (p *printer) newline() {
	p.buf = append(p.buf, byte(10))
	for i := 0; i < p.indent; i++ {
		p.writeString("\t")
	}
}

// Sprint returns n formatted as Go source text.
func Sprint(n *ast.Node) string {
	p := &printer{}
	p.printNode(n)
	return string(p.buf)
}

func (p *printer) printNode(n *ast.Node) {
	if n == nil {
		return
	}
	if n.Kind == ast.Ident {
		p.writeString(n.Name)
		return
	}
	if n.Kind == ast.BasicLit {
		p.writeString(n.Lit)
		return
	}
	if n.Kind == ast.BinaryExpr {
		p.printNode(n.X)
		p.writeString(" " + token.TokenString(n.Op) + " ")
		p.printNode(n.Y)
		return
	}
	if n.Kind == ast.UnaryExpr {
		p.writeString(token.TokenString(n.Op))
		p.printNode(n.X)
		return
	}
	if n.Kind == ast.ParenExpr {
		p.writeString("(")
		p.printNode(n.X)
		p.writeString(")")
		return
	}
	if n.Kind == ast.CallExpr {
		p.printNode(n.X)
		p.writeString("(")
		p.printExprList(n.Args)
		p.writeString(")")
		return
	}
	if n.Kind == ast.SelectorExpr {
		p.printNode(n.X)
		p.writeString("." + n.Name)
		return
	}
	if n.Kind == ast.IndexExpr {
		p.printNode(n.X)
		p.writeString("[")
		p.printNode(n.Y)
		p.writeString("]")
		return
	}
	if n.Kind == ast.PointerType {
		p.writeString("*")
		p.printNode(n.X)
		return
	}
	if n.Kind == ast.ArrayType {
		p.writeString("[]")
		p.printNode(n.X)
		return
	}
	if n.Kind == ast.MapType {
		p.writeString("map[")
		p.printNode(n.X)
		p.writeString("]")
		p.printNode(n.Y)
		return
	}
	if n.Kind == ast.CompositeLit {
		p.printNode(n.Type)
		p.writeString("{")
		p.printExprList(n.Args)
		p.writeString("}")
		return
	}
	if n.Kind == ast.ExprStmt {
		p.printNode(n.X)
		return
	}
	if n.Kind == ast.AssignStmt {
		p.printExprList(n.Lhs)
		p.writeString(" " + token.TokenString(n.Op) + " ")
		p.printExprList(n.Rhs)
		return
	}
	if n.Kind == ast.ReturnStmt {
		p.writeString("return")
		if len(n.Rhs) > 0 {
			p.writeString(" ")
			p.printExprList(n.Rhs)
		}
		return
	}
	if n.Kind == ast.IncDecStmt {
		p.printNode(n.X)
		p.writeString(token.TokenString(n.Op))
		return
	}
	if n.Kind == ast.BranchStmt {
		p.writeString(token.TokenString(n.Op))
		return
	}
	if n.Kind == ast.VarSpec || n.Kind == ast.ConstSpec {
		if n.Kind == ast.VarSpec {
			p.writeString("var ")
		} else {
			p.writeString("const ")
		}
		p.writeString(n.Name)
		if n.Type != nil {
			p.writeString(" ")
			p.printNode(n.Type)
		}
		if n.X != nil {
			p.writeString(" = ")
			p.printNode(n.X)
		}
		return
	}
	if n.Kind == ast.RangeStmt {
		p.writeString("for ")
		if len(n.Lhs) > 0 {
			p.printExprList(n.Lhs)
			p.writeString(" " + token.TokenString(n.Op) + " ")
		}
		p.writeString("range ")
		p.printNode(n.X)
		p.writeString(" ")
		p.printNode(n.Body)
		return
	}
	if n.Kind == ast.SwitchStmt {
		p.writeString("switch ")
		if n.Init != nil {
			p.printNode(n.Init)
			p.writeString("; ")
		}
		if n.Cond != nil {
			p.printNode(n.Cond)
			p.writeString(" ")
		}
		p.writeString("{")
		p.indent = p.indent + 1
		for i := 0; i < len(n.List); i++ {
			p.newline()
			p.printNode(n.List[i])
		}
		p.indent = p.indent - 1
		p.newline()
		p.writeString("}")
		return
	}
	if n.Kind == ast.CaseClause {
		if len(n.Args) > 0 {
			p.writeString("case ")
			p.printExprList(n.Args)
			p.writeString(":")
		} else {
			p.writeString("default:")
		}
		p.indent = p.indent + 1
		for i := 0; i < len(n.List); i++ {
			p.newline()
			p.printNode(n.List[i])
		}
		p.indent = p.indent - 1
		return
	}
	if n.Kind == ast.IfStmt {
		p.writeString("if ")
		p.printNode(n.Cond)
		p.writeString(" ")
		p.printNode(n.Body)
		if n.Else != nil {
			p.writeString(" else ")
			p.printNode(n.Else)
		}
		return
	}
	if n.Kind == ast.ForStmt {
		p.writeString("for ")
		if n.Init != nil {
			p.printNode(n.Init)
			p.writeString("; ")
			p.printNode(n.Cond)
			p.writeString("; ")
			p.printNode(n.Post)
			p.writeString(" ")
		} else if n.Cond != nil {
			p.printNode(n.Cond)
			p.writeString(" ")
		}
		p.printNode(n.Body)
		return
	}
	if n.Kind == ast.BlockStmt {
		p.writeString("{")
		p.indent = p.indent + 1
		for i := 0; i < len(n.List); i++ {
			p.newline()
			p.printNode(n.List[i])
		}
		p.indent = p.indent - 1
		p.newline()
		p.writeString("}")
		return
	}
	if n.Kind == ast.FuncDecl {
		p.writeString("func ")
		if n.X != nil {
			p.writeString("(")
			if n.X.Name != "" {
				p.writeString(n.X.Name + " ")
			}
			p.printNode(n.X.Type)
			p.writeString(") ")
		}
		p.writeString(n.Name + "(")
		p.printFieldList(n.Params)
		p.writeString(") ")
		if len(n.Results) == 1 && n.Results[0].Name == "" {
			p.printNode(n.Results[0].Type)
			p.writeString(" ")
		} else if len(n.Results) > 0 {
			p.writeString("(")
			p.printFieldList(n.Results)
			p.writeString(") ")
		}
		p.printNode(n.Body)
		return
	}
	if n.Kind == ast.TypeSpec {
		p.writeString("type " + n.Name)
		if len(n.Params) > 0 {
			p.writeString("[")
			p.printFieldList(n.Params)
			p.writeString("]")
		}
		p.writeString(" ")
		p.printNode(n.Type)
		return
	}
	if n.Kind == ast.FuncType {
		p.writeString("func")
		p.printFuncSig(n)
		return
	}
	if n.Kind == ast.InterfaceType {
		p.writeString("interface{")
		for i := 0; i < len(n.List); i++ {
			if i > 0 {
				p.writeString("; ")
			}
			m := n.List[i]
			p.writeString(m.Name)
			if m.Type != nil && m.Type.Kind == ast.FuncType {
				p.printFuncSig(m.Type)
			} else {
				p.printNode(m.Type)
			}
		}
		p.writeString("}")
		return
	}
	if n.Kind == ast.StructType {
		p.writeString("struct{")
		for i := 0; i < len(n.List); i++ {
			if i > 0 {
				p.writeString("; ")
			}
			f := n.List[i]
			if f.Name != "" {
				p.writeString(f.Name + " ")
			}
			p.printNode(f.Type)
		}
		p.writeString("}")
		return
	}
	if n.Kind == ast.File {
		p.writeString("package " + n.Name + "\n")
		for i := 0; i < len(n.List); i++ {
			p.writeString("\n")
			p.printNode(n.List[i])
			p.writeString("\n")
		}
		return
	}
	p.writeString("<" + strconv.Itoa(int(n.Kind)) + ">")
}

func (p *printer) printExprList(list []*ast.Node) {
	for i := 0; i < len(list); i++ {
		if i > 0 {
			p.writeString(", ")
		}
		p.printNode(list[i])
	}
}

func (p *printer) printFuncSig(n *ast.Node) {
	p.writeString("(")
	p.printFieldList(n.Params)
	p.writeString(")")
	if len(n.Results) == 1 && n.Results[0].Name == "" {
		p.writeString(" ")
		p.printNode(n.Results[0].Type)
	} else if len(n.Results) > 1 {
		p.writeString(" (")
		p.printFieldList(n.Results)
		p.writeString(")")
	}
}

func (p *printer) printFieldList(fields []*ast.Node) {
	for i := 0; i < len(fields); i++ {
		if i > 0 {
			p.writeString(", ")
		}
		if fields[i].Name != "" {
			p.writeString(fields[i].Name + " ")
		}
		p.printNode(fields[i].Type)
	}
}
