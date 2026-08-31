// Tiny subset of go/ast: one tagged Node struct instead of real Go's
// deep Node/Expr/Stmt/Decl interface hierarchy (dozens of concrete
// types each implementing a marker interface) -- this compiler's own
// internal AST uses exactly this same tagged-struct shape, for the same
// reason: it's far less risky to generate than a wide interface
// hierarchy. Kind constants are named after the real go/ast type they
// stand in for (ast.BinaryExpr, ast.IfStmt, ...) so code reads the same
// even though there's no actual `type BinaryExpr struct{...}`.
package ast

import "go/token"

type Kind int

const (
	Bad Kind = iota

	// Expressions
	Ident
	BasicLit
	BinaryExpr
	UnaryExpr
	ParenExpr
	CallExpr
	SelectorExpr
	IndexExpr
	CompositeLit

	// Types (also expressions, syntactically)
	PointerType
	ArrayType // always a slice type here -- no fixed-size [N]T
	MapType

	// Statements
	ExprStmt
	AssignStmt
	ReturnStmt
	IfStmt
	ForStmt
	RangeStmt
	SwitchStmt
	CaseClause
	BlockStmt
	IncDecStmt
	BranchStmt
	DeclStmt

	// Decls / misc
	Field
	FuncDecl
	VarSpec
	ConstSpec
	File

	// Appended so existing Kind values stay stable (astpkg compares by name,
	// but anything that stored the iota number would shift if these were
	// inserted earlier). TypeSpec is `type Name T` / `type Name[T] T`;
	// StructType/InterfaceType/FuncType are the type expressions themselves.
	TypeSpec
	StructType
	InterfaceType
	FuncType
)

// Node is every AST node this package produces. Only the fields
// relevant to a given Kind are meaningful; see the comment on each.
type Node struct {
	Kind Kind
	Pos  token.Pos

	Name    string      // Ident.Name; SelectorExpr's field name; FuncDecl/VarSpec/ConstSpec/TypeSpec.Name; File.Package
	Lit     string      // BasicLit.Value (literal text, as scanned)
	LitKind token.Token // BasicLit.Kind (INT/FLOAT/STRING/CHAR)

	Op token.Token // BinaryExpr/UnaryExpr/AssignStmt/IncDecStmt/BranchStmt/RangeStmt operator/keyword

	X *Node // BinaryExpr/UnaryExpr/ParenExpr/SelectorExpr/IndexExpr left operand;
	// CallExpr's callee; ExprStmt/IncDecStmt's expression;
	// PointerType/ArrayType's element type; RangeStmt's ranged-over expression;
	// VarSpec/ConstSpec's initializer (nil if none);
	// FuncDecl's receiver Field (nil if a package function)
	Y *Node // BinaryExpr's right operand; IndexExpr's index; MapType's value type (X is the key type)

	Args []*Node // CallExpr.Args; CompositeLit's elements (positional only, no keyed elements)

	Lhs []*Node // AssignStmt left side; RangeStmt's [key] or [key, value] (idents, possibly "_")
	Rhs []*Node // AssignStmt right side; ReturnStmt's result expressions

	Init *Node // IfStmt/ForStmt/SwitchStmt init statement (nil if none)
	Cond *Node // IfStmt/ForStmt condition (nil for a bare `for {}`); SwitchStmt's tag expr (nil for `switch {}`)
	Post *Node // ForStmt post statement (nil if none)
	Body *Node // IfStmt/ForStmt/RangeStmt/FuncDecl body (a BlockStmt node)
	Else *Node // IfStmt else branch (another IfStmt, a BlockStmt, or nil)

	List []*Node // BlockStmt.List; File.Decls; SwitchStmt's CaseClause list;
	// CaseClause's body statements (its Args holds the case expressions,
	// empty meaning `default`); StructType/InterfaceType members (Field nodes)

	Params  []*Node // FuncDecl/FuncType parameters (Field nodes); TypeSpec type params
	Results []*Node // FuncDecl/FuncType results (Field nodes)
	Type    *Node   // Field.Type; VarSpec/ConstSpec's declared type (nil if inferred from X);
	// CompositeLit.Type; TypeSpec's underlying/interface/struct type;
	// PointerType/ArrayType/MapType are themselves the type
}
