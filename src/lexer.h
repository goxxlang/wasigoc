// wasigoc lexer: tokenizes a subset of real Go source (not a bespoke IDL --
// see README.md for exactly which subset). Implements Go's automatic
// semicolon insertion (ASI) so ordinary, unmodified Go source -- without
// manually written trailing semicolons -- lexes the way a real Go file is
// written.
#pragma once

#include <stdexcept>
#include <string>
#include <vector>

namespace wasigo {

enum class TokKind {
  // literals / names
  Ident,
  IntLit,
  FloatLit,
  ImagLit,  // 2i, .5i — imaginary component of a complex128
  StringLit,
  RuneLit,

  // keywords
  KwPackage,
  KwImport,
  KwFunc,
  KwVar,
  KwConst,
  KwType,
  KwStruct,
  KwMap,
  KwReturn,
  KwIf,
  KwElse,
  KwFor,
  KwRange,
  KwTrue,
  KwFalse,
  KwNil,
  KwBreak,
  KwContinue,
  KwGo,
  KwDefer,
  KwChan,
  KwInterface,
  KwSwitch,
  KwCase,
  KwDefault,
  KwSelect,
  KwFallthrough,
  KwGoto,

  // punctuation
  LParen,
  RParen,
  LBrace,
  RBrace,
  LBracket,
  RBracket,
  Comma,
  Semi,
  Colon,
  Dot,
  Ellipsis,
  Arrow,  // <-

  // operators
  Assign,
  Define,
  Plus,
  Minus,
  Star,
  Slash,
  Percent,
  Amp,
  Pipe,
  Caret,
  AndNot,  // &^
  Shl,
  Shr,
  AndAnd,
  OrOr,
  Not,
  Eq,
  Neq,
  Lt,
  Leq,
  Gt,
  Geq,
  PlusEq,
  MinusEq,
  StarEq,
  SlashEq,
  PercentEq,
  AmpEq,
  PipeEq,
  CaretEq,
  AndNotEq,  // &^=
  ShlEq,
  ShrEq,
  PlusPlus,
  MinusMinus,

  Eof,
};

struct Token {
  TokKind kind;
  std::string text;   // raw/decoded text: identifier name, string contents, ...
  long long intval = 0;
  double floatval = 0;
  int line = 0;
  int col = 0;
};

class LexError : public std::runtime_error {
 public:
  explicit LexError(const std::string& msg) : std::runtime_error(msg) {}
};

// Tokenizes `source`, inserting `Semi` tokens per Go's ASI rule wherever a
// newline follows a token kind that can legally end a statement.
std::vector<Token> Tokenize(const std::string& source);

const char* TokKindName(TokKind kind);

}  // namespace wasigo
