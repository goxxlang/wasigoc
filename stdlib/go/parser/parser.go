// Tiny subset of go/parser: real recursive-descent parsing (including
// correct binary-operator precedence) into go/ast's tagged Node, for a
// deliberately bounded slice of Go's grammar. Expressions: idents,
// literals, binary/unary/paren, calls, selectors, index, composite
// literals (`T{...}`/`[]T{...}`/`map[K]V{...}`, positional elements
// only -- no keyed `Field: value` elements). Types: identifier/`pkg.Type`,
// pointer (`*T`), slice (`[]T`, no fixed-size `[N]T`), map (`map[K]V`),
// func types, struct types, interface types (method signatures), and
// single-arg instantiation `Set[int]`. Statements: expr/assign/define/
// incdec, if, for (3-clause/cond-only/bare/range), switch (expr cases
// only, no type switch), return, break/continue, blocks, single-spec
// `var`/`const`/`type`. Decls: `func` with optional receiver
// (`func (d Duration) String() string`), `name type` params, one or
// parenthesized results, plus top-level `var`/`const`/`type` (including
// `type Set[T any] []T`). NOT supported: select/go/defer, type switch,
// labeled statements, goto, grouped `var (...)` / multi-name specs,
// multi-arg instantiations `Map[K, V]`.
//
// Composite literals need the same disambiguation real Go's own parser
// has: `Ident{` after `if`/`for`/`switch` (in their init/cond/post, not
// inside `{}`) is the block's opening brace, not a literal -- suppressed
// via p.allowCompositeLit exactly like wasigoc's own parser.cc does with
// its `allow_composite_lit_` flag for the identical reason.
//
// Error handling is "sticky state", not panic/recover: a syntax error
// sets p.err once, and every parse method checks it first and
// short-circuits to a zero-value Node if already set. A real go/parser-
// shaped implementation would panic deep in the call stack and recover
// once at the top (bailout via a single top-level `defer recover()`) --
// that doesn't work on this compiler: recover() here only catches a
// panic raised within the SAME function that has the defer (it's
// implemented as a stash-and-goto-the-epilogue within one function body,
// not real cross-function stack unwinding -- wasm32-wasip1 has no
// exception-handling proposal support for a real unwind, and this
// compiler doesn't build a setjmp/longjmp-based one either). A panic()
// three calls deep in expect()/parseOperand()/etc. would abort the whole
// program, not get caught by ParseExpr/ParseFile's defer. Sticky-state
// is the correct shape given that constraint, not a workaround for a bug
// -- keep this pattern for any future recursive-descent-shaped parser in
// this compiler rather than reaching for panic/recover again.
package parser

import (
	"errors"
	"go/ast"
	"go/scanner"
	"go/token"
)

type Parser struct {
	s                 *scanner.Scanner
	pos               token.Pos
	tok               token.Token
	lit               string
	err               error
	allowCompositeLit bool
}

func New(src string) *Parser {
	p := &Parser{s: scanner.New(src), allowCompositeLit: true}
	p.next()
	return p
}

func (p *Parser) next() {
	p.pos, p.tok, p.lit = p.s.Scan()
}

func (p *Parser) fail(msg string) {
	if p.err == nil {
		p.err = errors.New(msg)
	}
}

var bad = &ast.Node{Kind: ast.Bad}

func (p *Parser) expect(tok token.Token) token.Pos {
	if p.err != nil {
		return token.NoPos
	}
	pos := p.pos
	if p.tok != tok {
		p.fail("parser: expected " + token.TokenString(tok) + ", got " + token.TokenString(p.tok))
		return token.NoPos
	}
	p.next()
	return pos
}

func (p *Parser) expectIdent() string {
	if p.err != nil {
		return ""
	}
	if p.tok != token.IDENT {
		p.fail("parser: expected an identifier, got " + token.TokenString(p.tok))
		return ""
	}
	name := p.lit
	p.next()
	return name
}

// --- Expressions ---------------------------------------------------------

// binPrec matches the real Go operator precedence table (5 highest, 1
// lowest); 0 means "not a binary operator".
func binPrec(tok token.Token) int {
	if tok == token.MUL || tok == token.QUO || tok == token.REM ||
		tok == token.SHL || tok == token.SHR || tok == token.AND || tok == token.AND_NOT {
		return 5
	}
	if tok == token.ADD || tok == token.SUB || tok == token.OR || tok == token.XOR {
		return 4
	}
	if tok == token.EQL || tok == token.NEQ || tok == token.LSS || tok == token.LEQ ||
		tok == token.GTR || tok == token.GEQ {
		return 3
	}
	if tok == token.LAND {
		return 2
	}
	if tok == token.LOR {
		return 1
	}
	return 0
}

func (p *Parser) parseExpr() *ast.Node {
	return p.parseBinaryExpr(1)
}

func (p *Parser) parseBinaryExpr(minPrec int) *ast.Node {
	x := p.parseUnaryExpr()
	for p.err == nil {
		prec := binPrec(p.tok)
		if prec < minPrec {
			return x
		}
		op := p.tok
		pos := p.pos
		p.next()
		y := p.parseBinaryExpr(prec + 1)
		x = &ast.Node{Kind: ast.BinaryExpr, Pos: pos, Op: op, X: x, Y: y}
	}
	return x
}

func (p *Parser) parseUnaryExpr() *ast.Node {
	if p.err != nil {
		return bad
	}
	// AND ("&x", address-of) and MUL ("*x", dereference) are unary
	// prefix operators in expression position, same UnaryExpr shape as
	// +/-/!/^ -- unrelated to MUL/AND's binary meaning (that's handled
	// entirely in parseBinaryExpr, which only ever sees these tokens
	// *between* two already-parsed operands, never at the start of one).
	if p.tok == token.ADD || p.tok == token.SUB || p.tok == token.NOT || p.tok == token.XOR ||
		p.tok == token.AND || p.tok == token.MUL {
		op := p.tok
		pos := p.pos
		p.next()
		x := p.parseUnaryExpr()
		return &ast.Node{Kind: ast.UnaryExpr, Pos: pos, Op: op, X: x}
	}
	return p.parsePrimaryExpr()
}

func (p *Parser) parsePrimaryExpr() *ast.Node {
	x := p.parseOperand()
	for p.err == nil {
		if p.tok == token.PERIOD {
			p.next()
			name := p.expectIdent()
			x = &ast.Node{Kind: ast.SelectorExpr, Pos: x.Pos, X: x, Name: name}
			continue
		}
		if p.tok == token.LPAREN {
			pos := p.pos
			p.next()
			var args []*ast.Node
			for p.err == nil && p.tok != token.RPAREN {
				args = append(args, p.parseExpr())
				if p.tok == token.COMMA {
					p.next()
					continue
				}
				break
			}
			p.expect(token.RPAREN)
			x = &ast.Node{Kind: ast.CallExpr, Pos: pos, X: x, Args: args}
			continue
		}
		if p.tok == token.LBRACK {
			pos := p.pos
			p.next()
			idx := p.parseExpr()
			p.expect(token.RBRACK)
			x = &ast.Node{Kind: ast.IndexExpr, Pos: pos, X: x, Y: idx}
			continue
		}
		if p.allowCompositeLit && p.tok == token.LBRACE && (x.Kind == ast.Ident || x.Kind == ast.SelectorExpr) {
			x = p.parseCompositeLitBody(x)
			continue
		}
		break
	}
	return x
}

// parseCompositeLitBody parses the `{ elem, elem, ... }` body of a
// composite literal whose type (typ) has already been parsed -- element
// positions, not keys (`Field: value`), and nested literals inside the
// braces are always allowed regardless of the enclosing context (the
// suppression only applies to the *outermost* literal in an if/for/
// switch header).
func (p *Parser) parseCompositeLitBody(typ *ast.Node) *ast.Node {
	pos := p.expect(token.LBRACE)
	saved := p.allowCompositeLit
	p.allowCompositeLit = true
	var elts []*ast.Node
	for p.err == nil && p.tok != token.RBRACE {
		elts = append(elts, p.parseExpr())
		if p.tok == token.COMMA {
			p.next()
			continue
		}
		break
	}
	p.allowCompositeLit = saved
	p.expect(token.RBRACE)
	return &ast.Node{Kind: ast.CompositeLit, Pos: pos, Type: typ, Args: elts}
}

func (p *Parser) parseOperand() *ast.Node {
	if p.err != nil {
		return bad
	}
	if p.tok == token.IDENT {
		pos := p.pos
		name := p.lit
		p.next()
		return &ast.Node{Kind: ast.Ident, Pos: pos, Name: name}
	}
	if p.tok == token.INT || p.tok == token.FLOAT || p.tok == token.STRING || p.tok == token.CHAR {
		pos := p.pos
		kind := p.tok
		lit := p.lit
		p.next()
		return &ast.Node{Kind: ast.BasicLit, Pos: pos, LitKind: kind, Lit: lit}
	}
	if p.tok == token.LPAREN {
		pos := p.pos
		p.next()
		x := p.parseExpr()
		p.expect(token.RPAREN)
		return &ast.Node{Kind: ast.ParenExpr, Pos: pos, X: x}
	}
	// `[]T{...}`/`map[K]V{...}` -- a slice/map type used as a composite
	// literal's type. These don't start with an identifier, so they
	// can't come from the Ident branch above the way `Point{...}`/
	// `pkg.Point{...}` do (parsePrimaryExpr's loop handles those once
	// this function returns the plain Ident/SelectorExpr).
	if p.tok == token.LBRACK || p.tok == token.MAP {
		typ := p.parseType()
		if p.err == nil && p.allowCompositeLit && p.tok == token.LBRACE {
			return p.parseCompositeLitBody(typ)
		}
		return typ
	}
	p.fail("parser: expected an operand, got " + token.TokenString(p.tok))
	return bad
}

// --- Statements ------------------------------------------------------------

func (p *Parser) parseSimpleStmt() *ast.Node {
	if p.err != nil {
		return bad
	}
	pos := p.pos
	x := p.parseExpr()
	if p.tok == token.INC || p.tok == token.DEC {
		op := p.tok
		p.next()
		return &ast.Node{Kind: ast.IncDecStmt, Pos: pos, Op: op, X: x}
	}
	// A comma-separated LHS (`a, b := f()`) has to be collected before
	// knowing whether this is even an assignment at all -- the operator
	// only shows up after the whole list.
	lhs := []*ast.Node{x}
	for p.err == nil && p.tok == token.COMMA {
		p.next()
		lhs = append(lhs, p.parseExpr())
	}
	if p.tok == token.ASSIGN || p.tok == token.DEFINE || isCompoundAssign(p.tok) {
		op := p.tok
		p.next()
		// `k, v := range x` / `k, v = range x` -- only valid directly
		// inside a `for` header, but recognizing it here (rather than
		// only in parseForStmt) means one LHS-collection codepath
		// handles both the assign and the range-for shape.
		if p.err == nil && p.tok == token.RANGE && (op == token.ASSIGN || op == token.DEFINE) {
			p.next()
			rx := p.parseExpr()
			return &ast.Node{Kind: ast.RangeStmt, Pos: pos, Op: op, Lhs: lhs, X: rx}
		}
		var rhs []*ast.Node
		rhs = append(rhs, p.parseExpr())
		for p.err == nil && p.tok == token.COMMA {
			p.next()
			rhs = append(rhs, p.parseExpr())
		}
		return &ast.Node{Kind: ast.AssignStmt, Pos: pos, Op: op, Lhs: lhs, Rhs: rhs}
	}
	return &ast.Node{Kind: ast.ExprStmt, Pos: pos, X: x}
}

func isCompoundAssign(tok token.Token) bool {
	return tok == token.ADD_ASSIGN || tok == token.SUB_ASSIGN || tok == token.MUL_ASSIGN ||
		tok == token.QUO_ASSIGN || tok == token.REM_ASSIGN || tok == token.AND_ASSIGN ||
		tok == token.OR_ASSIGN || tok == token.XOR_ASSIGN || tok == token.SHL_ASSIGN ||
		tok == token.SHR_ASSIGN || tok == token.AND_NOT_ASSIGN
}

func (p *Parser) parseBlockStmt() *ast.Node {
	pos := p.expect(token.LBRACE)
	var list []*ast.Node
	for p.err == nil && p.tok != token.RBRACE && p.tok != token.EOF {
		list = append(list, p.parseStmt())
	}
	p.expect(token.RBRACE)
	return &ast.Node{Kind: ast.BlockStmt, Pos: pos, List: list}
}

func (p *Parser) parseIfStmt() *ast.Node {
	pos := p.expect(token.IF)
	saved := p.allowCompositeLit
	p.allowCompositeLit = false
	cond := p.parseExpr()
	p.allowCompositeLit = saved
	body := p.parseBlockStmt()
	n := &ast.Node{Kind: ast.IfStmt, Pos: pos, Cond: cond, Body: body}
	if p.err == nil && p.tok == token.ELSE {
		p.next()
		if p.tok == token.IF {
			n.Else = p.parseIfStmt()
		} else {
			n.Else = p.parseBlockStmt()
		}
	}
	if p.err == nil && p.tok == token.SEMICOLON {
		p.next()
	}
	return n
}

func (p *Parser) parseForStmt() *ast.Node {
	pos := p.expect(token.FOR)
	if p.err != nil {
		return bad
	}
	if p.tok == token.LBRACE {
		body := p.parseBlockStmt()
		if p.err == nil && p.tok == token.SEMICOLON {
			p.next()
		}
		return &ast.Node{Kind: ast.ForStmt, Pos: pos, Body: body}
	}
	if p.tok == token.RANGE {
		p.next()
		saved := p.allowCompositeLit
		p.allowCompositeLit = false
		rx := p.parseExpr()
		p.allowCompositeLit = saved
		body := p.parseBlockStmt()
		if p.err == nil && p.tok == token.SEMICOLON {
			p.next()
		}
		return &ast.Node{Kind: ast.RangeStmt, Pos: pos, X: rx, Body: body}
	}
	saved := p.allowCompositeLit
	p.allowCompositeLit = false
	// A single simple statement before "{" is either the condition-only
	// form (`for cond {`) or a range-for with a key/value LHS
	// (parseSimpleStmt itself recognizes "... := range x" and returns a
	// RangeStmt directly); a ';'-separated triple is the classic
	// 3-clause form.
	first := p.parseSimpleStmt()
	if first.Kind == ast.RangeStmt {
		p.allowCompositeLit = saved
		body := p.parseBlockStmt()
		if p.err == nil && p.tok == token.SEMICOLON {
			p.next()
		}
		first.Pos = pos
		first.Body = body
		return first
	}
	if p.err == nil && p.tok == token.LBRACE {
		p.allowCompositeLit = saved
		body := p.parseBlockStmt()
		if p.err == nil && p.tok == token.SEMICOLON {
			p.next()
		}
		return &ast.Node{Kind: ast.ForStmt, Pos: pos, Cond: first.X, Body: body}
	}
	p.expect(token.SEMICOLON)
	var cond *ast.Node
	if p.err == nil && p.tok != token.SEMICOLON {
		cond = p.parseExpr()
	}
	p.expect(token.SEMICOLON)
	var post *ast.Node
	if p.err == nil && p.tok != token.LBRACE {
		post = p.parseSimpleStmt()
	}
	p.allowCompositeLit = saved
	body := p.parseBlockStmt()
	if p.err == nil && p.tok == token.SEMICOLON {
		p.next()
	}
	return &ast.Node{Kind: ast.ForStmt, Pos: pos, Init: first, Cond: cond, Post: post, Body: body}
}

func (p *Parser) parseSwitchStmt() *ast.Node {
	pos := p.expect(token.SWITCH)
	saved := p.allowCompositeLit
	p.allowCompositeLit = false
	var init, tag *ast.Node
	if p.err == nil && p.tok != token.LBRACE {
		first := p.parseSimpleStmt()
		if p.tok == token.SEMICOLON {
			p.next()
			init = first
			if p.err == nil && p.tok != token.LBRACE {
				tag = p.parseExpr()
			}
		} else if first.Kind == ast.ExprStmt {
			tag = first.X
		}
	}
	p.allowCompositeLit = saved
	p.expect(token.LBRACE)
	var cases []*ast.Node
	for p.err == nil && p.tok != token.RBRACE && p.tok != token.EOF {
		cases = append(cases, p.parseCaseClause())
	}
	p.expect(token.RBRACE)
	if p.err == nil && p.tok == token.SEMICOLON {
		p.next()
	}
	return &ast.Node{Kind: ast.SwitchStmt, Pos: pos, Init: init, Cond: tag, List: cases}
}

func (p *Parser) parseCaseClause() *ast.Node {
	pos := p.pos
	var exprs []*ast.Node
	if p.tok == token.CASE {
		p.next()
		exprs = append(exprs, p.parseExpr())
		for p.err == nil && p.tok == token.COMMA {
			p.next()
			exprs = append(exprs, p.parseExpr())
		}
	} else {
		p.expect(token.DEFAULT)
	}
	p.expect(token.COLON)
	var stmts []*ast.Node
	for p.err == nil && p.tok != token.CASE && p.tok != token.DEFAULT &&
		p.tok != token.RBRACE && p.tok != token.EOF {
		stmts = append(stmts, p.parseStmt())
	}
	return &ast.Node{Kind: ast.CaseClause, Pos: pos, Args: exprs, List: stmts}
}

func (p *Parser) parseReturnStmt() *ast.Node {
	pos := p.expect(token.RETURN)
	var results []*ast.Node
	if p.err == nil && p.tok != token.SEMICOLON && p.tok != token.RBRACE {
		results = append(results, p.parseExpr())
		for p.err == nil && p.tok == token.COMMA {
			p.next()
			results = append(results, p.parseExpr())
		}
	}
	if p.err == nil && p.tok == token.SEMICOLON {
		p.next()
	}
	return &ast.Node{Kind: ast.ReturnStmt, Pos: pos, Rhs: results}
}

func (p *Parser) parseBranchStmt() *ast.Node {
	if p.err != nil {
		return bad
	}
	pos := p.pos
	op := p.tok
	p.next()
	if p.tok == token.SEMICOLON {
		p.next()
	}
	return &ast.Node{Kind: ast.BranchStmt, Pos: pos, Op: op}
}

func (p *Parser) parseStmt() *ast.Node {
	if p.err != nil {
		return bad
	}
	if p.tok == token.IF {
		return p.parseIfStmt()
	}
	if p.tok == token.FOR {
		return p.parseForStmt()
	}
	if p.tok == token.SWITCH {
		return p.parseSwitchStmt()
	}
	if p.tok == token.RETURN {
		return p.parseReturnStmt()
	}
	if p.tok == token.LBRACE {
		n := p.parseBlockStmt()
		if p.err == nil && p.tok == token.SEMICOLON {
			p.next()
		}
		return n
	}
	if p.tok == token.BREAK || p.tok == token.CONTINUE {
		return p.parseBranchStmt()
	}
	if p.tok == token.VAR || p.tok == token.CONST || p.tok == token.TYPE {
		var n *ast.Node
		if p.tok == token.TYPE {
			n = p.parseTypeSpec()
		} else {
			n = p.parseSpecDecl()
		}
		if p.err == nil && p.tok == token.SEMICOLON {
			p.next()
		}
		return n
	}
	n := p.parseSimpleStmt()
	if p.err == nil && p.tok == token.SEMICOLON {
		p.next()
	}
	return n
}

// --- Declarations ----------------------------------------------------------

// parseType supports a bare identifier or a qualified `pkg.Type`
// (flattened into one Ident node whose Name is "pkg.Type"), a pointer
// `*T`, a slice `[]T` (no fixed-size `[N]T`), a map `map[K]V`, a func
// type, a struct type, an interface type, or a single-arg instantiation
// `Set[int]` (IndexExpr).
func (p *Parser) parseType() *ast.Node {
	if p.err != nil {
		return bad
	}
	if p.tok == token.MUL {
		pos := p.pos
		p.next()
		elem := p.parseType()
		return &ast.Node{Kind: ast.PointerType, Pos: pos, X: elem}
	}
	if p.tok == token.LBRACK {
		pos := p.pos
		p.next()
		p.expect(token.RBRACK)
		elem := p.parseType()
		return &ast.Node{Kind: ast.ArrayType, Pos: pos, X: elem}
	}
	if p.tok == token.MAP {
		pos := p.pos
		p.next()
		p.expect(token.LBRACK)
		key := p.parseType()
		p.expect(token.RBRACK)
		val := p.parseType()
		return &ast.Node{Kind: ast.MapType, Pos: pos, X: key, Y: val}
	}
	if p.tok == token.FUNC {
		return p.parseFuncType()
	}
	if p.tok == token.INTERFACE {
		return p.parseInterfaceType()
	}
	if p.tok == token.STRUCT {
		return p.parseStructType()
	}
	return p.parseTypeName()
}

func (p *Parser) parseTypeName() *ast.Node {
	pos := p.pos
	name := p.expectIdent()
	if p.err == nil && p.tok == token.PERIOD {
		p.next()
		name = name + "." + p.expectIdent()
	}
	n := &ast.Node{Kind: ast.Ident, Pos: pos, Name: name}
	if p.err == nil && p.tok == token.LBRACK {
		p.next()
		arg := p.parseType()
		p.expect(token.RBRACK)
		return &ast.Node{Kind: ast.IndexExpr, Pos: pos, X: n, Y: arg}
	}
	return n
}

func (p *Parser) parseFuncType() *ast.Node {
	pos := p.expect(token.FUNC)
	p.expect(token.LPAREN)
	params := p.parseParamList()
	p.expect(token.RPAREN)
	results := p.parseResults()
	return &ast.Node{Kind: ast.FuncType, Pos: pos, Params: params, Results: results}
}

func (p *Parser) parseInterfaceType() *ast.Node {
	pos := p.expect(token.INTERFACE)
	p.expect(token.LBRACE)
	var list []*ast.Node
	for p.err == nil && p.tok != token.RBRACE && p.tok != token.EOF {
		mpos := p.pos
		name := p.expectIdent()
		p.expect(token.LPAREN)
		params := p.parseParamList()
		p.expect(token.RPAREN)
		results := p.parseResults()
		ft := &ast.Node{Kind: ast.FuncType, Pos: mpos, Params: params, Results: results}
		list = append(list, &ast.Node{Kind: ast.Field, Pos: mpos, Name: name, Type: ft})
		if p.err == nil && p.tok == token.SEMICOLON {
			p.next()
		}
	}
	p.expect(token.RBRACE)
	return &ast.Node{Kind: ast.InterfaceType, Pos: pos, List: list}
}

func (p *Parser) parseStructType() *ast.Node {
	pos := p.expect(token.STRUCT)
	p.expect(token.LBRACE)
	var list []*ast.Node
	for p.err == nil && p.tok != token.RBRACE && p.tok != token.EOF {
		fpos := p.pos
		name := p.expectIdent()
		typ := p.parseType()
		list = append(list, &ast.Node{Kind: ast.Field, Pos: fpos, Name: name, Type: typ})
		if p.err == nil && p.tok == token.SEMICOLON {
			p.next()
		}
	}
	p.expect(token.RBRACE)
	return &ast.Node{Kind: ast.StructType, Pos: pos, List: list}
}

func (p *Parser) isTypeStart() bool {
	return p.tok == token.IDENT || p.tok == token.MUL || p.tok == token.LBRACK ||
		p.tok == token.MAP || p.tok == token.FUNC || p.tok == token.INTERFACE ||
		p.tok == token.STRUCT
}

func (p *Parser) parseParamList() []*ast.Node {
	var fields []*ast.Node
	for p.err == nil && p.tok != token.RPAREN {
		pos := p.pos
		if p.tok == token.IDENT {
			name := p.lit
			p.next()
			if p.tok == token.PERIOD {
				p.next()
				q := name + "." + p.expectIdent()
				typ := &ast.Node{Kind: ast.Ident, Pos: pos, Name: q}
				if p.err == nil && p.tok == token.LBRACK {
					p.next()
					arg := p.parseType()
					p.expect(token.RBRACK)
					typ = &ast.Node{Kind: ast.IndexExpr, Pos: pos, X: typ, Y: arg}
				}
				fields = append(fields, &ast.Node{Kind: ast.Field, Pos: pos, Type: typ})
			} else if p.tok == token.LBRACK {
				base := &ast.Node{Kind: ast.Ident, Pos: pos, Name: name}
				p.next()
				arg := p.parseType()
				p.expect(token.RBRACK)
				typ := &ast.Node{Kind: ast.IndexExpr, Pos: pos, X: base, Y: arg}
				fields = append(fields, &ast.Node{Kind: ast.Field, Pos: pos, Type: typ})
			} else if p.tok == token.COMMA || p.tok == token.RPAREN {
				fields = append(fields, &ast.Node{Kind: ast.Field, Pos: pos, Type: &ast.Node{Kind: ast.Ident, Pos: pos, Name: name}})
			} else {
				typ := p.parseType()
				fields = append(fields, &ast.Node{Kind: ast.Field, Pos: pos, Name: name, Type: typ})
			}
		} else {
			typ := p.parseType()
			fields = append(fields, &ast.Node{Kind: ast.Field, Pos: pos, Type: typ})
		}
		if p.err == nil && p.tok == token.COMMA {
			p.next()
			continue
		}
		break
	}
	return fields
}

func (p *Parser) parseResults() []*ast.Node {
	if p.err != nil {
		return nil
	}
	if p.tok == token.LPAREN {
		p.next()
		fields := p.parseParamList()
		p.expect(token.RPAREN)
		return fields
	}
	if p.isTypeStart() {
		typ := p.parseType()
		return []*ast.Node{&ast.Node{Kind: ast.Field, Pos: typ.Pos, Type: typ}}
	}
	return nil
}

// parseSpecDecl parses a single-spec `var name [type] [= expr]` or
// `const name [type] [= expr]` -- no grouped `var (...)`/`const (...)`
// blocks, no multi-name specs (`var a, b int`).
func (p *Parser) parseSpecDecl() *ast.Node {
	kw := p.tok
	pos := p.pos
	p.next()
	name := p.expectIdent()
	var typ *ast.Node
	if p.err == nil && p.tok != token.ASSIGN && p.tok != token.SEMICOLON &&
		p.tok != token.RPAREN && p.tok != token.EOF {
		typ = p.parseType()
	}
	var init *ast.Node
	if p.err == nil && p.tok == token.ASSIGN {
		p.next()
		init = p.parseExpr()
	}
	kind := ast.VarSpec
	if kw == token.CONST {
		kind = ast.ConstSpec
	}
	return &ast.Node{Kind: kind, Pos: pos, Name: name, Type: typ, X: init}
}

func (p *Parser) parseTypeSpec() *ast.Node {
	pos := p.expect(token.TYPE)
	name := p.expectIdent()
	var params []*ast.Node
	if p.err == nil && p.tok == token.LBRACK {
		p.next()
		for p.err == nil && p.tok != token.RBRACK {
			ppos := p.pos
			pn := p.expectIdent()
			pt := p.parseType()
			params = append(params, &ast.Node{Kind: ast.Field, Pos: ppos, Name: pn, Type: pt})
			if p.err == nil && p.tok == token.COMMA {
				p.next()
				continue
			}
			break
		}
		p.expect(token.RBRACK)
	}
	typ := p.parseType()
	return &ast.Node{Kind: ast.TypeSpec, Pos: pos, Name: name, Params: params, Type: typ}
}

func (p *Parser) parseFieldList() []*ast.Node {
	return p.parseParamList()
}

func (p *Parser) parseFuncDecl() *ast.Node {
	pos := p.expect(token.FUNC)
	var recv *ast.Node
	if p.err == nil && p.tok == token.LPAREN {
		p.next()
		rpos := p.pos
		rname := p.expectIdent()
		rtyp := p.parseType()
		p.expect(token.RPAREN)
		recv = &ast.Node{Kind: ast.Field, Pos: rpos, Name: rname, Type: rtyp}
	}
	name := p.expectIdent()
	p.expect(token.LPAREN)
	params := p.parseParamList()
	p.expect(token.RPAREN)
	results := p.parseResults()
	body := p.parseBlockStmt()
	if p.err == nil && p.tok == token.SEMICOLON {
		p.next()
	}
	return &ast.Node{Kind: ast.FuncDecl, Pos: pos, Name: name, X: recv, Params: params, Results: results, Body: body}
}

func (p *Parser) parseFile() *ast.Node {
	pos := p.expect(token.PACKAGE)
	pkgName := p.expectIdent()
	if p.err == nil && p.tok == token.SEMICOLON {
		p.next()
	}
	var decls []*ast.Node
	for p.err == nil && p.tok == token.IMPORT {
		p.next()
		if p.tok == token.LPAREN {
			p.next()
			for p.err == nil && p.tok != token.RPAREN {
				p.next()
				if p.tok == token.SEMICOLON {
					p.next()
				}
			}
			p.expect(token.RPAREN)
		} else {
			p.next()
		}
		if p.err == nil && p.tok == token.SEMICOLON {
			p.next()
		}
	}
	for p.err == nil && (p.tok == token.FUNC || p.tok == token.VAR || p.tok == token.CONST || p.tok == token.TYPE) {
		if p.tok == token.FUNC {
			decls = append(decls, p.parseFuncDecl())
			continue
		}
		var d *ast.Node
		if p.tok == token.TYPE {
			d = p.parseTypeSpec()
		} else {
			d = p.parseSpecDecl()
		}
		if p.err == nil && p.tok == token.SEMICOLON {
			p.next()
		}
		decls = append(decls, d)
	}
	return &ast.Node{Kind: ast.File, Pos: pos, Name: pkgName, List: decls}
}

// --- Public entry points ----------------------------------------------------

func ParseExpr(src string) (*ast.Node, error) {
	p := New(src)
	result := p.parseExpr()
	if p.err != nil {
		return nil, p.err
	}
	return result, nil
}

func ParseFile(src string) (*ast.Node, error) {
	p := New(src)
	result := p.parseFile()
	if p.err != nil {
		return nil, p.err
	}
	return result, nil
}

// ParseFileWithComments is ParseFile plus the file's comments (for
// go/doc's comment-association) -- a separate entry point, additive,
// rather than changing ParseFile's own signature and breaking every
// existing caller.
func ParseFileWithComments(src string) (*ast.Node, []scanner.Comment, error) {
	p := New(src)
	result := p.parseFile()
	if p.err != nil {
		return nil, nil, p.err
	}
	return result, p.s.Comments, nil
}
