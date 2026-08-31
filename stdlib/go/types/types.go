// Tiny subset of go/types: a real (bounded) expression type checker over
// go/ast's Node, built on an INTERNED type representation -- every
// distinct type (by structural shape: "int", "*int", "[]string",
// "Duration", "interface{M()}", "func(func(int) bool)", "Set[int]", ...)
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
// Intern keys, by design:
//   - Defined types (`type Duration int`) intern by NAME ("Duration"),
//     not underlying, so Duration != int and type A int != type B int.
//   - Generic instantiations intern as "Set[int]" -- same name+args,
//     same pointer; distinct from the underlying `[]int`.
//   - Anonymous interfaces intern by method-set shape
//     ("interface{Read([]int)int}"), so a named `type Reader interface{
//     Read([]int) int}` and a written `interface{ Read([]int) int }`
//     are the same *Type.
//   - Func types intern by signature ("func(func(int) bool)"), which is
//     what makes range-over-func a type-identity check, not a special
//     AST walk.
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
// float64/string, pointer-to/slice-of/map-of those, defined named types,
// interned anonymous interfaces, interned func types, single-arg generic
// instantiations. CheckExpr handles idents, literals, binary/unary/paren,
// index (slice and map, including through a named underlying), composite
// literals, conversions (`Duration(1)`), and method calls (`d.String()`).
// CheckStmt handles var/const, assign/define, if/for (including
// conditions), range-for (slice/map, named underlying, and range-over-
// func), switch, blocks. CheckFile intern-binds TypeSpecs and attaches
// method sets on defined types. No go/types.Info-style result maps --
// this is still "can I check one file's types," not a whole-program
// checker.
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
	NamedKind
	Interface
	Func
	Struct
)

// Method is one entry in an interned method set (interfaces and defined
// types). Type is the interned func type of the signature.
type Method struct {
	Name string
	Type *Type
}

type Type struct {
	TKind   TypeKind
	Elem    *Type     // Pointer/Slice's element; Map's value; Named's underlying
	Key     *Type     // Map's key type only; nil otherwise
	Name    string    // canonical intern key
	In      []*Type   // Func parameter types
	Out     []*Type   // Func result types
	Methods []*Method // Interface method set, or Named type's methods
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

func joinTypes(ts []*Type) string {
	s := ""
	for i := 0; i < len(ts); i++ {
		if i > 0 {
			s = s + ","
		}
		s = s + ts[i].Name
	}
	return s
}

// FuncOf intern a function type by signature. Range-over-func is
// identity on this interned shape: `func(func(V) bool)` / `func(func(K, V) bool)`.
func FuncOf(in []*Type, out []*Type) *Type {
	key := "func(" + joinTypes(in) + ")"
	if len(out) == 1 {
		key = key + out[0].Name
	} else if len(out) > 1 {
		key = key + "(" + joinTypes(out) + ")"
	}
	t := intern(key, Func, nil, nil)
	if t.In == nil && t.Out == nil {
		t.In = in
		t.Out = out
	}
	return t
}

// Named intern a defined type by NAME, not underlying -- Duration != int.
func Named(name string, under *Type) *Type {
	return intern(name, NamedKind, under, nil)
}

// Instantiate intern `Name[Arg]` as a defined type whose underlying is
// `under` (already substituted). Distinct from `under` itself.
func Instantiate(name string, arg *Type, under *Type) *Type {
	return Named(name+"["+arg.Name+"]", under)
}

func methodSetKey(methods []*Method) string {
	key := "interface{"
	for i := 0; i < len(methods); i++ {
		if i > 0 {
			key = key + ";"
		}
		sig := methods[i].Type.Name
		if len(sig) >= 4 && sig[0:4] == "func" {
			sig = sig[4:]
		}
		key = key + methods[i].Name + sig
	}
	key = key + "}"
	return key
}

// InterfaceOf intern an (anonymous) interface by its method-set shape.
// The same methods intern to the same *Type whether written inline or
// bound to a name via `type Reader interface{ ... }`.
func InterfaceOf(methods []*Method) *Type {
	key := methodSetKey(methods)
	t := intern(key, Interface, nil, nil)
	if t.Methods == nil {
		t.Methods = methods
	}
	return t
}

func NewMethod(name string, sig *Type) *Method {
	m := &Method{}
	m.Name = name
	m.Type = sig
	return m
}

func LookupMethod(t *Type, name string) *Method {
	if t == nil {
		return nil
	}
	if t.TKind == Pointer {
		t = t.Elem
	}
	for i := 0; i < len(t.Methods); i++ {
		if t.Methods[i].Name == name {
			return t.Methods[i]
		}
	}
	return nil
}

func (t *Type) AddMethod(name string, sig *Type) {
	if t == nil || LookupMethod(t, name) != nil {
		return
	}
	t.Methods = append(t.Methods, NewMethod(name, sig))
}

func underlying(t *Type) *Type {
	if t != nil && t.TKind == NamedKind && t.Elem != nil {
		return t.Elem
	}
	return t
}

func structKey(fields []*Method) string {
	key := "struct{"
	for i := 0; i < len(fields); i++ {
		if i > 0 {
			key = key + ";"
		}
		key = key + fields[i].Name + " " + fields[i].Type.Name
	}
	key = key + "}"
	return key
}

func StructOf(fields []*Method) *Type {
	key := structKey(fields)
	t := intern(key, Struct, nil, nil)
	if t.Methods == nil {
		t.Methods = fields
	}
	return t
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
	Env      map[string]*Type     // value names
	Types    map[string]*Type     // declared type names (interned)
	Generics map[string]*ast.Node // TypeSpec nodes with type params
}

func NewChecker() *Checker {
	return &Checker{
		Env:      make(map[string]*Type),
		Types:    make(map[string]*Type),
		Generics: make(map[string]*ast.Node),
	}
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
		xt = underlying(xt)
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
		lit := underlying(t)
		if lit.TKind == Slice {
			for i := 0; i < len(n.Args); i++ {
				et, err2 := c.CheckExpr(n.Args[i])
				if err2 != nil {
					return nil, err2
				}
				if !Identical(et, lit.Elem) {
					return nil, errors.New("types: cannot use " + et.String() + " as " + lit.Elem.String() +
						" value in slice literal")
				}
			}
			return t, nil
		}
		if lit.TKind == Map {
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
	if n.Kind == ast.CallExpr {
		if n.X != nil && n.X.Kind == ast.Ident && c.Types != nil {
			dest, ok := c.Types[n.X.Name]
			if ok {
				if len(n.Args) != 1 {
					return nil, errors.New("types: conversion of " + n.X.Name + " takes one argument")
				}
				xt, err := c.CheckExpr(n.Args[0])
				if err != nil {
					return nil, err
				}
				if Identical(xt, dest) {
					return dest, nil
				}
				if dest.TKind == NamedKind && Identical(xt, dest.Elem) {
					return dest, nil
				}
				if xt.TKind == NamedKind && Identical(xt.Elem, dest) {
					return dest, nil
				}
				return nil, errors.New("types: cannot convert " + xt.String() + " to " + dest.String())
			}
		}
		if n.X != nil && n.X.Kind == ast.SelectorExpr {
			recv, err := c.CheckExpr(n.X.X)
			if err != nil {
				return nil, err
			}
			meth := LookupMethod(recv, n.X.Name)
			if meth == nil {
				return nil, errors.New("types: " + recv.String() + " has no method " + n.X.Name)
			}
			if len(meth.Type.Out) == 1 {
				return meth.Type.Out[0], nil
			}
			if len(meth.Type.Out) == 0 {
				return intern("()", Invalid, nil, nil), nil
			}
			return meth.Type, nil
		}
		return nil, errors.New("types: unsupported call")
	}
	return nil, errors.New("types: unsupported expression kind")
}

// typeFromNode resolves a go/ast type expression into the corresponding
// interned *Type -- identifiers (builtins and CheckFile-bound names),
// pointer/slice/map, func types, anonymous interfaces/structs, and a
// single-arg instantiation `Set[int]` via IndexExpr.
func (c *Checker) typeFromNode(n *ast.Node) (*Type, error) {
	return c.typeFromNodeSubst(n, nil)
}

func (c *Checker) typeFromNodeSubst(n *ast.Node, subst map[string]*Type) (*Type, error) {
	if n == nil {
		return nil, errors.New("types: nil type expression")
	}
	if n.Kind == ast.Ident {
		if subst != nil {
			t, ok := subst[n.Name]
			if ok {
				return t, nil
			}
		}
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
		if n.Name == "any" || n.Name == "comparable" {
			return intern(n.Name, NamedKind, nil, nil), nil
		}
		if c.Types != nil {
			t, ok := c.Types[n.Name]
			if ok {
				return t, nil
			}
		}
		return nil, errors.New("types: unknown type: " + n.Name)
	}
	if n.Kind == ast.PointerType {
		elem, err := c.typeFromNodeSubst(n.X, subst)
		if err != nil {
			return nil, err
		}
		return PointerTo(elem), nil
	}
	if n.Kind == ast.ArrayType {
		elem, err := c.typeFromNodeSubst(n.X, subst)
		if err != nil {
			return nil, err
		}
		return SliceOf(elem), nil
	}
	if n.Kind == ast.MapType {
		key, err := c.typeFromNodeSubst(n.X, subst)
		if err != nil {
			return nil, err
		}
		elem, err2 := c.typeFromNodeSubst(n.Y, subst)
		if err2 != nil {
			return nil, err2
		}
		return MapOf(key, elem), nil
	}
	if n.Kind == ast.FuncType {
		in, err := c.typesOfFieldsSubst(n.Params, subst)
		if err != nil {
			return nil, err
		}
		out, err2 := c.typesOfFieldsSubst(n.Results, subst)
		if err2 != nil {
			return nil, err2
		}
		return FuncOf(in, out), nil
	}
	if n.Kind == ast.InterfaceType {
		var methods []*Method
		for i := 0; i < len(n.List); i++ {
			f := n.List[i]
			sig, err := c.typeFromNodeSubst(f.Type, subst)
			if err != nil {
				return nil, err
			}
			methods = append(methods, NewMethod(f.Name, sig))
		}
		return InterfaceOf(methods), nil
	}
	if n.Kind == ast.StructType {
		var fields []*Method
		for i := 0; i < len(n.List); i++ {
			f := n.List[i]
			ft, err := c.typeFromNodeSubst(f.Type, subst)
			if err != nil {
				return nil, err
			}
			fields = append(fields, NewMethod(f.Name, ft))
		}
		return StructOf(fields), nil
	}
	if n.Kind == ast.IndexExpr {
		if n.X == nil || n.X.Kind != ast.Ident {
			return nil, errors.New("types: unsupported type instantiation")
		}
		if c.Generics == nil {
			return nil, errors.New("types: unknown generic type: " + n.X.Name)
		}
		spec, ok := c.Generics[n.X.Name]
		if !ok {
			return nil, errors.New("types: unknown generic type: " + n.X.Name)
		}
		arg, err := c.typeFromNodeSubst(n.Y, subst)
		if err != nil {
			return nil, err
		}
		var sub map[string]*Type
		if len(spec.Params) > 0 {
			sub = make(map[string]*Type)
			sub[spec.Params[0].Name] = arg
		}
		under, err2 := c.typeFromNodeSubst(spec.Type, sub)
		if err2 != nil {
			return nil, err2
		}
		return Instantiate(n.X.Name, arg, under), nil
	}
	return nil, errors.New("types: unsupported type expression")
}

func (c *Checker) typesOfFieldsSubst(fields []*ast.Node, subst map[string]*Type) ([]*Type, error) {
	var out []*Type
	for i := 0; i < len(fields); i++ {
		t, err := c.typeFromNodeSubst(fields[i].Type, subst)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, nil
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
		u := underlying(xt)
		var keyType, valType *Type
		oneVal := false
		if u.TKind == Slice {
			keyType, valType = IntType, u.Elem
		} else if u.TKind == Map {
			keyType, valType = u.Key, u.Elem
		} else if u.TKind == Func && len(u.In) == 1 && u.In[0].TKind == Func {
			yield := u.In[0]
			if len(yield.Out) != 1 || !Identical(yield.Out[0], BoolType) {
				return errors.New("types: cannot range over a value of type " + xt.String())
			}
			if len(yield.In) == 1 {
				oneVal = true
				valType = yield.In[0]
			} else if len(yield.In) == 2 {
				keyType, valType = yield.In[0], yield.In[1]
			} else {
				return errors.New("types: cannot range over a value of type " + xt.String())
			}
		} else {
			return errors.New("types: cannot range over a value of type " + xt.String())
		}
		if oneVal {
			if len(n.Lhs) > 1 {
				return errors.New("types: range over seq yields one value")
			}
			if len(n.Lhs) > 0 && n.Lhs[0].Kind == ast.Ident && n.Lhs[0].Name != "_" {
				c.Env[n.Lhs[0].Name] = valType
			}
		} else {
			if len(n.Lhs) > 0 && n.Lhs[0].Kind == ast.Ident && n.Lhs[0].Name != "_" {
				c.Env[n.Lhs[0].Name] = keyType
			}
			if len(n.Lhs) > 1 && n.Lhs[1].Kind == ast.Ident && n.Lhs[1].Name != "_" {
				c.Env[n.Lhs[1].Name] = valType
			}
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
	if n.Kind == ast.TypeSpec {
		return c.checkTypeSpec(n)
	}
	return errors.New("types: unsupported statement kind")
}

func (c *Checker) checkTypeSpec(d *ast.Node) error {
	if c.Types == nil {
		c.Types = make(map[string]*Type)
	}
	if c.Generics == nil {
		c.Generics = make(map[string]*ast.Node)
	}
	if len(d.Params) > 0 {
		c.Generics[d.Name] = d
		return nil
	}
	under, err := c.typeFromNode(d.Type)
	if err != nil {
		return err
	}
	if d.Type != nil && d.Type.Kind == ast.InterfaceType {
		c.Types[d.Name] = under
		return nil
	}
	c.Types[d.Name] = Named(d.Name, under)
	return nil
}

// CheckFile intern-binds every TypeSpec and attaches method sets from
// `func (T) M()` declarations onto the interned named type. Function
// bodies are not checked -- call CheckStmt on those separately, after
// CheckFile so method lookups see the interned set.
func (c *Checker) CheckFile(f *ast.Node) error {
	if f == nil || f.Kind != ast.File {
		return errors.New("types: not a file")
	}
	for i := 0; i < len(f.List); i++ {
		d := f.List[i]
		if d.Kind == ast.TypeSpec {
			if err := c.checkTypeSpec(d); err != nil {
				return err
			}
		}
	}
	for i := 0; i < len(f.List); i++ {
		d := f.List[i]
		if d.Kind != ast.FuncDecl || d.X == nil {
			continue
		}
		recv, err := c.typeFromNode(d.X.Type)
		if err != nil {
			return err
		}
		if recv.TKind == Pointer {
			recv = recv.Elem
		}
		in, err2 := c.typesOfFieldsSubst(d.Params, nil)
		if err2 != nil {
			return err2
		}
		out, err3 := c.typesOfFieldsSubst(d.Results, nil)
		if err3 != nil {
			return err3
		}
		recv.AddMethod(d.Name, FuncOf(in, out))
	}
	return nil
}
