// Host tests for literal lexing and codegen error spans.
#include "cpp_generator.h"
#include "lexer.h"
#include "parser.h"

#include <cassert>
#include <iostream>
#include <string>

using namespace wasigo;

static const Token& First(const std::vector<Token>& toks, TokKind k) {
  for (auto& t : toks) {
    if (t.kind == k) return t;
  }
  assert(false && "missing token");
  return toks.front();
}

int main() {
  {
    auto t = First(Tokenize("0xFF"), TokKind::IntLit);
    assert(t.intval == 255);
    assert(t.line == 1);
    assert(t.col == 1);
  }
  {
    auto t = First(Tokenize("0o10"), TokKind::IntLit);
    assert(t.intval == 8);
  }
  {
    auto t = First(Tokenize("0b1010"), TokKind::IntLit);
    assert(t.intval == 10);
  }
  {
    auto t = First(Tokenize("010"), TokKind::IntLit);
    assert(t.intval == 8);
  }
  {
    bool threw = false;
    try {
      Tokenize("08");
    } catch (const LexError&) {
      threw = true;
    }
    assert(threw);
  }
  {
    auto t = First(Tokenize("2i"), TokKind::ImagLit);
    assert(t.floatval > 1.99 && t.floatval < 2.01);
  }
  {
    auto t = First(Tokenize(".5i"), TokKind::ImagLit);
    assert(t.floatval > 0.49 && t.floatval < 0.51);
  }
  {
    auto t = First(Tokenize("\"\\u0041\""), TokKind::StringLit);
    assert(t.text == "A");
  }
  {
    auto t = First(Tokenize("\"\\U00000041\""), TokKind::StringLit);
    assert(t.text == "A");
  }
  {
    auto t = First(Tokenize("\"\\x41\""), TokKind::StringLit);
    assert(t.text == "A");
  }
  {
    auto t = First(Tokenize("'\\a'"), TokKind::RuneLit);
    assert(t.intval == 7);
  }
  {
    auto t = First(Tokenize("'\\u03A9'"), TokKind::RuneLit);
    assert(t.intval == 0x03A9);
  }
  {
    auto t = First(Tokenize("\"\\U0001F600\""), TokKind::StringLit);
    assert(t.text.size() == 4);
    assert(static_cast<unsigned char>(t.text[0]) == 0xF0);
  }
  {
    auto t = First(Tokenize("0x1p3"), TokKind::FloatLit);
    assert(t.floatval > 7.99 && t.floatval < 8.01);
  }
  {
    auto t = First(Tokenize("0x1.fp+3"), TokKind::FloatLit);
    assert(t.floatval > 15.4 && t.floatval < 15.6);
  }
  {
    auto t = First(Tokenize("1."), TokKind::FloatLit);
    assert(t.floatval > 0.99 && t.floatval < 1.01);
  }
  {
    auto t = First(Tokenize("1.e2"), TokKind::FloatLit);
    assert(t.floatval > 99.9 && t.floatval < 100.1);
  }
  {
    auto t = First(Tokenize("1_000"), TokKind::IntLit);
    assert(t.intval == 1000);
  }
  {
    bool threw = false;
    try {
      Tokenize("1_");
    } catch (const LexError&) {
      threw = true;
    }
    assert(threw);
  }
  {
    bool threw = false;
    try {
      Tokenize("1__2");
    } catch (const LexError&) {
      threw = true;
    }
    assert(threw);
  }
  {
    bool threw = false;
    try {
      Tokenize("\"\\400\"");
    } catch (const LexError&) {
      threw = true;
    }
    assert(threw);
  }

  {
    const char* src =
        "package main\n"
        "func main() {\n"
        "  var n int\n"
        "  n.Nope()\n"
        "}\n";
    File f = Parse(Tokenize(src));
    f.path = "bad.go";
    bool threw = false;
    std::string msg;
    try {
      GenerateCpp(f);
    } catch (const GenError& e) {
      threw = true;
      msg = e.what();
    }
    if (!threw) {
      std::cerr << "expected codegen error for n.Nope()\n";
      return 1;
    }
    if (msg.find("bad.go:4:") == std::string::npos || msg.find("Nope") == std::string::npos) {
      std::cerr << "codegen error missing span: " << msg << "\n";
      return 1;
    }
  }

  {
    const char* src =
        "package main\n"
        "type T struct {\n"
        "  Name string `json:\"name\"`\n"
        "  Skip int `json:\"-\"`\n"
        "}\n";
    File f = Parse(Tokenize(src));
    assert(f.structs.size() == 1);
    assert(f.structs[0].fields.size() == 2);
    assert(f.structs[0].fields[0].tag.find("json:") != std::string::npos);
    assert(f.structs[0].fields[1].tag.find("-") != std::string::npos);
  }

  {
    const char* src =
        "package main\n"
        "const N = 4\n"
        "func main() {\n"
        "  var a [N]int\n"
        "  var b [1 << 3]byte\n"
        "  _ = a\n"
        "  _ = b\n"
        "}\n";
    File f = Parse(Tokenize(src));
    f.path = "arr.go";
    try {
      (void)GenerateCpp(f);
    } catch (const GenError& e) {
      std::cerr << "array length codegen failed: " << e.what() << "\n";
      return 1;
    }
  }

  {
    const char* src =
        "package main\n"
        "type Duration int64\n"
        "func (d Duration) String() string { return \"x\" }\n"
        "func main() {\n"
        "  var d Duration\n"
        "  _ = d.String()\n"
        "}\n";
    File f = Parse(Tokenize(src));
    f.path = "duration.go";
    try {
      (void)GenerateCpp(f);
    } catch (const GenError& e) {
      std::cerr << "defined-type method codegen failed: " << e.what() << "\n";
      return 1;
    }
  }

  std::cout << "frontend_test ok\n";
  return 0;
}
