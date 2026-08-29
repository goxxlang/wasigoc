#include "parser.h"

#include <memory>
#include <utility>

namespace wasigo {

namespace {

std::unique_ptr<Stmt> MakeStmt(StmtKind k) {
  auto s = std::make_unique<Stmt>();
  s->kind = k;
  return s;
}

std::unique_ptr<Expr> MakeLit(ExprKind k) {
  auto e = std::make_unique<Expr>();
  e->kind = k;
  return e;
}

int BinaryPrec(TokKind k) {
  switch (k) {
    case TokKind::OrOr:
      return 1;
    case TokKind::AndAnd:
      return 2;
    case TokKind::Eq:
    case TokKind::Neq:
    case TokKind::Lt:
    case TokKind::Leq:
    case TokKind::Gt:
    case TokKind::Geq:
      return 3;
    case TokKind::Plus:
    case TokKind::Minus:
    case TokKind::Pipe:
    case TokKind::Caret:
      return 4;
    case TokKind::Star:
    case TokKind::Slash:
    case TokKind::Percent:
    case TokKind::Shl:
    case TokKind::Shr:
    case TokKind::Amp:
    case TokKind::AndNot:
      return 5;
    default:
      return 0;
  }
}

bool IsAssignOpTok(TokKind k) {
  switch (k) {
    case TokKind::Assign:
    case TokKind::PlusEq:
    case TokKind::MinusEq:
    case TokKind::StarEq:
    case TokKind::SlashEq:
    case TokKind::PercentEq:
    case TokKind::AmpEq:
    case TokKind::PipeEq:
    case TokKind::CaretEq:
    case TokKind::AndNotEq:
    case TokKind::ShlEq:
    case TokKind::ShrEq:
      return true;
    default:
      return false;
  }
}

class ParserImpl {
 public:
  explicit ParserImpl(const std::vector<Token>& toks) : toks_(toks) {}

  File ParseFile() {
    File f;
    Expect(TokKind::KwPackage);
    f.package_name = ExpectIdent();
    ExpectSemi();

    while (Check(TokKind::KwImport)) {
      Advance();
      if (Accept(TokKind::LParen)) {
        SkipSemis();
        while (!Check(TokKind::RParen)) {
          ParseImportSpec(f);
          SkipSemis();
        }
        Expect(TokKind::RParen);
      } else {
        ParseImportSpec(f);
      }
      ExpectSemi();
    }

    while (!Check(TokKind::Eof)) {
      if (Check(TokKind::KwFunc)) {
        f.funcs.push_back(ParseFuncDecl());
      } else if (Check(TokKind::KwType)) {
        ParseTypeDecl(f);
      } else if (Check(TokKind::KwVar) || Check(TokKind::KwConst)) {
        ParseGlobalVarGroup(f.globals);
      } else {
        Fail("expected a top-level declaration ('func', 'type', 'var', or "
             "'const')");
      }
      SkipSemis();
    }
    return f;
  }

 private:
  const std::vector<Token>& toks_;
  size_t pos_ = 0;
  bool allow_composite_lit_ = true;

  const Token& Cur() const { return toks_[pos_]; }
  bool Check(TokKind k) const { return Cur().kind == k; }
  bool NextIs(TokKind k) const {
    return pos_ + 1 < toks_.size() && toks_[pos_ + 1].kind == k;
  }

  void Advance() {
    if (pos_ + 1 < toks_.size()) pos_++;
  }

  bool Accept(TokKind k) {
    if (Check(k)) {
      Advance();
      return true;
    }
    return false;
  }

  void Expect(TokKind k) {
    if (!Check(k)) {
      Fail(std::string("expected ") + TokKindName(k) + " but found " +
           TokKindName(Cur().kind));
    }
    Advance();
  }

  // Go's ASI already turns a source-level newline into a real Semi token, so
  // "expect a statement terminator" just means "expect that token".
  void ExpectSemi() { Expect(TokKind::Semi); }
  void SkipSemis() {
    while (Accept(TokKind::Semi)) {
    }
  }

  [[noreturn]] void Fail(const std::string& msg) {
    throw ParseError("line " + std::to_string(Cur().line) + ": " + msg);
  }

  std::string ExpectIdent() {
    if (!Check(TokKind::Ident)) {
      Fail("expected an identifier but found " + std::string(TokKindName(Cur().kind)));
    }
    std::string name = Cur().text;
    Advance();
    return name;
  }

  std::string ExpectString() {
    if (!Check(TokKind::StringLit)) {
      Fail("expected a string literal but found " +
           std::string(TokKindName(Cur().kind)));
    }
    std::string s = Cur().text;
    Advance();
    return s;
  }

  void ParseImportSpec(File& f) {
    ImportSpec spec;
    if (Check(TokKind::Ident)) {
      spec.local = ExpectIdent();
    } else if (Accept(TokKind::Dot)) {
      Fail("dot imports (import . \"path\") are not supported");
    }
    spec.path = ExpectString();
    f.import_specs.push_back(spec);
    f.imports.push_back(spec.path);
  }

  // ---- Types ------------------------------------------------------------

  static bool LooksLikeTypeStart(TokKind k) {
    switch (k) {
      case TokKind::Star:
      case TokKind::LBracket:
      case TokKind::KwMap:
      case TokKind::KwChan:
      case TokKind::KwFunc:
      case TokKind::KwInterface:
      case TokKind::Arrow:
      case TokKind::Ellipsis:
        return true;
      default:
        return false;
    }
  }

  bool ParamListIsNamed() const {
    int depth = 0;
    for (size_t i = pos_; i < toks_.size(); ++i) {
      TokKind k = toks_[i].kind;
      if (k == TokKind::LParen || k == TokKind::LBracket) {
        depth++;
      } else if (k == TokKind::RParen || k == TokKind::RBracket) {
        if (depth == 0 && k == TokKind::RParen) break;
        if (depth > 0) depth--;
      } else if (depth == 0 && k == TokKind::Ident) {
        TokKind n = (i + 1 < toks_.size()) ? toks_[i + 1].kind : TokKind::Eof;
        if (n == TokKind::Ident || LooksLikeTypeStart(n)) return true;
      }
    }
    return false;
  }

  std::unique_ptr<TypeNode> ParseType() {
    if (Accept(TokKind::Star)) {
      auto t = std::make_unique<TypeNode>();
      t->kind = TypeKind::Pointer;
      t->elem = ParseType();
      return t;
    }
    if (Accept(TokKind::Arrow)) {
      Expect(TokKind::KwChan);
      auto t = std::make_unique<TypeNode>();
      t->kind = TypeKind::Chan;
      t->chan_send = false;
      t->chan_recv = true;
      t->elem = ParseType();
      return t;
    }
    if (Accept(TokKind::KwChan)) {
      auto t = std::make_unique<TypeNode>();
      t->kind = TypeKind::Chan;
      t->chan_send = true;
      t->chan_recv = true;
      if (Accept(TokKind::Arrow)) t->chan_recv = false;
      t->elem = ParseType();
      return t;
    }
    if (Accept(TokKind::KwFunc)) {
      auto t = std::make_unique<TypeNode>();
      t->kind = TypeKind::Func;
      Expect(TokKind::LParen);
      t->func_params = ParseParamList(&t->variadic);
      Expect(TokKind::RParen);
      if (Accept(TokKind::LParen)) {
        if (!Accept(TokKind::RParen)) {
          for (;;) {
            std::vector<std::string> dummy;
            ParseOneResult(t->func_results, dummy);
            if (!Accept(TokKind::Comma)) break;
            if (Check(TokKind::RParen)) break;
          }
          Expect(TokKind::RParen);
        }
      } else if (LooksLikeTypeStart(Cur().kind) || Check(TokKind::Ident)) {
        t->func_results.push_back(ParseType());
      }
      return t;
    }
    if (Accept(TokKind::KwInterface)) {
      Expect(TokKind::LBrace);
      SkipSemis();
      Expect(TokKind::RBrace);
      return MakeNamedType("any");
    }
    if (Accept(TokKind::LBracket)) {
      if (Accept(TokKind::RBracket)) {
        auto t = std::make_unique<TypeNode>();
        t->kind = TypeKind::Slice;
        t->elem = ParseType();
        return t;
      }
      if (!Check(TokKind::IntLit)) Fail("array length must be an integer literal");
      int64_t n = Cur().intval;
      Advance();
      Expect(TokKind::RBracket);
      auto t = std::make_unique<TypeNode>();
      t->kind = TypeKind::Array;
      t->array_len = n;
      t->elem = ParseType();
      return t;
    }
    if (Accept(TokKind::KwMap)) {
      Expect(TokKind::LBracket);
      auto key = ParseType();
      Expect(TokKind::RBracket);
      auto t = std::make_unique<TypeNode>();
      t->kind = TypeKind::Map;
      t->key = std::move(key);
      t->elem = ParseType();
      return t;
    }
    std::string first = ExpectIdent();
    if (Accept(TokKind::Dot)) {
      return MakeNamedType(ExpectIdent(), first);
    }
    return MakeNamedType(std::move(first));
  }

  // ---- Expressions --------------------------------------------------------

  std::vector<std::unique_ptr<Expr>> ParseExprList() {
    std::vector<std::unique_ptr<Expr>> v;
    v.push_back(ParseExpr());
    while (Accept(TokKind::Comma)) v.push_back(ParseExpr());
    return v;
  }

  std::unique_ptr<Expr> ParseExpr() { return ParseBinary(1); }

  std::unique_ptr<Expr> ParseBinary(int min_prec) {
    auto lhs = ParseUnary();
    for (;;) {
      int prec = BinaryPrec(Cur().kind);
      if (prec == 0 || prec < min_prec) break;
      std::string op = Cur().text;
      Advance();
      auto rhs = ParseBinary(prec + 1);
      auto e = MakeLit(ExprKind::Binary);
      e->strval = op;
      e->x = std::move(lhs);
      e->y = std::move(rhs);
      lhs = std::move(e);
    }
    return lhs;
  }

  std::unique_ptr<Expr> ParseUnary() {
    if (Check(TokKind::Arrow)) {
      Advance();
      auto e = MakeLit(ExprKind::Recv);
      e->x = ParseUnary();
      return e;
    }
    if (Check(TokKind::Not) || Check(TokKind::Minus) || Check(TokKind::Amp) ||
        Check(TokKind::Star) || Check(TokKind::Plus) || Check(TokKind::Caret)) {
      std::string op = Cur().text;
      Advance();
      auto e = MakeLit(ExprKind::Unary);
      e->strval = op;
      e->x = ParseUnary();
      return e;
    }
    return ParsePostfix();
  }

  std::unique_ptr<Expr> ParsePostfix() {
    auto e = ParsePrimary();
    for (;;) {
      if (Accept(TokKind::Dot)) {
        if (Accept(TokKind::LParen)) {
          auto ta = MakeLit(ExprKind::TypeAssert);
          ta->x = std::move(e);
          if (Accept(TokKind::KwType)) {
            ta->type_switch = true;
          } else {
            ta->type = ParseType();
          }
          Expect(TokKind::RParen);
          e = std::move(ta);
          continue;
        }
        std::string name = ExpectIdent();
        auto sel = MakeLit(ExprKind::Selector);
        sel->x = std::move(e);
        sel->strval = name;
        e = std::move(sel);
        if (allow_composite_lit_ && Check(TokKind::LBrace) && e->x &&
            e->x->kind == ExprKind::Ident) {
          Advance();
          e = ParseCompositeLitBody(MakeNamedType(name, e->x->strval));
          continue;
        }
      } else if (Accept(TokKind::LParen)) {
        auto call = MakeLit(ExprKind::Call);
        call->callee = std::move(e);
        if (!Check(TokKind::RParen)) {
          auto arg = ParseExpr();
          if (Accept(TokKind::Ellipsis)) arg->ellipsis = true;
          call->args.push_back(std::move(arg));
          while (Accept(TokKind::Comma)) {
            if (Check(TokKind::RParen)) break;
            arg = ParseExpr();
            if (Accept(TokKind::Ellipsis)) arg->ellipsis = true;
            call->args.push_back(std::move(arg));
          }
        }
        Expect(TokKind::RParen);
        e = std::move(call);
      } else if (Accept(TokKind::LBracket)) {
        if (Accept(TokKind::Colon)) {
          auto sl = MakeLit(ExprKind::SliceExpr);
          sl->x = std::move(e);
          if (!Check(TokKind::Colon) && !Check(TokKind::RBracket)) sl->high = ParseExpr();
          if (Accept(TokKind::Colon)) {
            sl->slice_3 = true;
            if (!Check(TokKind::RBracket)) sl->max = ParseExpr();
          }
          Expect(TokKind::RBracket);
          e = std::move(sl);
        } else {
          auto first = ParseExpr();
          if (Accept(TokKind::Colon)) {
            auto sl = MakeLit(ExprKind::SliceExpr);
            sl->x = std::move(e);
            sl->low = std::move(first);
            if (!Check(TokKind::Colon) && !Check(TokKind::RBracket)) sl->high = ParseExpr();
            if (Accept(TokKind::Colon)) {
              sl->slice_3 = true;
              if (!Check(TokKind::RBracket)) sl->max = ParseExpr();
            }
            Expect(TokKind::RBracket);
            e = std::move(sl);
          } else {
            auto idx = MakeLit(ExprKind::Index);
            idx->x = std::move(e);
            idx->y = std::move(first);
            Expect(TokKind::RBracket);
            e = std::move(idx);
          }
        }
      } else {
        break;
      }
    }
    return e;
  }

  std::unique_ptr<Expr> ParseCompositeLitBody(std::unique_ptr<TypeNode> type) {
    // '{' already consumed by the caller.
    auto lit = MakeLit(ExprKind::CompositeLit);
    lit->type = std::move(type);
    bool saved = allow_composite_lit_;
    allow_composite_lit_ = true;
    // A bare `{...}` element elides its own type, reusing the outer
    // literal's element type -- e.g. `[][]int{{1, 2}, {3, 4}}`'s inner
    // `{1, 2}`/`{3, 4}` instead of the fully-spelled `[]int{1, 2}`, or
    // `map[string][]int{"a": {1, 2}}`'s value. Real Go allows this at any
    // composite-literal nesting depth; this parser previously had no case
    // for a bare LBrace in ParsePrimary at all, so it simply couldn't parse
    // this shape -- found building crypto/des's S-boxes.
    //
    // `val_elem_type` (used after a `:`) is unambiguous for both Map
    // (value type) and a keyed Slice/Array element -- both store it in
    // `type->elem`. `first_elem_type` (used for the part *before* an as-yet-
    // unseen `:`) is only ever safe for Slice/Array's positional element:
    // for Map that position is the *key*, which needs `type->key`, not
    // `type->elem` -- eliding the type of a composite map key is rare
    // enough in real Go to not be worth resolving that ambiguity here.
    const TypeNode* val_elem_type = lit->type ? lit->type->elem.get() : nullptr;
    const TypeNode* first_elem_type =
        (lit->type && (lit->type->kind == TypeKind::Slice || lit->type->kind == TypeKind::Array))
            ? lit->type->elem.get()
            : nullptr;
    if (!Check(TokKind::RBrace)) {
      for (;;) {
        std::unique_ptr<Expr> first;
        if (first_elem_type && Check(TokKind::LBrace)) {
          Advance();
          first = ParseCompositeLitBody(CloneType(first_elem_type));
        } else {
          first = ParseExpr();
        }
        if (Accept(TokKind::Colon)) {
          std::unique_ptr<Expr> val;
          if (val_elem_type && Check(TokKind::LBrace)) {
            Advance();
            val = ParseCompositeLitBody(CloneType(val_elem_type));
          } else {
            val = ParseExpr();
          }
          lit->fields.emplace_back(std::move(first), std::move(val));
        } else {
          lit->elems.push_back(std::move(first));
        }
        if (!Accept(TokKind::Comma)) break;
        if (Check(TokKind::RBrace)) break;  // trailing comma
      }
    }
    allow_composite_lit_ = saved;
    Expect(TokKind::RBrace);
    return lit;
  }

  std::unique_ptr<Expr> ParsePrimary() {
    const Token& t = Cur();
    switch (t.kind) {
      case TokKind::IntLit: {
        Advance();
        auto e = MakeLit(ExprKind::IntLit);
        e->intval = t.intval;
        return e;
      }
      case TokKind::RuneLit: {
        Advance();
        auto e = MakeLit(ExprKind::IntLit);
        e->intval = t.intval;
        return e;
      }
      case TokKind::FloatLit: {
        Advance();
        auto e = MakeLit(ExprKind::FloatLit);
        e->floatval = t.floatval;
        return e;
      }
      case TokKind::StringLit: {
        Advance();
        auto e = MakeLit(ExprKind::StringLit);
        e->strval = t.text;
        return e;
      }
      case TokKind::KwTrue: {
        Advance();
        auto e = MakeLit(ExprKind::BoolLit);
        e->boolval = true;
        return e;
      }
      case TokKind::KwFalse: {
        Advance();
        auto e = MakeLit(ExprKind::BoolLit);
        e->boolval = false;
        return e;
      }
      case TokKind::KwNil:
        Advance();
        return MakeLit(ExprKind::Nil);
      case TokKind::LParen: {
        Advance();
        bool saved = allow_composite_lit_;
        allow_composite_lit_ = true;
        auto inner = ParseExpr();
        allow_composite_lit_ = saved;
        Expect(TokKind::RParen);
        auto e = MakeLit(ExprKind::ParenExpr);
        e->x = std::move(inner);
        return e;
      }
      case TokKind::LBracket:
      case TokKind::KwMap:
      case TokKind::KwChan:
      case TokKind::KwInterface: {
        auto type = ParseType();
        if (allow_composite_lit_ && Check(TokKind::LBrace)) {
          Advance();
          return ParseCompositeLitBody(std::move(type));
        }
        auto e = MakeLit(ExprKind::CompositeLit);
        e->type = std::move(type);
        return e;
      }
      case TokKind::KwFunc:
        return ParseFuncLit();
      case TokKind::Ident: {
        std::string name = t.text;
        Advance();
        if (allow_composite_lit_ && Check(TokKind::LBrace)) {
          Advance();
          return ParseCompositeLitBody(MakeNamedType(name));
        }
        return MakeIdent(name);
      }
      default:
        Fail("expected an expression but found " +
             std::string(TokKindName(t.kind)));
    }
  }

  std::unique_ptr<Expr> ParseFuncLit() {
    Expect(TokKind::KwFunc);
    auto fl = std::make_unique<FuncLit>();
    Expect(TokKind::LParen);
    fl->params = ParseParamList(&fl->variadic);
    Expect(TokKind::RParen);
    if (Accept(TokKind::LParen)) {
      if (!Accept(TokKind::RParen)) {
        std::vector<std::string> dummy;
        for (;;) {
          ParseOneResult(fl->results, dummy);
          if (!Accept(TokKind::Comma)) break;
          if (Check(TokKind::RParen)) break;
        }
        Expect(TokKind::RParen);
      }
    } else if (LooksLikeTypeStart(Cur().kind) || Check(TokKind::Ident)) {
      fl->results.push_back(ParseType());
    }
    fl->body = ParseBlock();
    auto e = MakeLit(ExprKind::FuncLit);
    e->func_lit = std::move(fl);
    return e;
  }

  // ---- Statements ---------------------------------------------------------

  std::vector<std::unique_ptr<Stmt>> ParseBlock() {
    Expect(TokKind::LBrace);
    std::vector<std::unique_ptr<Stmt>> stmts;
    for (;;) {
      SkipSemis();
      if (Check(TokKind::RBrace) || Check(TokKind::Eof)) break;
      ParseStmtInto(stmts);
      SkipSemis();
    }
    Expect(TokKind::RBrace);
    return stmts;
  }

  void ParseStmtInto(std::vector<std::unique_ptr<Stmt>>& out) {
    if (Check(TokKind::KwVar) || Check(TokKind::KwConst)) {
      ParseVarGroupInto(out);
      return;
    }
    if (Check(TokKind::Ident) && NextIs(TokKind::Colon)) {
      auto s = MakeStmt(StmtKind::Labeled);
      s->names.push_back(ExpectIdent());
      Expect(TokKind::Colon);
      SkipSemis();
      s->body.push_back(ParseSingleStmt());
      out.push_back(std::move(s));
      return;
    }
    out.push_back(ParseSingleStmt());
  }

  void ParseVarGroupInto(std::vector<std::unique_ptr<Stmt>>& out) {
    Advance();  // 'var' or 'const'
    if (Accept(TokKind::LParen)) {
      SkipSemis();
      while (!Check(TokKind::RParen)) {
        out.push_back(ParseOneVarSpec());
        SkipSemis();
      }
      Expect(TokKind::RParen);
    } else {
      out.push_back(ParseOneVarSpec());
    }
  }

  std::unique_ptr<Stmt> ParseOneVarSpec() {
    std::vector<std::string> names;
    names.push_back(ExpectIdent());
    while (Accept(TokKind::Comma)) names.push_back(ExpectIdent());

    std::unique_ptr<TypeNode> type;
    if (!Check(TokKind::Assign) && !Check(TokKind::Semi) &&
        !Check(TokKind::RParen)) {
      type = ParseType();
    }

    std::vector<std::unique_ptr<Expr>> inits;
    if (Accept(TokKind::Assign)) {
      inits.push_back(ParseExpr());
      while (Accept(TokKind::Comma)) inits.push_back(ParseExpr());
    }

    auto s = MakeStmt(StmtKind::Var);
    s->names = std::move(names);
    s->var_type = std::move(type);
    s->rhs = std::move(inits);
    return s;
  }

  void ParseGlobalVarGroup(std::vector<GlobalVarDecl>& out) {
    bool is_const = Check(TokKind::KwConst);
    Advance();  // 'var' or 'const'
    if (Accept(TokKind::LParen)) {
      SkipSemis();
      int iota = 0;
      std::unique_ptr<Expr> prev_init;
      std::unique_ptr<TypeNode> prev_type;
      while (!Check(TokKind::RParen)) {
        ParseOneGlobalVarSpec(out, is_const, iota, prev_init, prev_type);
        iota++;
        SkipSemis();
      }
      Expect(TokKind::RParen);
    } else {
      std::unique_ptr<Expr> prev_init;
      std::unique_ptr<TypeNode> prev_type;
      int iota = 0;
      ParseOneGlobalVarSpec(out, is_const, iota, prev_init, prev_type);
    }
    ExpectSemi();
  }

  void ParseOneGlobalVarSpec(std::vector<GlobalVarDecl>& out, bool is_const, int iota,
                             std::unique_ptr<Expr>& prev_init,
                             std::unique_ptr<TypeNode>& prev_type) {
    std::vector<std::string> names;
    names.push_back(ExpectIdent());
    while (Accept(TokKind::Comma)) names.push_back(ExpectIdent());

    std::unique_ptr<TypeNode> type;
    if (!Check(TokKind::Assign) && !Check(TokKind::Semi) && !Check(TokKind::RParen)) {
      type = ParseType();
      prev_type = CloneType(type.get());
    } else if (is_const && !type && prev_type) {
      type = CloneType(prev_type.get());
    }

    std::vector<std::unique_ptr<Expr>> inits;
    if (Accept(TokKind::Assign)) {
      inits.push_back(ParseExpr());
      while (Accept(TokKind::Comma)) inits.push_back(ParseExpr());
      if (!inits.empty()) prev_init = CloneExpr(inits[0].get());
    } else if (is_const && prev_init) {
      inits.push_back(CloneExpr(prev_init.get()));
    }

    for (size_t i = 0; i < names.size(); ++i) {
      GlobalVarDecl g;
      g.name = names[i];
      g.type = CloneType(type.get());
      g.init = i < inits.size() ? std::move(inits[i]) : nullptr;
      g.is_const = is_const;
      g.iota_value = iota;
      out.push_back(std::move(g));
    }
  }

  std::string RequireIdentName(const Expr* e) {
    if (e->kind != ExprKind::Ident) {
      Fail("expected a plain identifier on the left of ':='");
    }
    return e->strval;
  }

  // Handles short var decl, plain/compound assignment, ++/--, and bare
  // expression statements -- everything that can start with an expression.
  std::unique_ptr<Stmt> ParseSimpleStmt() {
    std::vector<std::unique_ptr<Expr>> lhs;
    lhs.push_back(ParseExpr());
    while (Accept(TokKind::Comma)) lhs.push_back(ParseExpr());

    if (Accept(TokKind::Define)) {
      auto s = MakeStmt(StmtKind::ShortVarDecl);
      for (auto& e : lhs) s->names.push_back(RequireIdentName(e.get()));
      s->rhs = ParseExprList();
      return s;
    }
    if (Accept(TokKind::Arrow)) {
      if (lhs.size() != 1) Fail("send statement has one channel on the left of '<-'");
      auto s = MakeStmt(StmtKind::Send);
      s->lhs = std::move(lhs);
      s->rhs.push_back(ParseExpr());
      return s;
    }
    if (IsAssignOpTok(Cur().kind)) {
      std::string op = Cur().text;
      Advance();
      auto s = MakeStmt(StmtKind::Assign);
      s->op = op;
      s->lhs = std::move(lhs);
      s->rhs = ParseExprList();
      return s;
    }
    if (lhs.size() != 1) {
      Fail("expected ':=' or '=' after a comma-separated expression list");
    }
    if (Accept(TokKind::PlusPlus)) {
      auto s = MakeStmt(StmtKind::IncDec);
      s->op = "++";
      s->lhs.push_back(std::move(lhs[0]));
      return s;
    }
    if (Accept(TokKind::MinusMinus)) {
      auto s = MakeStmt(StmtKind::IncDec);
      s->op = "--";
      s->lhs.push_back(std::move(lhs[0]));
      return s;
    }
    auto s = MakeStmt(StmtKind::ExprStmt);
    s->lhs.push_back(std::move(lhs[0]));
    return s;
  }

  std::unique_ptr<Stmt> ParseSingleStmt() {
    switch (Cur().kind) {
      case TokKind::KwIf:
        return ParseIfStmt();
      case TokKind::KwFor:
        return ParseForStmt();
      case TokKind::KwReturn:
        return ParseReturnStmt();
      case TokKind::KwBreak: {
        Advance();
        auto s = MakeStmt(StmtKind::Break);
        if (Check(TokKind::Ident)) s->names.push_back(ExpectIdent());
        return s;
      }
      case TokKind::KwContinue: {
        Advance();
        auto s = MakeStmt(StmtKind::Continue);
        if (Check(TokKind::Ident)) s->names.push_back(ExpectIdent());
        return s;
      }
      case TokKind::KwGoto: {
        Advance();
        auto s = MakeStmt(StmtKind::Goto);
        s->names.push_back(ExpectIdent());
        return s;
      }
      case TokKind::KwFallthrough:
        Advance();
        return MakeStmt(StmtKind::Fallthrough);
      case TokKind::KwGo: {
        Advance();
        auto s = MakeStmt(StmtKind::Go);
        s->lhs.push_back(ParseExpr());
        return s;
      }
      case TokKind::KwDefer: {
        Advance();
        auto s = MakeStmt(StmtKind::Defer);
        s->lhs.push_back(ParseExpr());
        return s;
      }
      case TokKind::KwSwitch:
        return ParseSwitchStmt();
      case TokKind::KwSelect:
        return ParseSelectStmt();
      case TokKind::LBrace: {
        auto s = MakeStmt(StmtKind::Block);
        s->body = ParseBlock();
        return s;
      }
      default:
        return ParseSimpleStmt();
    }
  }

  std::unique_ptr<Stmt> ParseIfStmt() {
    Expect(TokKind::KwIf);
    bool saved = allow_composite_lit_;
    allow_composite_lit_ = false;
    std::unique_ptr<Stmt> init;
    // `if SimpleStmt; Condition { ... }` -- the same optional-init shape
    // `for` has, most commonly seen with a map's comma-ok index:
    // `if v, ok := m[k]; ok { ... }`.
    if (HeaderHasTopLevelSemi()) {
      init = ParseSimpleStmt();
      Expect(TokKind::Semi);
    }
    auto cond = ParseExpr();
    allow_composite_lit_ = saved;
    auto body = ParseBlock();
    auto s = MakeStmt(StmtKind::If);
    s->init = std::move(init);
    s->cond = std::move(cond);
    s->body = std::move(body);
    if (Accept(TokKind::KwElse)) {
      s->has_else = true;
      if (Check(TokKind::KwIf)) {
        s->else_body.push_back(ParseIfStmt());
      } else {
        s->else_body = ParseBlock();
      }
    }
    return s;
  }

  std::unique_ptr<Stmt> ParseReturnStmt() {
    Expect(TokKind::KwReturn);
    auto s = MakeStmt(StmtKind::Return);
    if (!Check(TokKind::Semi) && !Check(TokKind::RBrace)) {
      s->rhs = ParseExprList();
    }
    return s;
  }

  // Scans forward from the current position (without consuming) to decide
  // which of the three `for` header shapes we're looking at, tracking
  // paren/bracket depth so a nested call/index's own ';' or '{' can't be
  // mistaken for the loop header's. Doesn't track brace depth -- a composite
  // literal in a `for` header would confuse this, but real Go code essentially
  // never writes one there.
  bool HeaderHasRange() const {
    int depth = 0;
    for (size_t i = pos_; i < toks_.size(); ++i) {
      TokKind k = toks_[i].kind;
      if (k == TokKind::LParen || k == TokKind::LBracket) {
        depth++;
      } else if (k == TokKind::RParen || k == TokKind::RBracket) {
        depth--;
      } else if (depth == 0) {
        if (k == TokKind::LBrace || k == TokKind::Semi) return false;
        if (k == TokKind::KwRange) return true;
      }
    }
    return false;
  }

  bool HeaderHasDefine() const {
    int depth = 0;
    for (size_t i = pos_; i < toks_.size(); ++i) {
      TokKind k = toks_[i].kind;
      if (k == TokKind::LParen || k == TokKind::LBracket) {
        depth++;
      } else if (k == TokKind::RParen || k == TokKind::RBracket) {
        depth--;
      } else if (depth == 0) {
        if (k == TokKind::LBrace || k == TokKind::Semi) return false;
        if (k == TokKind::Define) return true;
      }
    }
    return false;
  }

  bool HeaderHasTopLevelSemi() const {
    int depth = 0;
    for (size_t i = pos_; i < toks_.size(); ++i) {
      TokKind k = toks_[i].kind;
      if (k == TokKind::LParen || k == TokKind::LBracket) {
        depth++;
      } else if (k == TokKind::RParen || k == TokKind::RBracket) {
        depth--;
      } else if (depth == 0) {
        if (k == TokKind::LBrace) return false;
        if (k == TokKind::Semi) return true;
      }
    }
    return false;
  }

  std::unique_ptr<Stmt> ParseForStmt() {
    Expect(TokKind::KwFor);

    if (Check(TokKind::LBrace)) {
      auto s = MakeStmt(StmtKind::ForInfinite);
      s->body = ParseBlock();
      return s;
    }

    if (HeaderHasRange()) {
      std::vector<std::string> names;
      bool declares = false;
      if (!Check(TokKind::KwRange)) {
        names.push_back(ExpectIdent());
        if (Accept(TokKind::Comma)) names.push_back(ExpectIdent());
        if (Accept(TokKind::Define)) {
          declares = true;
        } else {
          Expect(TokKind::Assign);
        }
      }
      Expect(TokKind::KwRange);
      bool saved = allow_composite_lit_;
      allow_composite_lit_ = false;
      auto rexpr = ParseExpr();
      allow_composite_lit_ = saved;
      auto body = ParseBlock();

      auto s = MakeStmt(StmtKind::ForRange);
      s->range_expr = std::move(rexpr);
      s->body = std::move(body);
      (void)declares;
      if (names.empty()) {
        s->range_has_key = false;
        s->range_has_value = false;
      } else if (names.size() == 1) {
        s->names = names;
        s->range_has_key = names[0] != "_";
        s->range_has_value = false;
      } else {
        s->names = names;
        s->range_has_key = names[0] != "_";
        s->range_has_value = names[1] != "_";
      }
      return s;
    }

    if (!HeaderHasTopLevelSemi()) {
      bool saved = allow_composite_lit_;
      allow_composite_lit_ = false;
      auto cond = ParseExpr();
      allow_composite_lit_ = saved;
      auto body = ParseBlock();
      auto s = MakeStmt(StmtKind::ForCond);
      s->cond = std::move(cond);
      s->body = std::move(body);
      return s;
    }

    // Composite literals are ambiguous with the loop body's opening '{'
    // across the *entire* classic-for header (init, cond, and post) --
    // not just cond -- e.g. `for p := head; p != nil; p = p.next { ... }`
    // would otherwise have its post-clause's `p.next` misparsed as the
    // start of a `p.next{...}` composite literal swallowing the body.
    bool saved_header = allow_composite_lit_;
    allow_composite_lit_ = false;

    std::unique_ptr<Stmt> init;
    if (!Check(TokKind::Semi)) init = ParseSimpleStmt();
    Expect(TokKind::Semi);

    std::unique_ptr<Expr> cond;
    if (!Check(TokKind::Semi)) {
      cond = ParseExpr();
    }
    Expect(TokKind::Semi);

    std::unique_ptr<Stmt> post;
    if (!Check(TokKind::LBrace)) post = ParseSimpleStmt();
    allow_composite_lit_ = saved_header;

    auto body = ParseBlock();
    auto s = MakeStmt(StmtKind::ForClassic);
    s->init = std::move(init);
    s->cond = std::move(cond);
    s->post = std::move(post);
    s->body = std::move(body);
    return s;
  }

  // ---- Top-level declarations ---------------------------------------------

  std::vector<std::unique_ptr<Stmt>> ParseCaseBody() {
    std::vector<std::unique_ptr<Stmt>> stmts;
    for (;;) {
      SkipSemis();
      if (Check(TokKind::KwCase) || Check(TokKind::KwDefault) ||
          Check(TokKind::RBrace) || Check(TokKind::Eof)) {
        break;
      }
      ParseStmtInto(stmts);
      SkipSemis();
    }
    return stmts;
  }

  std::unique_ptr<Stmt> ParseSwitchStmt() {
    Expect(TokKind::KwSwitch);
    bool saved = allow_composite_lit_;
    allow_composite_lit_ = false;
    std::unique_ptr<Stmt> init;
    std::unique_ptr<Expr> tag;
    if (!Check(TokKind::LBrace)) {
      if (HeaderHasTopLevelSemi()) {
        init = ParseSimpleStmt();
        Expect(TokKind::Semi);
        if (!Check(TokKind::LBrace)) tag = ParseExpr();
      } else if (HeaderHasDefine()) {
        init = ParseSimpleStmt();
      } else {
        tag = ParseExpr();
      }
    }
    allow_composite_lit_ = saved;
    Expect(TokKind::LBrace);
    auto s = MakeStmt(StmtKind::Switch);
    if (init && init->kind == StmtKind::ShortVarDecl && init->rhs.size() == 1 &&
        init->rhs[0] && init->rhs[0]->kind == ExprKind::TypeAssert &&
        init->rhs[0]->type_switch) {
      s->type_switch = true;
      s->names = init->names;
      s->cond = std::move(init->rhs[0]->x);
      init.reset();
    } else if (tag && tag->kind == ExprKind::TypeAssert && tag->type_switch) {
      s->type_switch = true;
      s->cond = std::move(tag->x);
      tag.reset();
    }
    s->init = std::move(init);
    if (!s->cond) s->cond = std::move(tag);
    SkipSemis();
    while (!Check(TokKind::RBrace) && !Check(TokKind::Eof)) {
      SwitchCase c;
      if (Accept(TokKind::KwDefault)) {
        Expect(TokKind::Colon);
      } else {
        Expect(TokKind::KwCase);
        if (s->type_switch) {
          for (;;) {
            if (Accept(TokKind::KwNil)) {
              c.types.push_back(MakeNamedType("nil"));
            } else {
              c.types.push_back(ParseType());
            }
            if (!Accept(TokKind::Comma)) break;
          }
        } else {
          c.values = ParseExprList();
        }
        Expect(TokKind::Colon);
      }
      c.body = ParseCaseBody();
      s->cases.push_back(std::move(c));
      SkipSemis();
    }
    Expect(TokKind::RBrace);
    return s;
  }

  std::unique_ptr<Stmt> ParseSelectStmt() {
    Expect(TokKind::KwSelect);
    Expect(TokKind::LBrace);
    auto s = MakeStmt(StmtKind::Select);
    SkipSemis();
    while (!Check(TokKind::RBrace) && !Check(TokKind::Eof)) {
      SelectCase c;
      if (Accept(TokKind::KwDefault)) {
        c.is_default = true;
        Expect(TokKind::Colon);
      } else {
        Expect(TokKind::KwCase);
        auto comm = ParseSimpleStmt();
        Expect(TokKind::Colon);
        if (comm->kind == StmtKind::Send) {
          c.is_send = true;
          c.chan = std::move(comm->lhs[0]);
          c.value = std::move(comm->rhs[0]);
        } else if (comm->kind == StmtKind::ShortVarDecl) {
          c.recv_names = comm->names;
          c.recv_define = true;
          c.recv_ok = comm->names.size() == 2;
          if (comm->rhs.size() == 1) c.chan = std::move(comm->rhs[0]);
        } else if (comm->kind == StmtKind::Assign) {
          for (auto& e : comm->lhs) {
            if (e->kind == ExprKind::Ident) c.recv_names.push_back(e->strval);
          }
          c.recv_ok = c.recv_names.size() == 2;
          if (comm->rhs.size() == 1) c.chan = std::move(comm->rhs[0]);
        } else if (comm->kind == StmtKind::ExprStmt && !comm->lhs.empty()) {
          c.chan = std::move(comm->lhs[0]);
        } else {
          Fail("select case must be a send, receive, or default");
        }
        if (c.chan && c.chan->kind == ExprKind::Recv) {
          auto inner = std::move(c.chan->x);
          c.chan = std::move(inner);
        }
      }
      c.body = ParseCaseBody();
      s->sel_cases.push_back(std::move(c));
      SkipSemis();
    }
    Expect(TokKind::RBrace);
    return s;
  }

  std::vector<Param> ParseParamList(bool* variadic_out = nullptr) {
    std::vector<Param> params;
    if (Check(TokKind::RParen)) return params;
    bool named = ParamListIsNamed();
    for (;;) {
      if (named) {
        std::vector<std::string> names;
        names.push_back(ExpectIdent());
        while (Accept(TokKind::Comma)) {
          if (Check(TokKind::RParen)) break;
          if (LooksLikeTypeStart(Cur().kind) || Check(TokKind::Ellipsis)) break;
          if (!Check(TokKind::Ident)) break;
          names.push_back(ExpectIdent());
        }
        bool v = Accept(TokKind::Ellipsis);
        auto type = ParseType();
        for (auto& n : names) {
          Param p;
          p.name = n;
          p.type = CloneType(type.get());
          p.variadic = v;
          params.push_back(std::move(p));
        }
        if (v && variadic_out) *variadic_out = true;
      } else {
        Param p;
        p.variadic = Accept(TokKind::Ellipsis);
        p.type = ParseType();
        params.push_back(std::move(p));
        if (p.variadic && variadic_out) *variadic_out = true;
      }
      if (!Accept(TokKind::Comma)) break;
      if (Check(TokKind::RParen)) break;
    }
    return params;
  }

  // Parses one result entry, which is either a bare type or `name Type`.
  // The two are disambiguated the way real Go's grammar requires: read an
  // identifier, then check whether a comma/')' follows immediately (meaning
  // it was actually the whole (unnamed) type) or whether a type follows it
  // (meaning it was a result name).
  void ParseOneResult(std::vector<std::unique_ptr<TypeNode>>& types,
                      std::vector<std::string>& names) {
    if (Check(TokKind::Ident)) {
      std::string first = ExpectIdent();
      if (Check(TokKind::Comma) || Check(TokKind::RParen)) {
        types.push_back(MakeNamedType(first));
        names.push_back("");
        return;
      }
      // `(token.Token, string)` -- an unnamed result whose type is a
      // qualified `pkg.Type`, not a named result. A result name is
      // always a single bare identifier (never dotted), so IDENT '.'
      // unambiguously means "this identifier was the package qualifier
      // of an unnamed type," not "this identifier was a result name."
      // Missing this case misparsed `first` as a name and then tried to
      // parse a type starting at '.', failing with "expected an
      // identifier but found '.'".
      if (Accept(TokKind::Dot)) {
        std::string name = ExpectIdent();
        types.push_back(MakeNamedType(name, first));
        names.push_back("");
        return;
      }
      types.push_back(ParseType());
      names.push_back(first);
      return;
    }
    types.push_back(ParseType());
    names.push_back("");
  }

  void ParseResults(FuncDecl& fn) {
    if (Check(TokKind::LBrace)) return;  // no results
    if (Accept(TokKind::LParen)) {
      if (Accept(TokKind::RParen)) return;  // explicit "()"
      for (;;) {
        ParseOneResult(fn.results, fn.result_names);
        if (!Accept(TokKind::Comma)) break;
        if (Check(TokKind::RParen)) break;
      }
      Expect(TokKind::RParen);
      return;
    }
    fn.results.push_back(ParseType());
    fn.result_names.push_back("");
  }

  FuncDecl ParseFuncDecl() {
    Expect(TokKind::KwFunc);
    FuncDecl fn;
    if (Accept(TokKind::LParen)) {
      fn.has_receiver = true;
      fn.receiver_name = ExpectIdent();
      fn.receiver_is_pointer = Accept(TokKind::Star);
      fn.receiver_type = ExpectIdent();
      Expect(TokKind::RParen);
    }
    fn.name = ExpectIdent();
    if (Accept(TokKind::LBracket)) {
      for (;;) {
        fn.type_params.push_back(ExpectIdent());
        if (!Check(TokKind::Comma) && !Check(TokKind::RBracket)) {
          if (LooksLikeTypeStart(Cur().kind) || Check(TokKind::Ident)) {
            (void)ParseType();  // constraint; C++ templates don't need it
          }
        }
        if (!Accept(TokKind::Comma)) break;
        if (Check(TokKind::RBracket)) break;
      }
      Expect(TokKind::RBracket);
    }
    Expect(TokKind::LParen);
    fn.params = ParseParamList(&fn.variadic);
    Expect(TokKind::RParen);
    ParseResults(fn);
    fn.body = ParseBlock();
    return fn;
  }

  void ParseStructFields(StructDecl& sd) {
    Expect(TokKind::KwStruct);
    Expect(TokKind::LBrace);
    SkipSemis();
    while (!Check(TokKind::RBrace)) {
      FieldDecl fd;
      if (Check(TokKind::Star) || Check(TokKind::KwMap) || Check(TokKind::KwChan) ||
          Check(TokKind::LBracket) || Check(TokKind::KwFunc)) {
        fd.embedded = true;
        fd.type = ParseType();
        if (fd.type->kind == TypeKind::Named) fd.name = fd.type->name;
        else if (fd.type->kind == TypeKind::Pointer && fd.type->elem &&
                 fd.type->elem->kind == TypeKind::Named) {
          fd.name = fd.type->elem->name;
        }
      } else {
        std::string name = ExpectIdent();
        if (Check(TokKind::Semi) || Check(TokKind::RBrace)) {
          fd.embedded = true;
          fd.name = name;
          fd.type = MakeNamedType(name);
        } else {
          fd.name = name;
          fd.type = ParseType();
        }
      }
      sd.fields.push_back(std::move(fd));
      SkipSemis();
    }
    Expect(TokKind::RBrace);
  }

  InterfaceDecl ParseInterfaceBody(std::string name) {
    InterfaceDecl id;
    id.name = std::move(name);
    Expect(TokKind::KwInterface);
    Expect(TokKind::LBrace);
    SkipSemis();
    while (!Check(TokKind::RBrace)) {
      std::string n = ExpectIdent();
      if (Check(TokKind::LParen)) {
        MethodSig ms;
        ms.name = std::move(n);
        Expect(TokKind::LParen);
        ms.params = ParseParamList();
        Expect(TokKind::RParen);
        if (Accept(TokKind::LParen)) {
          if (!Accept(TokKind::RParen)) {
            std::vector<std::string> dummy;
            for (;;) {
              ParseOneResult(ms.results, dummy);
              if (!Accept(TokKind::Comma)) break;
              if (Check(TokKind::RParen)) break;
            }
            Expect(TokKind::RParen);
          }
        } else if (LooksLikeTypeStart(Cur().kind) || Check(TokKind::Ident)) {
          ms.results.push_back(ParseType());
        }
        id.methods.push_back(std::move(ms));
      } else {
        id.embedded.push_back(std::move(n));
      }
      SkipSemis();
    }
    Expect(TokKind::RBrace);
    return id;
  }

  void ParseTypeDecl(File& f) {
    Expect(TokKind::KwType);
    if (Accept(TokKind::LParen)) {
      SkipSemis();
      while (!Check(TokKind::RParen)) {
        ParseOneTypeSpec(f);
        SkipSemis();
      }
      Expect(TokKind::RParen);
      return;
    }
    ParseOneTypeSpec(f);
  }

  void ParseOneTypeSpec(File& f) {
    std::string name = ExpectIdent();
    if (Check(TokKind::KwStruct)) {
      StructDecl sd;
      sd.name = std::move(name);
      ParseStructFields(sd);
      f.structs.push_back(std::move(sd));
    } else if (Check(TokKind::KwInterface)) {
      f.interfaces.push_back(ParseInterfaceBody(std::move(name)));
    } else {
      TypeAlias a;
      a.name = std::move(name);
      a.is_alias_eq = Accept(TokKind::Assign);
      a.type = ParseType();
      f.aliases.push_back(std::move(a));
    }
  }
};

}  // namespace

File Parse(const std::vector<Token>& tokens) {
  ParserImpl p(tokens);
  return p.ParseFile();
}

}  // namespace wasigo
