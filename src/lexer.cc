#include "lexer.h"

#include <cctype>
#include <cmath>
#include <cstdint>
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
    case TokKind::ImagLit:
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
          BeginTok();
          out.push_back(MakeTok(TokKind::Semi, ";"));
        }
        BeginTok();
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
  int tok_line_ = 1;
  int tok_col_ = 1;

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

  void BeginTok() {
    tok_line_ = line_;
    tok_col_ = col_;
  }

  Token MakeTok(TokKind k, std::string text) {
    Token t;
    t.kind = k;
    t.text = std::move(text);
    t.line = tok_line_;
    t.col = tok_col_;
    return t;
  }

  [[noreturn]] void Fail(const std::string& msg) {
    throw LexError("line " + std::to_string(line_) + ":" + std::to_string(col_) + ": " + msg);
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
          BeginTok();
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
          BeginTok();
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
    BeginTok();
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
        if (std::isdigit(static_cast<unsigned char>(Peek(1)))) {
          return ScanLeadingDotFloat();
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

  static bool IsHex(char c) {
    return static_cast<bool>(std::isxdigit(static_cast<unsigned char>(c)));
  }
  static bool IsOct(char c) { return c >= '0' && c <= '7'; }
  static bool IsBin(char c) { return c == '0' || c == '1'; }
  static bool IsDec(char c) {
    return static_cast<bool>(std::isdigit(static_cast<unsigned char>(c)));
  }

  // Underscores must separate successive digits (Go spec). After a radix
  // prefix, one leading underscore is allowed (`0x_FF`).
  bool ScanDigitRun(std::string& s, bool (*is_digit)(char), bool allow_leading_underscore) {
    bool last_digit = false;
    bool pending_us = false;
    if (allow_leading_underscore && Peek() == '_') {
      Advance();
      pending_us = true;
    }
    while (pos_ < src_.size()) {
      char c = Peek();
      if (c == '_') {
        if (!last_digit) Fail("underscore must separate successive digits");
        Advance();
        last_digit = false;
        pending_us = true;
        continue;
      }
      if (!is_digit(c)) break;
      s.push_back(Advance());
      last_digit = true;
      pending_us = false;
    }
    if (pending_us) Fail("underscore must separate successive digits");
    return last_digit;
  }

  Token MaybeImag(Token t) {
    if (Peek() != 'i') return t;
    Advance();
    Token im = MakeTok(TokKind::ImagLit, t.text + "i");
    if (t.kind == TokKind::FloatLit) {
      im.floatval = t.floatval;
    } else if (t.intval < 0) {
      im.floatval = static_cast<double>(static_cast<uint64_t>(t.intval));
    } else {
      im.floatval = static_cast<double>(t.intval);
    }
    return im;
  }

  Token ScanRadixInt(int base, bool (*is_digit)(char)) {
    Advance();
    Advance();
    std::string s;
    if (!ScanDigitRun(s, is_digit, true)) Fail("malformed number: no digits after radix prefix");
    Token t = MakeTok(TokKind::IntLit, s);
    t.intval = static_cast<int64_t>(std::stoull(s, nullptr, base));
    return MaybeImag(t);
  }

  Token ScanHexLiteral() {
    Advance();  // 0
    Advance();  // x/X
    std::string mag;
    std::string frac;
    bool mag_ok = ScanDigitRun(mag, IsHex, true);
    bool saw_dot = false;
    if (Peek() == '.') {
      char n = Peek(1);
      if (IsHex(n) || n == 'p' || n == 'P') {
        saw_dot = true;
        Advance();
        ScanDigitRun(frac, IsHex, false);
      }
    }
    if (Peek() == 'p' || Peek() == 'P') {
      if (!mag_ok && frac.empty()) Fail("malformed hex float: no mantissa digits");
      Advance();
      int sign = 1;
      if (Peek() == '+') Advance();
      else if (Peek() == '-') {
        sign = -1;
        Advance();
      }
      std::string exp;
      if (!ScanDigitRun(exp, IsDec, true)) Fail("malformed hex float: exponent has no digits");
      int expv = sign * static_cast<int>(std::stoi(exp));
      double v = 0;
      for (char c : mag) v = v * 16.0 + HexDigitValue(c);
      double place = 1;
      for (char c : frac) {
        place /= 16.0;
        v += HexDigitValue(c) * place;
      }
      v = std::ldexp(v, expv);
      std::string text = mag;
      if (saw_dot) text += "." + frac;
      text += "p" + exp;
      Token t = MakeTok(TokKind::FloatLit, text);
      t.floatval = v;
      return MaybeImag(t);
    }
    if (saw_dot) Fail("hexadecimal mantissa requires a 'p' exponent");
    if (!mag_ok) Fail("malformed number: no digits after radix prefix");
    Token t = MakeTok(TokKind::IntLit, mag);
    t.intval = static_cast<int64_t>(std::stoull(mag, nullptr, 16));
    return MaybeImag(t);
  }

  void ScanExponent(std::string& s) {
    s.push_back(Advance());
    if (Peek() == '+' || Peek() == '-') s.push_back(Advance());
    std::string exp;
    if (!ScanDigitRun(exp, IsDec, true)) Fail("malformed float: exponent has no digits");
    s += exp;
  }

  Token FinishFloat(const std::string& s) {
    std::string clean;
    for (char c : s) if (c != '_') clean.push_back(c);
    Token t = MakeTok(TokKind::FloatLit, clean);
    t.floatval = std::stod(clean);
    return MaybeImag(t);
  }

  Token ScanLeadingDotFloat() {
    std::string s;
    s.push_back(Advance());  // '.'
    ScanDigitRun(s, IsDec, false);
    if (Peek() == 'e' || Peek() == 'E') ScanExponent(s);
    return FinishFloat(s);
  }

  Token ScanNumber() {
    if (Peek() == '0' && (Peek(1) == 'x' || Peek(1) == 'X')) {
      return ScanHexLiteral();
    }
    if (Peek() == '0' && (Peek(1) == 'o' || Peek(1) == 'O')) {
      return ScanRadixInt(8, IsOct);
    }
    if (Peek() == '0' && (Peek(1) == 'b' || Peek(1) == 'B')) {
      return ScanRadixInt(2, IsBin);
    }
    std::string s;
    bool is_float = false;
    ScanDigitRun(s, IsDec, false);
    if (Peek() == '.' && Peek(1) != '.') {
      is_float = true;
      s.push_back(Advance());
      ScanDigitRun(s, IsDec, false);
    }
    if (Peek() == 'e' || Peek() == 'E') {
      is_float = true;
      ScanExponent(s);
    }
    if (is_float) return FinishFloat(s);
    std::string clean;
    for (char c : s) if (c != '_') clean.push_back(c);
    Token t = MakeTok(TokKind::IntLit, clean);
    // stoull (not stoll) so a decimal literal past INT64_MAX but within
    // uint64 range (e.g. FNV's 14695981039346656037 offset basis) doesn't
    // throw "out of range" -- the unsigned 64-bit bit pattern is stored in
    // intval and recovered for uint64-typed uses.
    // A leading 0 (not 0x/0o/0b) is still octal in Go (`0755`). Digits 8/9
    // in that form are a lex error, matching gc.
    if (clean.size() > 1 && clean[0] == '0') {
      for (char d : clean) {
        if (d == '8' || d == '9') Fail("invalid octal literal");
      }
      t.intval = static_cast<int64_t>(std::stoull(clean, nullptr, 8));
    } else {
      t.intval = static_cast<int64_t>(std::stoull(clean));
    }
    return MaybeImag(t);
  }

  static int HexDigitValue(char c) {
    if (c >= '0' && c <= '9') return c - '0';
    if (c >= 'a' && c <= 'f') return c - 'a' + 10;
    if (c >= 'A' && c <= 'F') return c - 'A' + 10;
    return -1;
  }

  uint32_t ScanHexN(int n, const char* what) {
    uint32_t v = 0;
    for (int i = 0; i < n; i++) {
      int d = HexDigitValue(Peek());
      if (d < 0) Fail(std::string("invalid ") + what + ": expected hex digit");
      Advance();
      v = (v << 4) | static_cast<uint32_t>(d);
    }
    return v;
  }

  void AppendUtf8(std::string& out, uint32_t cp) {
    if (cp > 0x10FFFF || (cp >= 0xD800 && cp <= 0xDFFF)) {
      Fail("invalid Unicode code point in escape");
    }
    if (cp < 0x80) {
      out.push_back(static_cast<char>(cp));
    } else if (cp < 0x800) {
      out.push_back(static_cast<char>(0xC0 | (cp >> 6)));
      out.push_back(static_cast<char>(0x80 | (cp & 0x3F)));
    } else if (cp < 0x10000) {
      out.push_back(static_cast<char>(0xE0 | (cp >> 12)));
      out.push_back(static_cast<char>(0x80 | ((cp >> 6) & 0x3F)));
      out.push_back(static_cast<char>(0x80 | (cp & 0x3F)));
    } else {
      out.push_back(static_cast<char>(0xF0 | (cp >> 18)));
      out.push_back(static_cast<char>(0x80 | ((cp >> 12) & 0x3F)));
      out.push_back(static_cast<char>(0x80 | ((cp >> 6) & 0x3F)));
      out.push_back(static_cast<char>(0x80 | (cp & 0x3F)));
    }
  }

  // Caller already consumed the backslash. Appends UTF-8 to `out` and
  // returns the Unicode code point (for rune literals).
  int ScanEscape(std::string& out) {
    char c = Advance();
    auto one = [&](int cp) {
      AppendUtf8(out, static_cast<uint32_t>(cp));
      return cp;
    };
    switch (c) {
      case 'a': return one('\a');
      case 'b': return one('\b');
      case 'f': return one('\f');
      case 'n': return one('\n');
      case 'r': return one('\r');
      case 't': return one('\t');
      case 'v': return one('\v');
      case '\\': return one('\\');
      case '\'': return one('\'');
      case '"': return one('"');
      case 'x': {
        uint32_t v = ScanHexN(2, "\\x escape");
        out.push_back(static_cast<char>(v));
        return static_cast<int>(v);
      }
      case 'u': {
        uint32_t v = ScanHexN(4, "\\u escape");
        AppendUtf8(out, v);
        return static_cast<int>(v);
      }
      case 'U': {
        uint32_t v = ScanHexN(8, "\\U escape");
        AppendUtf8(out, v);
        return static_cast<int>(v);
      }
      default:
        if (c >= '0' && c <= '7') {
          int v = c - '0';
          int n = 1;
          while (n < 3 && Peek() >= '0' && Peek() <= '7') {
            v = v * 8 + (Advance() - '0');
            n++;
          }
          if (v > 255) Fail("octal escape value out of range");
          out.push_back(static_cast<char>(v));
          return v;
        }
        Fail(std::string("unknown escape '\\") + c + "'");
    }
    return 0;
  }

  int DecodeUtf8Rune() {
    unsigned char b0 = static_cast<unsigned char>(Advance());
    if (b0 < 0x80) return b0;
    int need = 0;
    uint32_t cp = 0;
    if ((b0 & 0xE0) == 0xC0) {
      need = 1;
      cp = b0 & 0x1F;
    } else if ((b0 & 0xF0) == 0xE0) {
      need = 2;
      cp = b0 & 0x0F;
    } else if ((b0 & 0xF8) == 0xF0) {
      need = 3;
      cp = b0 & 0x07;
    } else {
      Fail("invalid UTF-8 in rune literal");
    }
    for (int i = 0; i < need; i++) {
      unsigned char b = static_cast<unsigned char>(Peek());
      if (pos_ >= src_.size() || (b & 0xC0) != 0x80) Fail("invalid UTF-8 in rune literal");
      Advance();
      cp = (cp << 6) | (b & 0x3F);
    }
    if (cp > 0x10FFFF || (cp >= 0xD800 && cp <= 0xDFFF)) {
      Fail("invalid Unicode code point in rune literal");
    }
    return static_cast<int>(cp);
  }

  Token ScanString() {
    Advance();  // opening "
    std::string s;
    while (pos_ < src_.size() && Peek() != '"') {
      char c = Peek();
      if (c == '\n') Fail("newline in string literal");
      if (c == '\\') {
        Advance();
        ScanEscape(s);
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
      std::string tmp;
      v = ScanEscape(tmp);
    } else {
      if (Peek() == '\'') Fail("empty rune literal");
      v = DecodeUtf8Rune();
    }
    if (Peek() != '\'') Fail("unterminated rune literal");
    Advance();
    Token t = MakeTok(TokKind::RuneLit, std::string(1, static_cast<char>(v < 128 ? v : '?')));
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
    case TokKind::ImagLit: return "imaginary literal";
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
