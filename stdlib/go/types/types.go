// Tiny subset of go/types: a real (bounded) expression type checker over
// go/ast's Node, built on an INTERNED type representation -- every
// distinct type (by structural shape: "int", "*int", "[]string", ...)
// gets exactly one canonical *Type, cached in a package-level map keyed
// by that shape's string. Two *Type values are the same type exactly
// when they're the same pointer (Identical is `a == b`, nothing deeper).
//
// This is the same "Object Type Identifier" shape Blink's WrapperTypeInfo
// uses for DOM wrapper objects, and the same idea this compiler's own
// runtime already uses for interface{} identity (see type_key_of<T>() in
// runtime.hpp: a stable per-C++-type token via a static object's address,
// compared by pointer, no RTTI needed) -- interning gives Go source the
// same property for its OWN static types: O(1) identity comparison
// instead of walking two type trees structurally every time.
//
// *Type values are ordinary Go pointers (built with plain &Type{...},
// same as every other stdlib package's pointer types) -- nothing
// Oilpan-specific here. Oilpan-lite (gc::GarbageCollected<T>/Member<T>/
// Trace, see runtime.hpp) is real infrastructure but isn't wired into
// the code generator yet -- `&T{...}` currently lowers to a plain `new`
// for every struct pointer in every package, this one included, so
// there's nothing extra to opt into or guard against here. Whenever the
// generator does start emitting Trace for struct pointers, an ordinary
// Go pointer field like Type.Elem needs no changes to become a traced
// edge -- that's the whole point of writing it as an ordinary Go pointer
// instead of reaching for something unusual.
//
// Scope: bounded to what go/parser can actually produce -- bool/int/
// float64/string, pointer-to/slice-of/map-of those (and of each other).
// CheckExpr handles idents, literals, binary/unary/paren, index (slice
// and map), composite literals (slice elements checked against the
// element type; a map literal's only checkable shape is the empty one,
// since go/parser doesn't parse keyed elements at all) -- not calls,
// which would need function signatures, out of scope here. CheckStmt
// handles var/const, assign/define, if/for (including conditions),
// range-for (binding key/value from the ranged type), switch (including
// a bare `switch {}`, treated as `switch true {}`), blocks -- not
// calls-as-statements (same boundary as CheckExpr). No package-level
// checking, no method sets, no generics, no go/types.Info-style result
// maps -- this is "can I check one function body," not a type checker
// for whole programs.
package types

import (
	"errors"
	"go/ast"
	"go/token"
)

type TypeKind int

const (
	Invalid TypeKind = iota
	Bool
	Int
	Float64
	String
	Pointer
	Slice
	Map
)

type Type struct {
	TKind TypeKind
	Elem  *Type  // Pointer/Slice's element type; Map's value type
	Key   *Type  // Map's key type only; nil otherwise
	Name  string // canonical shape string, e.g. "int", "*int", "[]string", "map[string]int"
}

func (t *Type) String() string { return t.Name }

var interned map[string]*Type

func intern(key string, kind TypeKind, elem *Type, keyType *Type) *Type {
	if interned == nil {
		interned = make(map[string]*Type)
	}
	existing, ok := interned[key]
	if ok {
		return existing
	}
	t := &Type{TKind: kind, Elem: elem, Key: keyType, Name: key}
	interned[key] = t
	return t
}

func basic(name string, kind TypeKind) *Type {
	return intern(name, kind, nil, nil)
}

var (
	BoolType    = basic("bool", Bool)
	IntType     = basic("int", Int)
	Float64Type = basic("float64", Float64)
	StringType  = basic("string", String)
)

func PointerTo(elem *Type) *Type {
	return intern("*"+elem.Name, Pointer, elem, nil)
}

func SliceOf(elem *Type) *Type {
	return intern("[]"+elem.Name, Slice, elem, nil)
}

func MapOf(key *Type, elem *Type) *Type {
	return intern("map["+key.Name+"]"+elem.Name, Map, elem, key)
}

// Identical reports whether a and b are the same type -- pointer
// equality, safe because every *Type in existence came from intern().
func Identical(a *Type, b *Type) bool {
	return a == b
}

func isComparisonOp(op token.Token) bool {
	return op == token.EQL || op == token.NEQ || op == token.LSS || op == token.LEQ ||
		op == token.GTR || op == token.GEQ || op == token.LAND || op == token.LOR
}

// Checker is a tiny type environment: a name maps to the *Type it was
// declared with. CheckExpr walks a go/ast expression and either infers
// its Type or reports the first mismatch/undefined name it finds.
type Checker struct {
	Env map[string]*Type
}

func NewChecker() *Checker {
	return &Checker{Env: make(map[string]*Type)}
}

func (c *Checker) CheckExpr(n *ast.Node) (*Type, error) {
	if n == nil {
		return nil, errors.New("types: nil expression")
	}
	if n.Kind == ast.Ident {
		t, ok := c.Env[n.Name]
		if !ok {
			return nil, errors.New("types: undefined: " + n.Name)
		}
		return t, nil
	}
	if n.Kind == ast.BasicLit {
		if n.LitKind == token.INT {
			return IntType, nil
		}
		if n.LitKind == token.FLOAT {
			return Float64Type, nil
		}
		if n.LitKind == token.STRING {
			return StringType, nil
		}
		return nil, errors.New("types: unsupported literal kind")
	}
	if n.Kind == ast.ParenExpr {
		return c.CheckExpr(n.X)
	}
	if n.Kind == ast.UnaryExpr {
		return c.CheckExpr(n.X)
	}
	if n.Kind == ast.BinaryExpr {
		xt, err := c.CheckExpr(n.X)
		if err != nil {
			return nil, err
		}
		yt, err2 := c.CheckExpr(n.Y)
		if err2 != nil {
			return nil, err2
		}
		if !Identical(xt, yt) {
			return nil, errors.New("types: mismatched types " + xt.String() + " and " + yt.String() +
				" (operator " + token.TokenString(n.Op) + ")")
		}
		if isComparisonOp(n.Op) {
			return BoolType, nil
		}
		return xt, nil
	}
	if n.Kind == ast.IndexExpr {
		xt, err := c.CheckExpr(n.X)
		if err != nil {
			return nil, err
		}
		if xt.TKind == Slice {
			it, err2 := c.CheckExpr(n.Y)
			if err2 != nil {
				return nil, err2
			}
			if !Identical(it, IntType) {
				return nil, errors.New("types: slice index must be int, got " + it.String())
			}
			return xt.Elem, nil
		}
		if xt.TKind == Map {
			kt, err2 := c.CheckExpr(n.Y)
			if err2 != nil {
				return nil, err2
			}
			if !Identical(kt, xt.Key) {
				return nil, errors.New("types: cannot use " + kt.String() + " as map key of type " + xt.Key.String())
			}
			return xt.Elem, nil
		}
		return nil, errors.New("types: cannot index a value of type " + xt.String())
	}
	if n.Kind == ast.CompositeLit {
		t, err := c.typeFromNode(n.Type)
		if err != nil {
			return nil, err
		}
		if t.TKind == Slice {
			for i := 0; i < len(n.Args); i++ {
				et, err2 := c.CheckExpr(n.Args[i])
				if err2 != nil {
					return nil, err2
				}
				if !Identical(et, t.Elem) {
					return nil, errors.New("types: cannot use " + et.String() + " as " + t.Elem.String() +
						" value in slice literal")
				}
			}
			return t, nil
		}
		if t.TKind == Map {
			// Keyed elements (`map[K]V{key: value}`) aren't parseable by
			// go/parser at all (positional elements only) -- the only
			// composite literal a map type can actually have here is the
			// empty one.
			if len(n.Args) != 0 {
				return nil, errors.New("types: map composite literal elements are not supported")
			}
			return t, nil
		}
		return nil, errors.New("types: unsupported composite literal type " + t.String())
	}
	return nil, errors.New("types: unsupported expression kind")
}

// typeFromNode resolves a go/ast type expression (an Ident naming a
// basic type, or a PointerType/ArrayType) into the corresponding
// interned *Type. Unknown identifiers, map types, and anything else
// go/ast can produce that isn't a type expression are all errors --
// there's no named-struct-type support here (this package doesn't parse
// `type X struct{...}` at all), so "cannot resolve" covers a lot of
// otherwise-valid Go.
func (c *Checker) typeFromNode(n *ast.Node) (*Type, error) {
	if n == nil {
		return nil, errors.New("types: nil type expression")
	}
	if n.Kind == ast.Ident {
		if n.Name == "bool" {
			return BoolType, nil
		}
		if n.Name == "int" {
			return IntType, nil
		}
		if n.Name == "float64" {
			return Float64Type, nil
		}
		if n.Name == "string" {
			return StringType, nil
		}
		return nil, errors.New("types: unknown type: " + n.Name)
	}
	if n.Kind == ast.PointerType {
		elem, err := c.typeFromNode(n.X)
		if err != nil {
			return nil, err
		}
		return PointerTo(elem), nil
	}
	if n.Kind == ast.ArrayType {
		elem, err := c.typeFromNode(n.X)
		if err != nil {
			return nil, err
		}
		return SliceOf(elem), nil
	}
	if n.Kind == ast.MapType {
		key, err := c.typeFromNode(n.X)
		if err != nil {
			return nil, err
		}
		elem, err2 := c.typeFromNode(n.Y)
		if err2 != nil {
			return nil, err2
		}
		return MapOf(key, elem), nil
	}
	return nil, errors.New("types: unsupported type expression")
}

// CheckStmt type-checks a statement (and, recursively, everything
// nested in it) against and through the Checker's Env -- a `var`/`:=`/
// range-for adds a binding, a plain `=`/compound assign or `if`/`for`/
// `switch` condition checks against what's already there. NOT
// supported: calls-as-statements (same boundary as CheckExpr) -- reports
// "unsupported" rather than silently skipping, so a caller can tell
// "this really doesn't type-check" from "this construct isn't
// implemented yet."
func (c *Checker) CheckStmt(n *ast.Node) error {
	if n == nil {
		return nil
	}
	if n.Kind == ast.ExprStmt {
		_, err := c.CheckExpr(n.X)
		return err
	}
	if n.Kind == ast.VarSpec || n.Kind == ast.ConstSpec {
		var declared *Type
		if n.Type != nil {
			t, err := c.typeFromNode(n.Type)
			if err != nil {
				return err
			}
			declared = t
		}
		if n.X != nil {
			xt, err := c.CheckExpr(n.X)
			if err != nil {
				return err
			}
			if declared == nil {
				declared = xt
			} else if !Identical(declared, xt) {
				return errors.New("types: cannot use " + xt.String() + " as " + declared.String() +
					" in declaration of " + n.Name)
			}
		}
		if declared == nil {
			return errors.New("types: cannot infer a type for " + n.Name)
		}
		c.Env[n.Name] = declared
		return nil
	}
	if n.Kind == ast.AssignStmt {
		if n.Op == token.DEFINE {
			if len(n.Lhs) != len(n.Rhs) {
				return errors.New("types: assignment count mismatch")
			}
			for i := 0; i < len(n.Lhs); i++ {
				t, err := c.CheckExpr(n.Rhs[i])
				if err != nil {
					return err
				}
				if n.Lhs[i].Kind == ast.Ident && n.Lhs[i].Name != "_" {
					c.Env[n.Lhs[i].Name] = t
				}
			}
			return nil
		}
		n2 := len(n.Lhs)
		if len(n.Rhs) < n2 {
			n2 = len(n.Rhs)
		}
		for i := 0; i < n2; i++ {
			// The blank identifier is a write-only sink -- valid on the
			// LHS of any assignment, never itself typed or bound.
			if n.Lhs[i].Kind == ast.Ident && n.Lhs[i].Name == "_" {
				if _, err := c.CheckExpr(n.Rhs[i]); err != nil {
					return err
				}
				continue
			}
			lt, err := c.CheckExpr(n.Lhs[i])
			if err != nil {
				return err
			}
			rt, err2 := c.CheckExpr(n.Rhs[i])
			if err2 != nil {
				return err2
			}
			if !Identical(lt, rt) {
				return errors.New("types: cannot assign " + rt.String() + " to " + lt.String())
			}
		}
		return nil
	}
	if n.Kind == ast.IncDecStmt {
		_, err := c.CheckExpr(n.X)
		return err
	}
	if n.Kind == ast.ReturnStmt {
		for i := 0; i < len(n.Rhs); i++ {
			_, err := c.CheckExpr(n.Rhs[i])
			if err != nil {
				return err
			}
		}
		return nil
	}
	if n.Kind == ast.BranchStmt {
		return nil
	}
	if n.Kind == ast.BlockStmt {
		for i := 0; i < len(n.List); i++ {
			err := c.CheckStmt(n.List[i])
			if err != nil {
				return err
			}
		}
		return nil
	}
	if n.Kind == ast.IfStmt {
		if n.Init != nil {
			if err := c.CheckStmt(n.Init); err != nil {
				return err
			}
		}
		ct, err := c.CheckExpr(n.Cond)
		if err != nil {
			return err
		}
		if !Identical(ct, BoolType) {
			return errors.New("types: if condition must be bool, got " + ct.String())
		}
		if err := c.CheckStmt(n.Body); err != nil {
			return err
		}
		if n.Else != nil {
			return c.CheckStmt(n.Else)
		}
		return nil
	}
	if n.Kind == ast.ForStmt {
		if n.Init != nil {
			if err := c.CheckStmt(n.Init); err != nil {
				return err
			}
		}
		if n.Cond != nil {
			ct, err := c.CheckExpr(n.Cond)
			if err != nil {
				return err
			}
			if !Identical(ct, BoolType) {
				return errors.New("types: for condition must be bool, got " + ct.String())
			}
		}
		if n.Post != nil {
			if err := c.CheckStmt(n.Post); err != nil {
				return err
			}
		}
		return c.CheckStmt(n.Body)
	}
	if n.Kind == ast.RangeStmt {
		xt, err := c.CheckExpr(n.X)
		if err != nil {
			return err
		}
		var keyType, valType *Type
		if xt.TKind == Slice {
			keyType, valType = IntType, xt.Elem
		} else if xt.TKind == Map {
			keyType, valType = xt.Key, xt.Elem
		} else {
			return errors.New("types: cannot range over a value of type " + xt.String())
		}
		if len(n.Lhs) > 0 && n.Lhs[0].Kind == ast.Ident && n.Lhs[0].Name != "_" {
			c.Env[n.Lhs[0].Name] = keyType
		}
		if len(n.Lhs) > 1 && n.Lhs[1].Kind == ast.Ident && n.Lhs[1].Name != "_" {
			c.Env[n.Lhs[1].Name] = valType
		}
		return c.CheckStmt(n.Body)
	}
	if n.Kind == ast.SwitchStmt {
		if n.Init != nil {
			if err := c.CheckStmt(n.Init); err != nil {
				return err
			}
		}
		tagType := BoolType
		if n.Cond != nil {
			ct, err := c.CheckExpr(n.Cond)
			if err != nil {
				return err
			}
			tagType = ct
		}
		for i := 0; i < len(n.List); i++ {
			cc := n.List[i]
			for j := 0; j < len(cc.Args); j++ {
				et, err := c.CheckExpr(cc.Args[j])
				if err != nil {
					return err
				}
				if !Identical(et, tagType) {
					return errors.New("types: cannot use " + et.String() + " as case value of type " + tagType.String())
				}
			}
			for j := 0; j < len(cc.List); j++ {
				if err := c.CheckStmt(cc.List[j]); err != nil {
					return err
				}
			}
		}
		return nil
	}
	return errors.New("types: unsupported statement kind")
}
