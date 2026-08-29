#include "lexer.h"

#include <cctype>
#include <unordered_map>

namespace wasigo {

namespace {

const std::unordered_map<std::string, TokKind> kKeywords = {
    {"package", TokKind::KwPackage}, {"import", TokKind::KwImport},
    {"func", TokKind::KwFunc},       {"var", TokKind::KwVar},
    {"const", TokKind::KwConst},     {"type", TokKind::KwType},
    {"struct", TokKind::KwStruct},   {"map", TokKind::KwMap},
    {"return", TokKind::KwReturn},   {"if", TokKind::KwIf},
    {"else", TokKind::KwElse},       {"for", TokKind::KwFor},
    {"range", TokKind::KwRange},     {"true", TokKind::KwTrue},
    {"false", TokKind::KwFalse},     {"nil", TokKind::KwNil},
    {"break", TokKind::KwBreak},     {"continue", TokKind::KwContinue},
    {"go", TokKind::KwGo},           {"defer", TokKind::KwDefer},
    {"chan", TokKind::KwChan},       {"interface", TokKind::KwInterface},
    {"switch", TokKind::KwSwitch},   {"case", TokKind::KwCase},
    {"default", TokKind::KwDefault}, {"select", TokKind::KwSelect},
    {"fallthrough", TokKind::KwFallthrough},
    {"goto", TokKind::KwGoto},
};

// Go's ASI rule (spec "Semicolons"): a newline after one of these token
// kinds becomes a semicolon. Everything else, a newline is just whitespace.
bool EndsStatement(TokKind k) {
  switch (k) {
    case TokKind::Ident:
    case TokKind::IntLit:
    case TokKind::FloatLit:
    case TokKind::StringLit:
    case TokKind::RuneLit:
    case TokKind::KwTrue:
    case TokKind::KwFalse:
    case TokKind::KwNil:
    case TokKind::KwBreak:
    case TokKind::KwContinue:
    case TokKind::KwFallthrough:
    case TokKind::KwReturn:
    case TokKind::RParen:
    case TokKind::RBrace:
    case TokKind::RBracket:
    case TokKind::PlusPlus:
    case TokKind::MinusMinus:
      return true;
    default:
      return false;
  }
}

class Scanner {
 public:
  explicit Scanner(const std::string& src) : src_(src) {}

  std::vector<Token> Run() {
    std::vector<Token> out;
    TokKind last = TokKind::Semi;  // pretend start-of-file already ended one
    bool have_last = false;
    while (true) {
      SkipSpacesAndComments(out, last, have_last);
      if (pos_ >= src_.size()) {
        if (have_last && EndsStatement(last)) {
          out.push_back(MakeTok(TokKind::Semi, ";"));
        }
        out.push_back(MakeTok(TokKind::Eof, ""));
        break;
      }
      Token t = NextToken();
      out.push_back(t);
      last = t.kind;
      have_last = true;
    }
    return out;
  }

 private:
  const std::string& src_;
  size_t pos_ = 0;
  int line_ = 1;
  int col_ = 1;

  char Peek(size_t off = 0) const {
    size_t p = pos_ + off;
    return p < src_.size() ? src_[p] : '\0';
  }

  char Advance() {
    char c = src_[pos_++];
    if (c == '\n') {
      line_++;
      col_ = 1;
    } else {
      col_++;
    }
    return c;
  }

  Token MakeTok(TokKind k, std::string text) {
    Token t;
    t.kind = k;
    t.text = std::move(text);
    t.line = line_;
    t.col = col_;
    return t;
  }

  [[noreturn]] void Fail(const std::string& msg) {
    throw LexError("line " + std::to_string(line_) + ": " + msg);
  }

  // Skips whitespace/comments, emitting a synthetic Semi into `out` the
  // first time a newline is seen after a statement-ending token.
  void SkipSpacesAndComments(std::vector<Token>& out, TokKind last,
                              bool have_last) {
    bool inserted_semi_this_run = false;
    for (;;) {
      char c = Peek();
      if (c == '\n') {
        if (have_last && !inserted_semi_this_run && EndsStatement(last)) {
          out.push_back(MakeTok(TokKind::Semi, ";"));
          inserted_semi_this_run = true;
          last = TokKind::Semi;
          have_last = true;
        }
        Advance();
        continue;
      }
      if (c == ' ' || c == '\t' || c == '\r') {
        Advance();
        continue;
      }
      if (c == '/' && Peek(1) == '/') {
        while (pos_ < src_.size() && Peek() != '\n') Advance();
        continue;
      }
      if (c == '/' && Peek(1) == '*') {
        Advance();
        Advance();
        bool saw_newline = false;
        while (pos_ < src_.size() && !(Peek() == '*' && Peek(1) == '/')) {
          if (Peek() == '\n') saw_newline = true;
          Advance();
        }
        if (pos_ >= src_.size()) Fail("unterminated block comment");
        Advance();
        Advance();
        // A block comment containing a newline acts like a newline for ASI.
        if (saw_newline && have_last && !inserted_semi_this_run &&
            EndsStatement(last)) {
          out.push_back(MakeTok(TokKind::Semi, ";"));
          inserted_semi_this_run = true;
          last = TokKind::Semi;
          have_last = true;
        }
        continue;
      }
      break;
    }
  }

  static bool IsIdentStart(char c) {
    return std::isalpha(static_cast<unsigned char>(c)) || c == '_';
  }
  static bool IsIdentCont(char c) {
    return std::isalnum(static_cast<unsigned char>(c)) || c == '_';
  }

  Token NextToken() {
    char c = Peek();

    if (IsIdentStart(c)) return ScanIdentOrKeyword();
    if (std::isdigit(static_cast<unsigned char>(c))) return ScanNumber();
    if (c == '"') return ScanString();
    if (c == '`') return ScanRawString();
    if (c == '\'') return ScanRune();

    switch (c) {
      case '(': Advance(); return MakeTok(TokKind::LParen, "(");
      case ')': Advance(); return MakeTok(TokKind::RParen, ")");
      case '{': Advance(); return MakeTok(TokKind::LBrace, "{");
      case '}': Advance(); return MakeTok(TokKind::RBrace, "}");
      case '[': Advance(); return MakeTok(TokKind::LBracket, "[");
      case ']': Advance(); return MakeTok(TokKind::RBracket, "]");
      case ',': Advance(); return MakeTok(TokKind::Comma, ",");
      case ';': Advance(); return MakeTok(TokKind::Semi, ";");
      case ':':
        Advance();
        if (Peek() == '=') { Advance(); return MakeTok(TokKind::Define, ":="); }
        return MakeTok(TokKind::Colon, ":");
      case '.':
        if (Peek(1) == '.' && Peek(2) == '.') {
          Advance(); Advance(); Advance();
          return MakeTok(TokKind::Ellipsis, "...");
        }
        Advance();
        return MakeTok(TokKind::Dot, ".");
      case '+':
        Advance();
        if (Peek() == '+') { Advance(); return MakeTok(TokKind::PlusPlus, "++"); }
        if (Peek() == '=') { Advance(); return MakeTok(TokKind::PlusEq, "+="); }
        return MakeTok(TokKind::Plus, "+");
      case '-':
        Advance();
        if (Peek() == '-') { Advance(); return MakeTok(TokKind::MinusMinus, "--"); }
        if (Peek() == '=') { Advance(); return MakeTok(TokKind::MinusEq, "-="); }
        return MakeTok(TokKind::Minus, "-");
      case '*':
        Advance();
        if (Peek() == '=') { Advance(); return MakeTok(TokKind::StarEq, "*="); }
        return MakeTok(TokKind::Star, "*");
      case '/':
        Advance();
        if (Peek() == '=') { Advance(); return MakeTok(TokKind::SlashEq, "/="); }
        return MakeTok(TokKind::Slash, "/");
      case '%':
        Advance();
        if (Peek() == '=') { Advance(); return MakeTok(TokKind::PercentEq, "%="); }
        return MakeTok(TokKind::Percent, "%");
      case '&':
        Advance();
        if (Peek() == '&') { Advance(); return MakeTok(TokKind::AndAnd, "&&"); }
        if (Peek() == '^') {
          Advance();
          if (Peek() == '=') { Advance(); return MakeTok(TokKind::AndNotEq, "&^="); }
          return MakeTok(TokKind::AndNot, "&^");
        }
        if (Peek() == '=') { Advance(); return MakeTok(TokKind::AmpEq, "&="); }
        return MakeTok(TokKind::Amp, "&");
      case '|':
        Advance();
        if (Peek() == '|') { Advance(); return MakeTok(TokKind::OrOr, "||"); }
        if (Peek() == '=') { Advance(); return MakeTok(TokKind::PipeEq, "|="); }
        return MakeTok(TokKind::Pipe, "|");
      case '^':
        Advance();
        if (Peek() == '=') { Advance(); return MakeTok(TokKind::CaretEq, "^="); }
        return MakeTok(TokKind::Caret, "^");
      case '!':
        Advance();
        if (Peek() == '=') { Advance(); return MakeTok(TokKind::Neq, "!="); }
        return MakeTok(TokKind::Not, "!");
      case '=':
        Advance();
        if (Peek() == '=') { Advance(); return MakeTok(TokKind::Eq, "=="); }
        return MakeTok(TokKind::Assign, "=");
      case '<':
        Advance();
        if (Peek() == '-') { Advance(); return MakeTok(TokKind::Arrow, "<-"); }
        if (Peek() == '=') { Advance(); return MakeTok(TokKind::Leq, "<="); }
        if (Peek() == '<') {
          Advance();
          if (Peek() == '=') { Advance(); return MakeTok(TokKind::ShlEq, "<<="); }
          return MakeTok(TokKind::Shl, "<<");
        }
        return MakeTok(TokKind::Lt, "<");
      case '>':
        Advance();
        if (Peek() == '=') { Advance(); return MakeTok(TokKind::Geq, ">="); }
        if (Peek() == '>') {
          Advance();
          if (Peek() == '=') { Advance(); return MakeTok(TokKind::ShrEq, ">>="); }
          return MakeTok(TokKind::Shr, ">>");
        }
        return MakeTok(TokKind::Gt, ">");
      default:
        Fail(std::string("unexpected character '") + c + "'");
    }
  }

  Token ScanIdentOrKeyword() {
    std::string s;
    while (pos_ < src_.size() && IsIdentCont(Peek())) s.push_back(Advance());
    auto it = kKeywords.find(s);
    if (it != kKeywords.end()) return MakeTok(it->second, s);
    return MakeTok(TokKind::Ident, s);
  }

  // 0x/0X hex, 0o/0O octal, 0b/0B binary integer literals (underscores
  // allowed as digit separators, same as decimal). Only the explicit-
  // prefix forms -- no legacy `0755`-style octal, no hex floats
  // (`0x1p10`). Long overdue: crypto/hash constants (MD5's K table,
  // SHA-256's round constants, CRC polynomials) are always specified in
  // hex, and hand-converting 64 magic numbers to decimal by hand is
  // exactly the kind of transcription work that quietly introduces bugs.
  Token ScanRadixInt(int base, bool (*is_digit)(char)) {
    Advance();
    Advance();
    std::string s;
    while (pos_ < src_.size() && (is_digit(Peek()) || Peek() == '_')) {
      char d = Advance();
      if (d != '_') s.push_back(d);
    }
    if (s.empty()) Fail("malformed number: no digits after radix prefix");
    Token t = MakeTok(TokKind::IntLit, s);
    t.intval = static_cast<int64_t>(std::stoull(s, nullptr, base));
    return t;
  }

  Token ScanNumber() {
    if (Peek() == '0' && (Peek(1) == 'x' || Peek(1) == 'X')) {
      return ScanRadixInt(16, [](char c) { return static_cast<bool>(std::isxdigit(static_cast<unsigned char>(c))); });
    }
    if (Peek() == '0' && (Peek(1) == 'o' || Peek(1) == 'O')) {
      return ScanRadixInt(8, [](char c) { return c >= '0' && c <= '7'; });
    }
    if (Peek() == '0' && (Peek(1) == 'b' || Peek(1) == 'B')) {
      return ScanRadixInt(2, [](char c) { return c == '0' || c == '1'; });
    }
    std::string s;
    bool is_float = false;
    while (pos_ < src_.size() &&
           (std::isdigit(static_cast<unsigned char>(Peek())) || Peek() == '_')) {
      s.push_back(Advance());
    }
    if (Peek() == '.' && std::isdigit(static_cast<unsigned char>(Peek(1)))) {
      is_float = true;
      s.push_back(Advance());
      while (pos_ < src_.size() && std::isdigit(static_cast<unsigned char>(Peek()))) {
        s.push_back(Advance());
      }
    }
    if (Peek() == 'e' || Peek() == 'E') {
      is_float = true;
      s.push_back(Advance());
      if (Peek() == '+' || Peek() == '-') s.push_back(Advance());
      while (pos_ < src_.size() && std::isdigit(static_cast<unsigned char>(Peek()))) {
        s.push_back(Advance());
      }
    }
    std::string clean;
    for (char c : s) if (c != '_') clean.push_back(c);
    Token t = MakeTok(is_float ? TokKind::FloatLit : TokKind::IntLit, clean);
    if (is_float) {
      t.floatval = std::stod(clean);
    } else {
      // stoull (not stoll) so a decimal literal past INT64_MAX but within
      // uint64 range (e.g. FNV's 14695981039346656037 offset basis, or a
      // CRC64 polynomial) doesn't throw "out of range" -- reinterpreting
      // the unsigned 64-bit result as int64_t is a bit-pattern-preserving
      // no-op (well-defined conversion, C++20), so a later uint64-typed
      // use of this constant still recovers the intended value via C++'s
      // own two's-complement wraparound. No hex/octal/binary literals are
      // supported at all (unrelated gap, unfixed -- see README).
      t.intval = static_cast<int64_t>(std::stoull(clean));
    }
    return t;
  }

  static int HexDigitValue(char c) {
    if (c >= '0' && c <= '9') return c - '0';
    if (c >= 'a' && c <= 'f') return c - 'a' + 10;
    if (c >= 'A' && c <= 'F') return c - 'A' + 10;
    return -1;
  }

  int ScanEscape() {
    char c = Advance();
    switch (c) {
      case 'n': return '\n';
      case 't': return '\t';
      case 'r': return '\r';
      case '\\': return '\\';
      case '\'': return '\'';
      case '"': return '"';
      case '0': return '\0';
      case 'x': {
        // \xHH: exactly 2 hex digits, one resulting byte -- matches real
        // Go's own \x escape (unlike \u/\U, which decode a Unicode code
        // point to potentially multiple UTF-8 bytes; ScanEscape returns
        // exactly one byte value to both its callers (ScanString pushes
        // one char, ScanRune takes one int), so \u/\U stay a separate,
        // unfixed gap -- this fix is scoped to exactly the case that was
        // blocking real stdlib source (debug/buildinfo's own magic
        // string, matching real Go's linker-emitted byte sequence).
        int hi = HexDigitValue(Advance());
        int lo = HexDigitValue(Advance());
        if (hi < 0 || lo < 0) Fail("invalid \\x escape: expected 2 hex digits");
        return (hi << 4) | lo;
      }
      default: Fail(std::string("unknown escape '\\") + c + "'");
    }
  }

  Token ScanString() {
    Advance();  // opening "
    std::string s;
    while (pos_ < src_.size() && Peek() != '"') {
      char c = Peek();
      if (c == '\n') Fail("newline in string literal");
      if (c == '\\') {
        Advance();
        s.push_back(static_cast<char>(ScanEscape()));
      } else {
        s.push_back(Advance());
      }
    }
    if (pos_ >= src_.size()) Fail("unterminated string literal");
    Advance();  // closing "
    return MakeTok(TokKind::StringLit, s);
  }

  Token ScanRawString() {
    Advance();  // opening `
    std::string s;
    while (pos_ < src_.size() && Peek() != '`') s.push_back(Advance());
    if (pos_ >= src_.size()) Fail("unterminated raw string literal");
    Advance();  // closing `
    return MakeTok(TokKind::StringLit, s);
  }

  Token ScanRune() {
    Advance();  // opening '
    int v;
    if (Peek() == '\\') {
      Advance();
      v = ScanEscape();
    } else {
      v = static_cast<unsigned char>(Advance());
    }
    if (Peek() != '\'') Fail("unterminated rune literal");
    Advance();
    Token t = MakeTok(TokKind::RuneLit, std::string(1, static_cast<char>(v)));
    t.intval = v;
    return t;
  }
};

}  // namespace

std::vector<Token> Tokenize(const std::string& source) {
  Scanner s(source);
  return s.Run();
}

const char* TokKindName(TokKind kind) {
  switch (kind) {
    case TokKind::Ident: return "identifier";
    case TokKind::IntLit: return "int literal";
    case TokKind::FloatLit: return "float literal";
    case TokKind::StringLit: return "string literal";
    case TokKind::RuneLit: return "rune literal";
    case TokKind::KwPackage: return "'package'";
    case TokKind::KwImport: return "'import'";
    case TokKind::KwFunc: return "'func'";
    case TokKind::KwVar: return "'var'";
    case TokKind::KwConst: return "'const'";
    case TokKind::KwType: return "'type'";
    case TokKind::KwStruct: return "'struct'";
    case TokKind::KwMap: return "'map'";
    case TokKind::KwReturn: return "'return'";
    case TokKind::KwIf: return "'if'";
    case TokKind::KwElse: return "'else'";
    case TokKind::KwFor: return "'for'";
    case TokKind::KwRange: return "'range'";
    case TokKind::KwTrue: return "'true'";
    case TokKind::KwFalse: return "'false'";
    case TokKind::KwNil: return "'nil'";
    case TokKind::KwBreak: return "'break'";
    case TokKind::KwContinue: return "'continue'";
    case TokKind::KwGo: return "'go'";
    case TokKind::KwDefer: return "'defer'";
    case TokKind::KwChan: return "'chan'";
    case TokKind::KwInterface: return "'interface'";
    case TokKind::KwSwitch: return "'switch'";
    case TokKind::KwCase: return "'case'";
    case TokKind::KwDefault: return "'default'";
    case TokKind::KwSelect: return "'select'";
    case TokKind::KwFallthrough: return "'fallthrough'";
    case TokKind::KwGoto: return "'goto'";
    case TokKind::LParen: return "'('";
    case TokKind::RParen: return "')'";
    case TokKind::LBrace: return "'{'";
    case TokKind::RBrace: return "'}'";
    case TokKind::LBracket: return "'['";
    case TokKind::RBracket: return "']'";
    case TokKind::Comma: return "','";
    case TokKind::Semi: return "';'";
    case TokKind::Colon: return "':'";
    case TokKind::Dot: return "'.'";
    case TokKind::Ellipsis: return "'...'";
    case TokKind::Arrow: return "'<-'";
    case TokKind::Assign: return "'='";
    case TokKind::Define: return "':='";
    case TokKind::Plus: return "'+'";
    case TokKind::Minus: return "'-'";
    case TokKind::Star: return "'*'";
    case TokKind::Slash: return "'/'";
    case TokKind::Percent: return "'%'";
    case TokKind::Amp: return "'&'";
    case TokKind::Pipe: return "'|'";
    case TokKind::Caret: return "'^'";
    case TokKind::AndNot: return "'&^'";
    case TokKind::Shl: return "'<<'";
    case TokKind::Shr: return "'>>'";
    case TokKind::AndAnd: return "'&&'";
    case TokKind::OrOr: return "'||'";
    case TokKind::Not: return "'!'";
    case TokKind::Eq: return "'=='";
    case TokKind::Neq: return "'!='";
    case TokKind::Lt: return "'<'";
    case TokKind::Leq: return "'<='";
    case TokKind::Gt: return "'>'";
    case TokKind::Geq: return "'>='";
    case TokKind::PlusEq: return "'+='";
    case TokKind::MinusEq: return "'-='";
    case TokKind::StarEq: return "'*='";
    case TokKind::SlashEq: return "'/='";
    case TokKind::PercentEq: return "'%='";
    case TokKind::AmpEq: return "'&='";
    case TokKind::PipeEq: return "'|='";
    case TokKind::CaretEq: return "'^='";
    case TokKind::AndNotEq: return "'&^='";
    case TokKind::ShlEq: return "'<<='";
    case TokKind::ShrEq: return "'>>='";
    case TokKind::PlusPlus: return "'++'";
    case TokKind::MinusMinus: return "'--'";
    case TokKind::Eof: return "end of file";
  }
  return "?";
}

}  // namespace wasigo
