// AST for the Go subset wasigoc accepts. See README.md's grammar section
// for exactly which constructs exist; this file is deliberately a flat,
// tagged-node tree (one Expr kind, one Stmt kind) rather than a deep class
// hierarchy -- there's no visitor double-dispatch anywhere in this
// compiler, so the extra ceremony wouldn't pay for itself.
#pragma once

#include <cstdint>
#include <memory>
#include <string>
#include <utility>
#include <vector>

namespace wasigo {

// ---- Types ----------------------------------------------------------------

enum class TypeKind {
  Named,      // int, string, bool, float64, byte, rune, any, error, or a declared name
  Pointer,    // *T
  Slice,      // []T
  Map,        // map[K]V
  Chan,       // chan T  /  chan<- T  /  <-chan T
  Func,       // func(params) results
  Array,      // [N]T
};

// Constant integer expression used as `[N]T` / `[1<<10]T` / `[Size]T`.
// Kept separate from Expr so TypeNode can own it without an Expr cycle.
struct ArrayLenExpr {
  enum class Kind { Lit, Ident, Unary, Binary };
  Kind kind = Kind::Lit;
  int64_t lit = 0;
  std::string ident;
  std::string op;
  std::unique_ptr<ArrayLenExpr> x;
  std::unique_ptr<ArrayLenExpr> y;
};

inline std::unique_ptr<ArrayLenExpr> CloneArrayLen(const ArrayLenExpr* e) {
  if (!e) return nullptr;
  auto c = std::make_unique<ArrayLenExpr>();
  c->kind = e->kind;
  c->lit = e->lit;
  c->ident = e->ident;
  c->op = e->op;
  c->x = CloneArrayLen(e->x.get());
  c->y = CloneArrayLen(e->y.get());
  return c;
}

struct TypeNode;

struct Param {
  std::string name;
  std::unique_ptr<TypeNode> type;
  bool variadic = false;
};

struct TypeNode {
  TypeKind kind;
  std::string name;                  // Named only
  std::string pkg;                   // Named only: imported package (empty = this file)
  std::unique_ptr<TypeNode> key;     // Map only
  std::unique_ptr<TypeNode> elem;    // Pointer/Slice/Map(value)/Chan/Array
  bool chan_send = true;             // Chan direction
  bool chan_recv = true;
  int64_t array_len = 0;             // Array only (when array_len_expr is null)
  std::unique_ptr<ArrayLenExpr> array_len_expr;  // named const / 1<<n
  std::vector<Param> func_params;    // Func only
  std::vector<std::unique_ptr<TypeNode>> func_results;
  bool variadic = false;
  std::vector<std::unique_ptr<TypeNode>> type_args;  // Named only: Set[int]
};

inline std::unique_ptr<TypeNode> MakeNamedType(std::string name, std::string pkg = "") {
  auto t = std::make_unique<TypeNode>();
  t->kind = TypeKind::Named;
  t->name = std::move(name);
  t->pkg = std::move(pkg);
  return t;
}

std::unique_ptr<TypeNode> CloneType(const TypeNode* t);

inline std::unique_ptr<TypeNode> CloneType(const TypeNode* t) {
  if (!t) return nullptr;
  auto c = std::make_unique<TypeNode>();
  c->kind = t->kind;
  c->name = t->name;
  c->pkg = t->pkg;
  c->key = CloneType(t->key.get());
  c->elem = CloneType(t->elem.get());
  c->chan_send = t->chan_send;
  c->chan_recv = t->chan_recv;
  c->array_len = t->array_len;
  c->array_len_expr = CloneArrayLen(t->array_len_expr.get());
  c->variadic = t->variadic;
  for (auto& p : t->func_params) {
    Param np;
    np.name = p.name;
    np.type = CloneType(p.type.get());
    np.variadic = p.variadic;
    c->func_params.push_back(std::move(np));
  }
  for (auto& r : t->func_results) c->func_results.push_back(CloneType(r.get()));
  for (auto& a : t->type_args) c->type_args.push_back(CloneType(a.get()));
  return c;
}

// ---- Expressions ------------------------------------------------------------

enum class ExprKind {
  IntLit,
  FloatLit,
  ImagLit,
  StringLit,
  BoolLit,
  Nil,
  Ident,
  Binary,
  Unary,
  Call,
  Selector,       // x.Name
  Index,          // x[i]  or  m[k]
  CompositeLit,   // T{...} or []T{...} or map[K]V{...}
  ParenExpr,
  Recv,           // <-ch
  FuncLit,        // func(params) results { body }
  SliceExpr,      // x[low:high] or x[low:high:max]
  TypeAssert,     // x.(T)
};

struct Stmt;

struct FuncLit {
  std::vector<Param> params;
  std::vector<std::unique_ptr<TypeNode>> results;
  std::vector<std::unique_ptr<Stmt>> body;
  bool variadic = false;
};

struct Expr {
  ExprKind kind;
  int line = 0;
  int col = 0;

  long long intval = 0;
  double floatval = 0;
  bool boolval = false;
  std::string strval;

  std::unique_ptr<Expr> x;
  std::unique_ptr<Expr> y;

  std::unique_ptr<Expr> callee;
  std::vector<std::unique_ptr<Expr>> args;

  std::unique_ptr<TypeNode> type;
  std::vector<std::unique_ptr<Expr>> elems;
  std::vector<std::pair<std::unique_ptr<Expr>, std::unique_ptr<Expr>>> fields;

  // SliceExpr: x[low:high:max] -- x is `x`; low/high/max may be null (omitted).
  std::unique_ptr<Expr> low;
  std::unique_ptr<Expr> high;
  std::unique_ptr<Expr> max;
  bool slice_3 = false;
  bool ellipsis = false;      // last call arg: x...
  bool type_switch = false;   // x.(type)

  std::unique_ptr<FuncLit> func_lit;
};

inline std::unique_ptr<Expr> MakeIdent(std::string name, int line = 0, int col = 0) {
  auto e = std::make_unique<Expr>();
  e->kind = ExprKind::Ident;
  e->strval = std::move(name);
  e->line = line;
  e->col = col;
  return e;
}

std::unique_ptr<Expr> CloneExpr(const Expr* e);

inline std::unique_ptr<Expr> CloneExpr(const Expr* e) {
  if (!e) return nullptr;
  auto c = std::make_unique<Expr>();
  c->kind = e->kind;
  c->line = e->line;
  c->col = e->col;
  c->intval = e->intval;
  c->floatval = e->floatval;
  c->boolval = e->boolval;
  c->strval = e->strval;
  c->x = CloneExpr(e->x.get());
  c->y = CloneExpr(e->y.get());
  c->callee = CloneExpr(e->callee.get());
  for (auto& a : e->args) c->args.push_back(CloneExpr(a.get()));
  c->type = CloneType(e->type.get());
  for (auto& el : e->elems) c->elems.push_back(CloneExpr(el.get()));
  for (auto& kv : e->fields) {
    c->fields.emplace_back(CloneExpr(kv.first.get()), CloneExpr(kv.second.get()));
  }
  c->low = CloneExpr(e->low.get());
  c->high = CloneExpr(e->high.get());
  c->max = CloneExpr(e->max.get());
  c->slice_3 = e->slice_3;
  c->ellipsis = e->ellipsis;
  c->type_switch = e->type_switch;
  return c;
}

// ---- Statements -------------------------------------------------------------

enum class StmtKind {
  Var,
  ShortVarDecl,
  Assign,
  IncDec,
  ExprStmt,
  Return,
  If,
  ForClassic,
  ForCond,
  ForInfinite,
  ForRange,
  Block,
  Break,
  Continue,
  Go,
  Defer,
  Send,            // ch <- x
  Switch,
  Select,
  Fallthrough,
  Labeled,
  Goto,
};

struct SwitchCase {
  std::vector<std::unique_ptr<Expr>> values;  // empty => default (value switch)
  std::vector<std::unique_ptr<TypeNode>> types;  // type switch; name "nil" => nil case
  std::vector<std::unique_ptr<Stmt>> body;
};

struct SelectCase {
  bool is_default = false;
  bool is_send = false;
  std::unique_ptr<Expr> chan;
  std::unique_ptr<Expr> value;           // send payload
  std::vector<std::string> recv_names;   // v  or  v, ok
  bool recv_define = false;
  bool recv_ok = false;
  std::vector<std::unique_ptr<Stmt>> body;
};

struct Stmt {
  StmtKind kind;
  int line = 0;
  int col = 0;

  std::vector<std::string> names;
  std::unique_ptr<TypeNode> var_type;

  std::vector<std::unique_ptr<Expr>> lhs;
  std::vector<std::unique_ptr<Expr>> rhs;
  std::string op;

  std::unique_ptr<Expr> cond;
  std::unique_ptr<Stmt> init;
  std::unique_ptr<Stmt> post;

  std::unique_ptr<Expr> range_expr;
  bool range_has_key = true;
  bool range_has_value = true;
  bool is_const = false;

  std::vector<std::unique_ptr<Stmt>> body;
  std::vector<std::unique_ptr<Stmt>> else_body;
  bool has_else = false;

  std::vector<SwitchCase> cases;
  std::vector<SelectCase> sel_cases;
  bool type_switch = false;  // switch x := i.(type)
};

// ---- Top-level declarations --------------------------------------------------

struct FieldDecl {
  std::string name;
  std::unique_ptr<TypeNode> type;
  std::string tag;  // raw `json:"name"` string, empty if omitted
  bool embedded = false;
};

struct StructDecl {
  std::string name;
  std::vector<std::string> type_params;  // [T any] => "T"
  std::vector<FieldDecl> fields;
};

struct MethodSig {
  std::string name;
  std::vector<Param> params;
  std::vector<std::unique_ptr<TypeNode>> results;
};

struct InterfaceDecl {
  std::string name;
  std::vector<MethodSig> methods;
  std::vector<std::string> embedded;  // other interface names
};

struct FuncDecl {
  std::string name;

  bool has_receiver = false;
  std::string receiver_name;
  std::string receiver_type;
  bool receiver_is_pointer = false;

  std::vector<std::string> type_params;  // [T any] => "T"
  std::vector<Param> params;
  std::vector<std::unique_ptr<TypeNode>> results;
  std::vector<std::string> result_names;
  bool variadic = false;

  std::vector<std::unique_ptr<Stmt>> body;
};

struct GlobalVarDecl {
  std::string name;
  std::unique_ptr<TypeNode> type;
  std::unique_ptr<Expr> init;
  bool is_const = false;
  int iota_value = 0;
};

// `type Name T` (defined type) or `type Name = T` (alias). Defined types
// with a method set become a distinct C++ struct (object-type identity);
// aliases and method-less defined types stay `using`.
struct TypeAlias {
  std::string name;
  std::unique_ptr<TypeNode> type;
  bool is_alias_eq = false;  // true for `type Name = T`
  std::vector<std::string> type_params;
};

struct ImportSpec {
  std::string path;
  std::string local;  // empty = default package name; "_" = blank (init only)
};

struct File {
  std::string path;
  std::string package_name;
  std::vector<std::string> imports;  // paths, for the loader
  std::vector<ImportSpec> import_specs;
  std::vector<StructDecl> structs;
  std::vector<InterfaceDecl> interfaces;
  std::vector<TypeAlias> aliases;
  std::vector<GlobalVarDecl> globals;
  std::vector<FuncDecl> funcs;
};

}  // namespace wasigo
