#include "cpp_generator.h"

#include <cctype>
#include <cstdio>
#include <fstream>
#include <functional>
#include <map>
#include <memory>
#include <set>
#include <sstream>
#include <unordered_map>
#include <vector>

#ifndef WASIGO_RUNTIME_PATH
#define WASIGO_RUNTIME_PATH "src/runtime.hpp"
#endif

namespace wasigo {

namespace {

bool IsBuiltinTypeName(const std::string& n) {
  static const std::set<std::string> kNames = {
      "string", "int",   "int8",   "int16",  "int32",  "int64",
      "uint",   "uint8", "uint16", "uint32", "uint64", "float32",
      "float64", "byte", "rune",   "bool",   "uintptr", "complex64", "complex128",
  };
  return kNames.count(n) != 0;
}

bool IsCppKeyword(const std::string& n) {
  static const std::set<std::string> kWords = {
      "alignas",     "alignof",      "and",         "and_eq",
      "asm",         "auto",         "bitand",      "bitor",
      "bool",        "break",        "case",        "catch",
      "char",        "char8_t",      "char16_t",    "char32_t",
      "class",       "compl",        "concept",     "const",
      "consteval",   "constexpr",    "constinit",   "const_cast",
      "continue",    "co_await",     "co_return",   "co_yield",
      "decltype",    "default",      "delete",      "do",
      "double",      "dynamic_cast", "else",        "enum",
      "explicit",    "export",       "extern",      "false",
      "float",       "for",          "friend",      "goto",
      "if",          "inline",       "int",         "long",
      "mutable",     "namespace",    "new",         "noexcept",
      "not",         "not_eq",       "nullptr",     "operator",
      "or",          "or_eq",        "private",     "protected",
      "public",      "register",     "reinterpret_cast", "requires",
      "return",      "short",        "signed",      "sizeof",
      "static",      "static_assert","static_cast", "struct",
      "switch",      "template",     "this",        "thread_local",
      "throw",       "true",         "try",         "typedef",
      "typeid",      "typename",     "union",       "unsigned",
      "using",       "virtual",      "void",        "volatile",
      "wchar_t",     "while",        "xor",         "xor_eq",
  };
  return kWords.count(n) != 0;
}

std::string CppIdent(const std::string& n) {
  if (n.empty() || n == "_") return n;
  if (IsCppKeyword(n)) return n + "_";
  // Common libc macros/globals that smash Go names (io.EOF; `package log`
  // as a bare `namespace log {` collides with global ::log from <cmath> --
  // MSVC hard-errors on that, "'log': a symbol with this name already
  // exists and therefore this name cannot be used as a namespace name").
  if (n == "EOF" || n == "NULL" || n == "errno" || n == "stdin" || n == "stdout" ||
      n == "stderr" || n == "time" || n == "stat" || n == "random" || n == "log" ||
      n == "rand" || n == "signal" || n == "syscall") {
    return n + "_";
  }
  return n;
}

bool IsComparisonOrLogicalOp(const std::string& op) {
  return op == "==" || op == "!=" || op == "<" || op == "<=" || op == ">" ||
         op == ">=" || op == "&&" || op == "||";
}

std::string FormatDouble(double v) {
  std::ostringstream oss;
  oss.precision(17);
  oss << v;
  std::string s = oss.str();
  if (s.find('.') == std::string::npos && s.find('e') == std::string::npos &&
      s.find("inf") == std::string::npos && s.find("nan") == std::string::npos) {
    s += ".0";
  }
  return s;
}

std::string EscapeCppStringLiteral(const std::string& s) {
  std::string out = "\"";
  for (unsigned char c : s) {
    switch (c) {
      case '"': out += "\\\""; break;
      case '\\': out += "\\\\"; break;
      case '\n': out += "\\n"; break;
      case '\t': out += "\\t"; break;
      case '\r': out += "\\r"; break;
      default:
        if (c < 0x20 || c == 0x7f) {
          char buf[8];
          std::snprintf(buf, sizeof(buf), "\\x%02x", c);
          out += buf;
        } else {
          out += static_cast<char>(c);
        }
    }
  }
  out += "\"";
  return out;
}

// ---------------------------------------------------------------------------

// A synthetic `File` (in the ast.h sense: one parsed .go file's worth of
// decls) standing in for the "os" package's File type and its
// filesystem-touching functions. `os` is one of the three builtin packages
// (fmt/errors/os -- see module_loader.cc's IsBuiltinImport), so unlike
// every other package there's no real stdlib/os/*.go for the module loader
// to parse. Wiring these into the same StructDecl/FuncDecl shape everything
// else uses means LookupStruct/LookupMethod/LookupFreeFunc/
// ResolveCalledFunc all work on `os.File`/`os.Open`/etc. unmodified --
// multi-return unpacking (`f, err := os.Open(...)`) in particular depends
// on ResolveCalledFunc finding a real FuncDecl. The actual C++ (`wasigo::
// os_open`, `wasigo::File`) is hand-written in runtime.hpp, since a method
// body that calls raw `std::fread`/`std::fwrite` can't be expressed as Go
// source for EmitMethodInline to translate -- see NamedCppType's os.File
// case and EmitCall's "os" branch for where the two meet.
File BuildOsBuiltinFile() {
  File f;
  f.package_name = "os";

  StructDecl sd;
  sd.name = "File";
  f.structs.push_back(std::move(sd));

  // os.FileInfo / os.DirEntry: like File above, these have no Go-source
  // method bodies -- LookupMethod finds the FuncDecl (so `fi.IsDir()`
  // resolves and type-checks) and NamedCppType maps the receiver straight
  // to a hand-written wasigo::FileInfo/DirEntry struct (see runtime.hpp)
  // that supplies the real implementation, the same split File's
  // Read/Write/Close already use.
  StructDecl fi_sd;
  fi_sd.name = "FileInfo";
  f.structs.push_back(std::move(fi_sd));

  StructDecl de_sd;
  de_sd.name = "DirEntry";
  f.structs.push_back(std::move(de_sd));

  auto byteSlice = [] {
    auto t = std::make_unique<TypeNode>();
    t->kind = TypeKind::Slice;
    t->elem = MakeNamedType("byte");
    return t;
  };
  auto results2 = [](std::unique_ptr<TypeNode> a, std::unique_ptr<TypeNode> b) {
    std::vector<std::unique_ptr<TypeNode>> v;
    v.push_back(std::move(a));
    v.push_back(std::move(b));
    return v;
  };
  auto results1 = [](std::unique_ptr<TypeNode> a) {
    std::vector<std::unique_ptr<TypeNode>> v;
    v.push_back(std::move(a));
    return v;
  };
  auto param = [](std::string name, std::unique_ptr<TypeNode> t) {
    Param p;
    p.name = std::move(name);
    p.type = std::move(t);
    return p;
  };

  FuncDecl read;
  read.has_receiver = true;
  read.receiver_name = "f";
  read.receiver_type = "File";
  read.receiver_is_pointer = true;
  read.name = "Read";
  read.params.push_back(param("p", byteSlice()));
  read.results = results2(MakeNamedType("int"), MakeNamedType("error"));
  f.funcs.push_back(std::move(read));

  FuncDecl write;
  write.has_receiver = true;
  write.receiver_name = "f";
  write.receiver_type = "File";
  write.receiver_is_pointer = true;
  write.name = "Write";
  write.params.push_back(param("p", byteSlice()));
  write.results = results2(MakeNamedType("int"), MakeNamedType("error"));
  f.funcs.push_back(std::move(write));

  FuncDecl close;
  close.has_receiver = true;
  close.receiver_name = "f";
  close.receiver_type = "File";
  close.receiver_is_pointer = true;
  close.name = "Close";
  close.results = results1(MakeNamedType("error"));
  f.funcs.push_back(std::move(close));

  FuncDecl open;
  open.name = "Open";
  open.params.push_back(param("name", MakeNamedType("string")));
  open.results = results2(MakeNamedType("File", "os"), MakeNamedType("error"));
  f.funcs.push_back(std::move(open));

  FuncDecl create;
  create.name = "Create";
  create.params.push_back(param("name", MakeNamedType("string")));
  create.results = results2(MakeNamedType("File", "os"), MakeNamedType("error"));
  f.funcs.push_back(std::move(create));

  FuncDecl read_file;
  read_file.name = "ReadFile";
  read_file.params.push_back(param("name", MakeNamedType("string")));
  read_file.results = results2(byteSlice(), MakeNamedType("error"));
  f.funcs.push_back(std::move(read_file));

  FuncDecl write_file;
  write_file.name = "WriteFile";
  write_file.params.push_back(param("name", MakeNamedType("string")));
  write_file.params.push_back(param("data", byteSlice()));
  write_file.params.push_back(param("perm", MakeNamedType("int")));
  write_file.results = results1(MakeNamedType("error"));
  f.funcs.push_back(std::move(write_file));

  auto method0 = [&](const char* recv_type, const char* name, std::unique_ptr<TypeNode> result) {
    FuncDecl m;
    m.has_receiver = true;
    m.receiver_name = "x";
    m.receiver_type = recv_type;
    m.receiver_is_pointer = false;
    m.name = name;
    m.results = results1(std::move(result));
    f.funcs.push_back(std::move(m));
  };
  method0("FileInfo", "Name", MakeNamedType("string"));
  method0("FileInfo", "Size", MakeNamedType("int64"));
  method0("FileInfo", "IsDir", MakeNamedType("bool"));
  method0("DirEntry", "Name", MakeNamedType("string"));
  method0("DirEntry", "IsDir", MakeNamedType("bool"));

  FuncDecl stat;
  stat.name = "Stat";
  stat.params.push_back(param("name", MakeNamedType("string")));
  stat.results = results2(MakeNamedType("FileInfo", "os"), MakeNamedType("error"));
  f.funcs.push_back(std::move(stat));

  FuncDecl read_dir;
  read_dir.name = "ReadDir";
  read_dir.params.push_back(param("name", MakeNamedType("string")));
  auto dirEntrySlice = [] {
    auto t = std::make_unique<TypeNode>();
    t->kind = TypeKind::Slice;
    t->elem = MakeNamedType("DirEntry", "os");
    return t;
  };
  read_dir.results = results2(dirEntrySlice(), MakeNamedType("error"));
  f.funcs.push_back(std::move(read_dir));

  return f;
}

// gocvm: the one dispatch gate to a native host bridge (see
// wasigo::gocvm::Call in runtime.hpp and docs/design-log.md). Like `os`,
// this is a builtin with no real stdlib/gocvm/*.go source -- Call is the
// single function, wired straight to wasigo::gocvm::Call by EmitCall's
// "gocvm" branch, the same way os.Getenv routes to wasigo::os_getenv.
File BuildGocvmBuiltinFile() {
  File f;
  f.package_name = "gocvm";

  FuncDecl call;
  call.name = "Call";
  call.params.push_back(Param{"topic", MakeNamedType("string")});
  call.params.push_back(Param{"payload", MakeNamedType("string")});
  call.results.push_back(MakeNamedType("string"));
  call.results.push_back(MakeNamedType("error"));
  f.funcs.push_back(std::move(call));

  return f;
}

// reflect: Value and Type are both, under the hood, exactly wasigo::Any
// (see NamedCppType's `t->pkg == "reflect"` case) -- Kind/Name/NumField/
// Field/FieldName/Interface/Int/Float/Bool/String/Type are real C++
// methods on Any itself (see runtime.hpp), so a method call on a Value/
// Type routes through the ordinary struct-method emission path with no
// special-casing beyond that type mapping. Only the two free functions
// (TypeOf/ValueOf) need EmitCall's help, the same way os.Getenv does --
// see EmitCall's `pkg == "reflect"` branch.
//
// Scope: SetInt/SetString/SetBool/SetFloat/Len/Index are real. Field()
// aliases struct memory via adapt_ptr so encoding/json can Unmarshal into
// a struct pointer. Slice Kind is bound in finish_any_kind after Slice<T>.
File BuildReflectBuiltinFile() {
  File f;
  f.package_name = "reflect";

  StructDecl value_sd;
  value_sd.name = "Value";
  f.structs.push_back(std::move(value_sd));

  StructDecl type_sd;
  type_sd.name = "Type";
  f.structs.push_back(std::move(type_sd));

  auto param = [](std::string name, std::unique_ptr<TypeNode> t) {
    Param p;
    p.name = std::move(name);
    p.type = std::move(t);
    return p;
  };
  auto results1 = [](std::unique_ptr<TypeNode> a) {
    std::vector<std::unique_ptr<TypeNode>> v;
    v.push_back(std::move(a));
    return v;
  };
  auto params1 = [](Param p) {
    std::vector<Param> v;
    v.push_back(std::move(p));
    return v;
  };
  auto anySlice = [] {
    auto t = std::make_unique<TypeNode>();
    t->kind = TypeKind::Slice;
    t->elem = MakeNamedType("any");
    return t;
  };
  auto method = [&](const char* recv_type, const char* name, std::vector<Param> params,
                     std::unique_ptr<TypeNode> result) {
    FuncDecl m;
    m.has_receiver = true;
    m.receiver_name = "v";
    m.receiver_type = recv_type;
    m.receiver_is_pointer = false;
    m.name = name;
    m.params = std::move(params);
    if (result) m.results = results1(std::move(result));
    f.funcs.push_back(std::move(m));
  };

  for (const char* recv : {"Value", "Type"}) {
    method(recv, "Kind", {}, MakeNamedType("int"));
    method(recv, "Name", {}, MakeNamedType("string"));
    method(recv, "String", {}, MakeNamedType("string"));
  }
  method("Value", "NumField", {}, MakeNamedType("int"));
  method("Value", "Field", params1(param("i", MakeNamedType("int"))), MakeNamedType("Value", "reflect"));
  method("Value", "FieldName", params1(param("i", MakeNamedType("int"))), MakeNamedType("string"));
  method("Value", "Interface", {}, MakeNamedType("any"));
  method("Value", "Int", {}, MakeNamedType("int64"));
  method("Value", "Float", {}, MakeNamedType("float64"));
  method("Value", "Bool", {}, MakeNamedType("bool"));
  method("Value", "Type", {}, MakeNamedType("Type", "reflect"));
  method("Value", "CanSet", {}, MakeNamedType("bool"));
  method("Value", "SetInt", params1(param("n", MakeNamedType("int64"))), nullptr);
  method("Value", "SetFloat", params1(param("n", MakeNamedType("float64"))), nullptr);
  method("Value", "SetBool", params1(param("b", MakeNamedType("bool"))), nullptr);
  method("Value", "SetString", params1(param("s", MakeNamedType("string"))), nullptr);
  method("Value", "SetSlice", params1(param("elems", anySlice())), MakeNamedType("bool"));
  method("Value", "Len", {}, MakeNamedType("int"));
  method("Value", "Index", params1(param("i", MakeNamedType("int"))), MakeNamedType("Value", "reflect"));

  FuncDecl type_of;
  type_of.name = "TypeOf";
  type_of.params.push_back(param("i", MakeNamedType("any")));
  type_of.results = results1(MakeNamedType("Type", "reflect"));
  f.funcs.push_back(std::move(type_of));

  FuncDecl value_of;
  value_of.name = "ValueOf";
  value_of.params.push_back(param("i", MakeNamedType("any")));
  value_of.results = results1(MakeNamedType("Value", "reflect"));
  f.funcs.push_back(std::move(value_of));

  // Kind constants (reflect.Bool, reflect.Struct, ...) are NOT declared
  // here as GlobalVarDecls -- there's no real generated reflect.hpp for
  // QualName("reflect", name) to reference (this whole File only exists
  // for LookupMethod/LookupFreeFunc, never for actual emission), so a
  // GlobalVarDecl here would be dead: nothing would ever emit real C++
  // for it. They're handled directly in InferType's Selector case and
  // EmitSelector instead -- see IsReflectKindName.

  return f;
}

class Generator {
 public:
  Generator(const File& file, GenOptions opt) : file_(file), opt_(std::move(opt)) {}

  std::string Run() {
    for (auto& g : file_.globals) {
      const TypeNode* t = g.type ? g.type.get() : InferType(g.init.get());
      if (!t) {
        Error("cannot infer a type for global '" + g.name +
              "'; give it an explicit type");
      }
      globals_[g.name] = t;
    }
    for (auto& fn : file_.funcs) {
      if (fn.results.size() > 1) result_struct_names_.insert(ResultStructName(fn));
    }

    bool has_main = false;
    for (auto& fn : file_.funcs) {
      if (!fn.has_receiver && fn.name == "main") has_main = true;
    }
    if (!opt_.library && !has_main) {
      Error("no 'func main()' found in package '" + file_.package_name +
            "' -- a program entry needs main; a library file is package != main");
    }
    if (opt_.library && has_main) {
      Error("package '" + file_.package_name + "' is a library and cannot define func main");
    }

    // A method's receiver is a struct, or a defined type (`type Duration
    // int64`) that we emit as a distinct C++ struct so the method set has
    // somewhere to attach. True aliases (`type T = U`) cannot have methods.
    for (auto& fn : file_.funcs) {
      if (!fn.has_receiver) continue;
      if (LookupStruct(fn.receiver_type)) continue;
      const TypeAlias* a = LookupAlias(fn.receiver_type);
      if (a && !a->is_alias_eq) continue;
      Error("method '" + fn.name + "' has receiver type '" + fn.receiver_type +
            "', which is not a struct or defined type declared in this package");
    }

    AnalyzeAsync();
    BuildImportAliases();

    out_ << "// Generated by wasigoc. Do not edit by hand.\n";
    if (opt_.library) {
      out_ << "#pragma once\n";
      for (auto& h : opt_.imported_headers) {
        out_ << "#include \"" << h << "\"\n";
      }
      if (!opt_.imported_headers.empty()) out_ << "\n";
    } else {
      out_ << "#define WASIGO_GENERATED 1\n";
      if (NeedCoro()) out_ << "#define WASIGO_NEED_CORO 1\n";
      out_ << LoadRuntime() << "\n";
      for (auto& h : opt_.imported_headers) {
        out_ << "#include \"" << h << "\"\n";
      }
      if (!opt_.imported_headers.empty()) out_ << "\n";
    }

    EmitNsOpen();
    EmitAliases();
    // Struct forward decls before interface defs, not after: an
    // interface method whose signature uses a plain STRUCT type (not a
    // pointer, not a builtin) -- e.g. `Bounds() Rectangle` in image.Image
    // -- needs `Rectangle` at least forward-declared for the VTable's
    // function-pointer member (`Rectangle (*Bounds)(void*);`) to parse at
    // all; getting this backwards read as "expected identifier before
    // '*' token", since `Rectangle` wasn't ANY recognized name yet.
    // Every earlier interface tested only used builtin types or OTHER
    // interfaces in its methods (hash.Hash, cipher.Block, color.Model),
    // never a plain struct -- found building image.Image. A forward
    // declaration is all EmitInterfaceDefs needs (it only ever uses the
    // type as a return/parameter type, never requires a complete type),
    // so simply reordering these two calls is sufficient.
    EmitStructForwardDecls();
    EmitInterfaceDefs();
    EmitFreeFuncPrototypes();
    // Struct field-only skeletons before result-struct defs, not after: a
    // multi-return function whose result struct holds a plain STRUCT type
    // BY VALUE (e.g. `func Foo() (Header, error)` -> `struct FooResult {
    // Header r0{}; ... }`) needs `Header` at least field-complete right
    // here -- a forward declaration (what EmitStructForwardDecls gives it)
    // is not enough for a by-value member, unlike EmitInterfaceDefs/
    // EmitFreeFuncPrototypes above, which only ever use such types as
    // return/parameter types (forward decl is enough there, see the
    // comment on EmitStructForwardDecls/EmitInterfaceDefs's own ordering).
    // Every earlier multi-return function returned a pointer or a builtin
    // in that slot, never a plain named struct by value -- found porting
    // uniloader's bundle package (decodeHeader() (Header, error),
    // Bundle.Get() (File, error)).
    EmitStructDefs();
    EmitResultStructDefs();
    EmitSimpleConstDecls();
    EmitGlobalForwardDecls();
    // See EmitMethodOutOfLine's own comment: a struct method's body needs
    // every OTHER struct type it touches complete, which only just
    // became true (every struct now has its field-only skeleton).
    EmitAllMethodDefs();
    // See FlushDeferredIfaceMethodDefs's own comment: an interface
    // method's forwarding body needs every struct type it touches
    // complete too.
    FlushDeferredIfaceMethodDefs();
    EmitGlobalDecls();
    EmitFreeFuncDefs();
    EmitPackageInit();
    EmitNsClose();

    return out_.str();
  }

 private:
  const File& file_;
  GenOptions opt_;
  std::ostringstream out_;
  int indent_ = 0;
  mutable int err_line_ = 0;
  mutable int err_col_ = 0;

  std::vector<std::unique_ptr<TypeNode>> synth_types_;
  std::vector<std::map<std::string, const TypeNode*>> scopes_;
  std::vector<std::map<std::string, const Expr*>> const_inits_;
  std::map<std::string, const TypeNode*> globals_;
  std::set<std::string> result_struct_names_;
  const FuncDecl* current_func_ = nullptr;
  // Set only while emitting a func LITERAL's own body (EmitFuncLitToString)
  // -- current_func_ itself is deliberately left pointing at the ENCLOSING
  // named function throughout (many places, e.g. EmitExprAs's interface
  // adaptation, don't need to change for a nested literal), so EmitReturn's
  // "what does a bare `return` mean here" logic needs its OWN, separate
  // signal for "we're actually inside the literal's body, not the
  // enclosing function's" -- without this, a func literal assigned to a
  // local var (`f := func() { ...; return; ... }`) written lexically
  // inside `main` had its own bare `return` emit `return 0;` (main's own
  // convention) instead of a plain `return;`, and a literal written inside
  // a function with NAMED results would wrongly try to return those.
  bool in_func_lit_ = false;
  const std::vector<std::unique_ptr<TypeNode>>* func_lit_results_ = nullptr;
  // See UnscopedFile()'s comment: normally empty, temporarily set only
  // around the one recursive InferType call that re-infers a cross-package
  // global's un-typed initializer.
  std::string unscoped_lookup_pkg_;
  // An interface method's forwarding body (`vt->Bounds(self.get())`) is
  // buffered here instead of written inline in the interface struct at
  // EmitInterfaceDefs time, then flushed by FlushDeferredIfaceMethodDefs
  // right after EmitStructDefs -- see that flush call's comment in Run()
  // for why.
  std::ostringstream deferred_iface_method_defs_;
  int temp_id_ = 0;
  std::set<std::string> async_free_;
  std::set<std::string> async_methods_;
  bool program_has_go_ = false;
  bool current_async_ = false;
  bool current_has_defers_ = false;
  struct JumpFrame {
    std::string name;
    std::string brk;
    std::string cont;
    bool is_loop = false;
    bool range_func = false;
    // Only set on a range_func frame: the shared flag/value locals a
    // `return` inside its yield lambda stashes into before stopping
    // iteration (`return false`), so the real C++ return can happen once
    // control is back at an ordinary statement position -- see
    // EmitRangeFuncReturn/EmitRangeOverFunc's post-call check. A range_func
    // frame nested inside another one inherits its nearest enclosing
    // range_func frame's own vars instead of declaring fresh ones -- one
    // shared escape path all the way out, matching real Go's own
    // return-inside-range-over-func desugaring.
    std::string rf_ret_var;
    std::string rf_val_var;
  };
  std::vector<JumpFrame> jump_stack_;
  std::string pending_label_;
  std::vector<std::string> current_type_params_;
  std::map<std::string, std::string> pkg_alias_;

  static std::string LoadRuntime() {
    std::ifstream in(WASIGO_RUNTIME_PATH, std::ios::binary);
    if (!in) {
      throw GenError(std::string("cannot read wasigo runtime at '") + WASIGO_RUNTIME_PATH + "'");
    }
    std::ostringstream ss;
    ss << in.rdbuf();
    std::string s = ss.str();
    const std::string pragma = "#pragma once";
    auto pos = s.find(pragma);
    if (pos != std::string::npos) s.replace(pos, pragma.size(), "/* wasigo runtime */");
    return s;
  }

  static std::string MethodKey(const std::string& type, const std::string& name) {
    return type + "." + name;
  }

  bool IsAsyncFree(const std::string& name) const { return async_free_.count(name) != 0; }
  bool IsAsyncMethod(const std::string& type, const std::string& name) const {
    return async_methods_.count(MethodKey(type, name)) != 0;
  }

  [[noreturn]] void Error(const std::string& msg) const {
    std::string loc;
    if (!file_.path.empty() && err_line_ > 0) {
      loc = file_.path + ":" + std::to_string(err_line_) + ":" +
            std::to_string(err_col_) + ": ";
    } else if (err_line_ > 0) {
      loc = std::to_string(err_line_) + ":" + std::to_string(err_col_) + ": ";
    }
    throw GenError(loc + msg);
  }

  void NoteLoc(const Expr& e) const {
    if (e.line > 0) {
      err_line_ = e.line;
      err_col_ = e.col;
    }
  }
  void NoteLoc(const Stmt& s) const {
    if (s.line > 0) {
      err_line_ = s.line;
      err_col_ = s.col;
    }
  }
  void NoteLoc(const Expr* e) const {
    if (e) NoteLoc(*e);
  }

  static bool IsBlank(const Expr& e) { return e.kind == ExprKind::Ident && e.strval == "_"; }

  static bool IsComplexName(const std::string& n) {
    return n == "complex64" || n == "complex128";
  }
  bool IsComplexType(const TypeNode* t) const {
    t = ResolveUnderlying(t);
    return t && t->kind == TypeKind::Named && t->pkg.empty() && IsComplexName(t->name);
  }
  static bool IsIntegerName(const std::string& n) {
    return n == "int" || n == "int8" || n == "int16" || n == "int32" || n == "int64" ||
           n == "uint" || n == "uint8" || n == "uint16" || n == "uint32" || n == "uint64" ||
           n == "uintptr" || n == "byte" || n == "rune";
  }
  bool IsIntegerType(const TypeNode* t) const {
    t = ResolveUnderlying(t);
    return t && t->kind == TypeKind::Named && t->pkg.empty() && IsIntegerName(t->name);
  }
  bool IsFloat32Type(const TypeNode* t) const {
    t = ResolveUnderlying(t);
    return t && t->kind == TypeKind::Named && t->pkg.empty() && t->name == "float32";
  }

  static std::string ReflectFieldName(const FieldDecl& f) {
    const std::string& tag = f.tag;
    auto p = tag.find("json:\"");
    if (p == std::string::npos) return f.name;
    p += 6;
    auto q = tag.find('"', p);
    if (q == std::string::npos) return f.name;
    std::string spec = tag.substr(p, q - p);
    if (spec == "-") return "";
    auto comma = spec.find(',');
    if (comma != std::string::npos) spec = spec.substr(0, comma);
    if (spec.empty()) return f.name;
    return spec;
  }

  std::string Indent() const { return std::string(static_cast<size_t>(indent_) * 2, ' '); }

  // ---- scope / type bookkeeping -------------------------------------------

  void PushScope() {
    scopes_.emplace_back();
    const_inits_.emplace_back();
  }
  void PopScope() {
    scopes_.pop_back();
    const_inits_.pop_back();
  }
  void Declare(const std::string& name, const TypeNode* type) {
    if (!scopes_.empty()) scopes_.back()[name] = type;
  }
  // Go's `:=` reuses (assigns to) any name already declared in the SAME
  // block scope, as long as at least one name on the left is new -- it
  // only redeclares fresh variables for names not yet in this exact
  // scope. Every multi-name ':=' emission site needs this check before
  // emitting a C++ declaration, or reusing a name (`size, err := ...`
  // then `data, err := ...` in the same function) emits two C++
  // declarations of `err` and fails to compile ("redeclaration of ...").
  bool DeclaredInCurrentScope(const std::string& name) const {
    return !scopes_.empty() && scopes_.back().count(name) != 0;
  }
  const Expr* LookupConstInit(const std::string& name, int* iota_out = nullptr) const {
    for (auto it = const_inits_.rbegin(); it != const_inits_.rend(); ++it) {
      auto found = it->find(name);
      if (found != it->end()) {
        if (iota_out) *iota_out = 0;
        return found->second;
      }
    }
    const GlobalVarDecl* g = LookupGlobalDecl(name);
    if (g && g->is_const && g->init) {
      if (iota_out) *iota_out = g->iota_value;
      return g->init.get();
    }
    return nullptr;
  }
  const TypeNode* Lookup(const std::string& name) const {
    for (auto it = scopes_.rbegin(); it != scopes_.rend(); ++it) {
      auto found = it->find(name);
      if (found != it->end()) return found->second;
    }
    auto git = globals_.find(name);
    if (git != globals_.end()) return git->second;
    return nullptr;
  }
  const TypeNode* SynthNamed(const std::string& name, const std::string& pkg = "") {
    synth_types_.push_back(MakeNamedType(name, pkg));
    return synth_types_.back().get();
  }
  const TypeNode* SynthPointer(const TypeNode* elem) {
    auto t = std::make_unique<TypeNode>();
    t->kind = TypeKind::Pointer;
    t->elem = CloneType(elem);
    synth_types_.push_back(std::move(t));
    return synth_types_.back().get();
  }
  const TypeNode* SynthSlice(const TypeNode* elem) {
    auto t = std::make_unique<TypeNode>();
    t->kind = TypeKind::Slice;
    t->elem = CloneType(elem);
    synth_types_.push_back(std::move(t));
    return synth_types_.back().get();
  }
  const TypeNode* SynthMap(const TypeNode* key, const TypeNode* elem) {
    auto t = std::make_unique<TypeNode>();
    t->kind = TypeKind::Map;
    t->key = CloneType(key);
    t->elem = CloneType(elem);
    synth_types_.push_back(std::move(t));
    return synth_types_.back().get();
  }
  const TypeNode* SynthChan(const TypeNode* elem) {
    auto t = std::make_unique<TypeNode>();
    t->kind = TypeKind::Chan;
    t->elem = CloneType(elem);
    synth_types_.push_back(std::move(t));
    return synth_types_.back().get();
  }
  const TypeNode* SynthFuncType(const std::vector<Param>& params,
                                const std::vector<std::unique_ptr<TypeNode>>& results) {
    auto t = std::make_unique<TypeNode>();
    t->kind = TypeKind::Func;
    for (auto& p : params) {
      Param np;
      np.name = p.name;
      np.type = CloneType(p.type.get());
      np.variadic = p.variadic;
      t->func_params.push_back(std::move(np));
    }
    for (auto& r : results) t->func_results.push_back(CloneType(r.get()));
    synth_types_.push_back(std::move(t));
    return synth_types_.back().get();
  }

  const File* FindPackage(const std::string& pkg) const {
    if (pkg.empty() || pkg == file_.package_name) return &file_;
    if (pkg == "os") {
      static const File kOsFile = BuildOsBuiltinFile();
      return &kOsFile;
    }
    if (pkg == "reflect") {
      static const File kReflectFile = BuildReflectBuiltinFile();
      return &kReflectFile;
    }
    if (pkg == "gocvm") {
      static const File kGocvmFile = BuildGocvmBuiltinFile();
      return &kGocvmFile;
    }
    for (const File* f : opt_.imported_files) {
      if (f && f->package_name == pkg) return f;
    }
    return nullptr;
  }

  static std::string LastPathComp(const std::string& p) {
    std::string s = p;
    while (!s.empty() && (s.back() == '/' || s.back() == '\\')) s.pop_back();
    size_t slash = s.find_last_of("/\\");
    if (slash != std::string::npos) s = s.substr(slash + 1);
    if (s.size() > 3 && s.compare(s.size() - 3, 3, ".go") == 0) s = s.substr(0, s.size() - 3);
    return s;
  }

  void BuildImportAliases() {
    for (auto& spec : file_.import_specs) {
      if (spec.local == "_") continue;
      std::string dest = spec.path;
      if (dest == "fmt" || dest == "errors" || dest == "os" || dest == "reflect" ||
          dest == "gocvm") {
        std::string local = spec.local.empty() ? dest : spec.local;
        pkg_alias_[local] = dest;
        continue;
      }
      dest = LastPathComp(spec.path);
      for (const File* f : opt_.imported_files) {
        if (f && (f->package_name == dest || f->package_name == spec.local)) {
          dest = f->package_name;
          break;
        }
      }
      std::string local = spec.local.empty() ? dest : spec.local;
      pkg_alias_[local] = dest;
    }
  }

  std::string PkgOf(const std::string& local) const {
    auto it = pkg_alias_.find(local);
    if (it != pkg_alias_.end()) return it->second;
    return local;
  }

  bool IsImportedPackage(const std::string& name) const {
    const std::string n = PkgOf(name);
    if (n == "fmt" || n == "errors" || n == "os" || n == "reflect" || n == "gocvm") return false;
    return FindPackage(n) != nullptr && n != file_.package_name;
  }

  static std::string NamespacePrefix(const std::string& pkg) {
    if (pkg.empty()) return "";
    std::string prefix;
    std::string cur;
    for (char c : pkg) {
      if (c == '.') {
        prefix += CppIdent(cur);
        prefix += "::";
        cur.clear();
      } else {
        cur.push_back(c);
      }
    }
    if (!cur.empty()) {
      prefix += CppIdent(cur);
      prefix += "::";
    }
    return prefix;
  }

  std::string QualName(const std::string& pkg, const std::string& name) const {
    std::string n = CppIdent(name);
    if (pkg.empty() || pkg == file_.package_name) return n;
    return NamespacePrefix(pkg) + n;
  }

  // An unqualified name ALWAYS means the current package in Go (no dot
  // imports here -- see the parser's explicit rejection of `import .`), so
  // every Lookup* below with a `pkg`-empty case must only ever search
  // `file_`, never any *other* package in `opt_.imported_files` (which
  // holds every other package in the whole program, not just same-package
  // files -- module_loader already merges a package directory's *.go files
  // into one File before this runs). Falling through to imported_files
  // used to cause a real cross-package name collision: two unrelated
  // packages each declaring a type with the same bare name (e.g. two
  // different `Reader` structs) could resolve a same-package unqualified
  // reference to the WRONG package's type, entirely silently.
  static const StructDecl* LookupStructIn(const File* f, const std::string& name) {
    if (!f) return nullptr;
    for (auto& s : f->structs) {
      if (s.name == name) return &s;
    }
    return nullptr;
  }

  // Normally just `&file_` (the file currently being generated) -- an
  // unqualified name always means the current package, per the comment
  // above. But ONE call site is a genuine, deliberate exception: inferring
  // the type of a cross-package global var that has no explicit type
  // (`var globalSrc = NewSource(seedFromClock())` in `math/rand`) needs to
  // re-run InferType on THAT declaring package's own init expression,
  // which was written -- and must be resolved -- relative to ITS package,
  // not whichever file happens to be generating right now. Without this,
  // a caller in a different package referencing `pkg.SomeGlobal` (no
  // explicit type) broke with "call to undefined function" for any
  // same-package function that global's initializer calls, because the
  // unscoped lookup silently searched the CALLER's file instead --
  // another instance of the "asking an unscoped/wrong-package question"
  // bug family. `unscoped_lookup_pkg_` is set only around that one
  // recursive InferType call (see its call site) and read here instead of
  // always defaulting to `&file_`.
  const File* UnscopedFile() const {
    return unscoped_lookup_pkg_.empty() ? &file_ : FindPackage(unscoped_lookup_pkg_);
  }

  const StructDecl* LookupStruct(const std::string& name, const std::string& pkg = "") const {
    if (!pkg.empty()) return LookupStructIn(FindPackage(pkg), name);
    return LookupStructIn(UnscopedFile(), name);
  }
  const FieldDecl* LookupField(const std::string& structName, const std::string& fieldName,
                               const std::string& pkg = "") const {
    const StructDecl* s = LookupStruct(structName, pkg);
    if (!s) return nullptr;
    for (auto& f : s->fields) {
      if (f.name == fieldName) return &f;
    }
    return nullptr;
  }
  bool HasMethodsOn(const std::string& name, const std::string& pkg = "") const {
    const File* f = pkg.empty() ? UnscopedFile() : FindPackage(pkg);
    if (!f) return false;
    for (auto& fn : f->funcs) {
      if (fn.has_receiver && fn.receiver_type == name) return true;
    }
    return false;
  }
  const FuncDecl* LookupMethod(const std::string& structName, const std::string& methodName,
                               const std::string& pkg = "") const {
    const File* f = pkg.empty() ? UnscopedFile() : FindPackage(pkg);
    if (!f) return nullptr;
    for (auto& fn : f->funcs) {
      if (fn.has_receiver && fn.receiver_type == structName && fn.name == methodName) return &fn;
    }
    return nullptr;
  }
  const FuncDecl* LookupFreeFunc(const std::string& name, const std::string& pkg = "") const {
    if (name == "init") return nullptr;
    const File* f = pkg.empty() ? UnscopedFile() : FindPackage(pkg);
    if (!f) return nullptr;
    for (auto& fn : f->funcs) {
      if (!fn.has_receiver && fn.name == name && fn.name != "init") return &fn;
    }
    return nullptr;
  }

  static bool IsInitFunc(const FuncDecl& fn) {
    return !fn.has_receiver && fn.name == "init";
  }

  static const TypeAlias* LookupAliasIn(const File* f, const std::string& name) {
    if (!f) return nullptr;
    for (auto& a : f->aliases) {
      if (a.name == name) return &a;
    }
    return nullptr;
  }

  const GlobalVarDecl* LookupGlobalDecl(const std::string& name, const std::string& pkg = "") const {
    const File* f = pkg.empty() ? UnscopedFile() : FindPackage(pkg);
    if (!f) return nullptr;
    for (auto& g : f->globals) {
      if (g.name == name) return &g;
    }
    return nullptr;
  }

  const TypeAlias* LookupAlias(const std::string& name, const std::string& pkg = "") const {
    if (!pkg.empty()) return LookupAliasIn(FindPackage(pkg), name);
    return LookupAliasIn(UnscopedFile(), name);
  }

  // `type Name T` is a C++ `using` (README: "not a distinct defined type"),
  // so a defined-over-slice/map/etc type (`type IntHeap []int`) needs its
  // alias followed before code that switches on TypeKind::Slice/Map/... --
  // e.g. indexing `h[i]` for `h IntHeap` -- can recognize it as one.
  // A composite literal's synthesized IIFE (`[&]{ T __s{}; __s.f = ...;
  // return __s; }()`) needs `[&]` to see a local it's initializing a field
  // from (e.g. `Ring{prev: p}`), but that capture-default is ill-formed at
  // namespace scope (a global var initializer, where current_func_ is
  // null) -- there's nothing local there to capture anyway, since Go's
  // global initializers can only reference other globals/consts.
  const char* LambdaCapture() const { return current_func_ ? "[&]" : "[]"; }

  // A struct name used unqualified inside its own member function is
  // normally fine (injected-class-name), but Go allows a field with the
  // very same name as its enclosing struct (net/mail's `Address struct {
  // Address string }`), and C++ hides the injected-class-name behind a
  // member of that name in that case. Namespace-qualifying sidesteps it.
  // MSVC (unlike g++) fails to compile `ptr->Address` when `Address` is
  // both the field name and the enclosing struct's name -- it parses the
  // right side of `->`/`.` as the injected-class-name (a type), which is
  // invalid there, regardless of `mail::`-style qualification (see
  // SelfTypeName just below, which handles the *type name* side of this
  // same net/mail.Address collision; this handles the *field* side). Since
  // g++ and Go itself both accept a field named after its own struct, the
  // C++-only identifier for that one field is mangled with a trailing `_`
  // wherever it's declared, written, or read -- purely a codegen-side
  // rename, invisible to the Go source and to LookupField's name matching.
  // reflect.Bool/Struct/etc. (see BuildReflectBuiltinFile) have no real
  // C++ symbol anywhere -- "reflect" is a synthesized builtin package, no
  // generated reflect.hpp ever exists for QualName("reflect", name) to
  // reference. Both InferType's Selector case and EmitSelector special-
  // case these 15 names directly instead, mapping straight to
  // wasigo::RKind (see runtime.hpp) so the two numberings can't drift.
  static bool IsReflectKindName(const std::string& n) {
    static const std::set<std::string> kNames = {
        "Invalid", "Bool",    "Int8",    "Int16",   "Int32",  "Int64",  "Uint8", "Uint16",
        "Uint32",  "Uint64",  "Float32", "Float64", "String", "Ptr",    "Struct", "Slice",
        "Complex64", "Complex128",
    };
    return kNames.count(n) != 0;
  }

  std::string FieldCppName(const std::string& struct_name, const std::string& go_field_name) const {
    std::string id = CppIdent(go_field_name);
    if (id == struct_name) return id + "_";
    return id;
  }

  std::string SelfTypeName(const StructDecl& sd) const {
    std::string n;
    if (opt_.library && !file_.package_name.empty() && file_.package_name != "main") {
      for (auto& f : sd.fields) {
        if (f.name == sd.name) {
          n = file_.package_name + "::" + sd.name;
          break;
        }
      }
    }
    if (n.empty()) n = sd.name;
    if (!sd.type_params.empty()) {
      n += "<";
      for (size_t i = 0; i < sd.type_params.size(); ++i) {
        if (i) n += ", ";
        n += sd.type_params[i];
      }
      n += ">";
    }
    return n;
  }

  const TypeNode* ResolveUnderlying(const TypeNode* t) const {
    int guard = 0;
    while (t && t->kind == TypeKind::Named && guard++ < 8) {
      const TypeAlias* a = LookupAlias(t->name, t->pkg);
      if (!a || !a->type) break;
      t = a->type.get();
    }
    return t;
  }

  static bool HasNamedResults(const FuncDecl* fn) {
    if (!fn) return false;
    for (auto& n : fn->result_names) {
      if (!n.empty() && n != "_") return true;
    }
    return false;
  }
  static const InterfaceDecl* LookupInterfaceIn(const File* f, const std::string& name) {
    if (!f) return nullptr;
    for (auto& i : f->interfaces) {
      if (i.name == name) return &i;
    }
    return nullptr;
  }

  const InterfaceDecl* LookupInterface(const std::string& name, const std::string& pkg = "") const {
    if (!pkg.empty()) return LookupInterfaceIn(FindPackage(pkg), name);
    return LookupInterfaceIn(UnscopedFile(), name);
  }

  void CollectIfaceMethods(const InterfaceDecl* id, const std::string& pkg,
                           std::vector<const MethodSig*>& out,
                           std::set<std::string>& seen_methods,
                           std::set<std::string>& seen_ifaces) const {
    if (!id) return;
    if (!seen_ifaces.insert(id->name).second) return;
    for (auto& m : id->methods) {
      if (seen_methods.insert(m.name).second) out.push_back(&m);
    }
    for (auto& emb : id->embedded) {
      // Interface embedding only ever parses a bare identifier (no
      // `pkg.Name` -- ParseInterfaceType's ExpectIdent), so an embedded
      // interface always lives in the SAME package as the interface
      // embedding it, never `file_`/global scope. Passing `pkg` through
      // (not defaulting to "") is the fix -- without it, an embedded
      // interface declared in an imported package (e.g. hash.Hash32
      // embedding hash.Hash) silently resolved against the wrong package
      // and came back "not found".
      CollectIfaceMethods(LookupInterface(emb, pkg), pkg, out, seen_methods, seen_ifaces);
    }
  }
  std::vector<const MethodSig*> FlattenIfaceMethods(const InterfaceDecl* id,
                                                     const std::string& pkg = "") const {
    std::vector<const MethodSig*> out;
    std::set<std::string> seen_methods;
    std::set<std::string> seen_ifaces;
    CollectIfaceMethods(id, pkg, out, seen_methods, seen_ifaces);
    return out;
  }

  const MethodSig* LookupIfaceMethod(const std::string& iface, const std::string& method,
                                     const std::string& pkg = "") const {
    const InterfaceDecl* id = LookupInterface(iface, pkg);
    if (!id) return nullptr;
    for (auto* m : FlattenIfaceMethods(id, pkg)) {
      if (m->name == method) return m;
    }
    return nullptr;
  }

  bool IsInterfaceType(const TypeNode* t) const {
    if (!t || t->kind != TypeKind::Named) return false;
    if (t->name == "any") return true;
    if (LookupInterface(t->name, t->pkg) != nullptr) return true;
    // A defined alias of `any` (`type Value any`, e.g. database/sql/
    // driver's own `Value` type) resolves, via ResolveUnderlying, to a
    // Named "any" node -- neither check above sees that without first
    // resolving through the alias chain, so a type assertion on a
    // `driver.Value`-typed expression (`args[0].(int64)`) wrongly failed
    // with "type assertion requires an interface value". Only recurse
    // through ResolveUnderlying when the direct checks miss, and only
    // when it actually changes the node (guards the same "any" case
    // reached a different way, not infinite recursion -- ResolveUnderlying
    // itself already bounds alias-chain depth).
    const TypeNode* underlying = ResolveUnderlying(t);
    if (underlying && underlying != t) return IsInterfaceType(underlying);
    return false;
  }

  void EmitNsOpen() {
    if (opt_.library && !file_.package_name.empty() && file_.package_name != "main") {
      std::string pkg = file_.package_name;
      std::string cur;
      for (char c : pkg) {
        if (c == '.') {
          out_ << "namespace " << CppIdent(cur) << " {\n";
          cur.clear();
        } else {
          cur.push_back(c);
        }
      }
      if (!cur.empty()) out_ << "namespace " << CppIdent(cur) << " {\n";
      out_ << "\n";
    }
  }
  void EmitNsClose() {
    if (opt_.library && !file_.package_name.empty() && file_.package_name != "main") {
      std::string pkg = file_.package_name;
      int n = 1;
      for (char c : pkg) {
        if (c == '.') n++;
      }
      for (int i = 0; i < n; ++i) out_ << "}  // namespace\n";
    }
  }

  bool ExprNeedsAwait(const Expr& e) {
    switch (e.kind) {
      case ExprKind::Recv:
        return true;
      case ExprKind::FuncLit:
        return e.func_lit && StmtsNeedAwait(e.func_lit->body);
      case ExprKind::Call: {
        if (e.callee->kind == ExprKind::Ident && IsAsyncFree(e.callee->strval)) return true;
        if (e.callee->kind == ExprKind::Selector) {
          auto* sel = e.callee.get();
          // pkg.Func(...) -- async_free_ is keyed by bare name (same as
          // Ident calls). Without this, `st, body, _ := http.Get(...)` in
          // main never marked main as a coroutine, and EmitCall emitted a
          // bare TaskT with no co_await (no r0/r1 on the Task).
          if (sel->x && sel->x->kind == ExprKind::Ident && IsImportedPackage(sel->x->strval) &&
              IsAsyncFree(sel->strval)) {
            return true;
          }
          // gocvm.Call is the one builtin async dispatch point (see
          // gocvm::CallAsync in runtime.hpp): it's not a parsed .go
          // FuncDecl at all, so IsAsyncFree/IsImportedPackage above
          // (which only know about real stdlib/user source) never see
          // it -- special-cased here the same way ExprKind::Recv is,
          // as an inherent await point rather than something inferred
          // from a callee's own body.
          if (sel->x && sel->x->kind == ExprKind::Ident && PkgOf(sel->x->strval) == "gocvm" &&
              sel->strval == "Call") {
            return true;
          }
          for (auto& k : async_methods_) {
            if (k.size() > sel->strval.size() + 1 &&
                k.compare(k.size() - sel->strval.size(), sel->strval.size(), sel->strval) == 0 &&
                k[k.size() - sel->strval.size() - 1] == '.') {
              return true;
            }
          }
        }
        if (e.callee && ExprNeedsAwait(*e.callee)) return true;
        for (auto& a : e.args) {
          if (a && ExprNeedsAwait(*a)) return true;
        }
        return false;
      }
      default:
        if (e.x && ExprNeedsAwait(*e.x)) return true;
        if (e.y && ExprNeedsAwait(*e.y)) return true;
        if (e.callee && ExprNeedsAwait(*e.callee)) return true;
        for (auto& a : e.args) {
          if (a && ExprNeedsAwait(*a)) return true;
        }
        for (auto& el : e.elems) {
          if (el && ExprNeedsAwait(*el)) return true;
        }
        if (e.low && ExprNeedsAwait(*e.low)) return true;
        if (e.high && ExprNeedsAwait(*e.high)) return true;
        if (e.max && ExprNeedsAwait(*e.max)) return true;
        return false;
    }
  }

  bool StmtsNeedAwait(const std::vector<std::unique_ptr<Stmt>>& stmts) {
    for (auto& sp : stmts) {
      if (sp && StmtNeedsAwait(*sp)) return true;
    }
    return false;
  }

  bool StmtNeedsAwait(const Stmt& s) {
    std::vector<const Stmt*> dummy;
    (void)dummy;
    switch (s.kind) {
      case StmtKind::Go:
        program_has_go_ = true;
        return false;
      case StmtKind::Send:
      case StmtKind::Select:
        return true;
      case StmtKind::ForRange:
        if (s.range_expr) {
          auto t = InferType(s.range_expr.get());
          if (t && t->kind == TypeKind::Chan) return true;
        }
        break;
      default:
        break;
    }
    if (s.cond && ExprNeedsAwait(*s.cond)) return true;
    if (s.range_expr && ExprNeedsAwait(*s.range_expr)) return true;
    if (s.init && StmtNeedsAwait(*s.init)) return true;
    if (s.post && StmtNeedsAwait(*s.post)) return true;
    for (auto& e : s.lhs) {
      if (e && ExprNeedsAwait(*e)) return true;
    }
    for (auto& e : s.rhs) {
      if (e && ExprNeedsAwait(*e)) return true;
    }
    if (StmtsNeedAwait(s.body) || StmtsNeedAwait(s.else_body)) return true;
    for (auto& c : s.cases) {
      for (auto& v : c.values) {
        if (v && ExprNeedsAwait(*v)) return true;
      }
      if (StmtsNeedAwait(c.body)) return true;
    }
    for (auto& c : s.sel_cases) {
      if (!c.is_default) return true;
      if (StmtsNeedAwait(c.body)) return true;
    }
    return false;
  }

  bool TypeHasChan(const TypeNode* t) const {
    if (!t) return false;
    if (t->kind == TypeKind::Chan) return true;
    return TypeHasChan(t->elem.get()) || TypeHasChan(t->key.get());
  }

  bool StmtsHaveRecover(const std::vector<std::unique_ptr<Stmt>>& stmts) const {
    for (auto& sp : stmts) {
      if (!sp) continue;
      for (auto& e : sp->lhs) {
        if (e && e->kind == ExprKind::Call && e->callee && e->callee->kind == ExprKind::Ident &&
            (e->callee->strval == "panic" || e->callee->strval == "recover")) {
          return true;
        }
      }
      for (auto& e : sp->rhs) {
        if (e && e->kind == ExprKind::Call && e->callee && e->callee->kind == ExprKind::Ident &&
            (e->callee->strval == "panic" || e->callee->strval == "recover")) {
          return true;
        }
      }
      if (StmtsHaveRecover(sp->body) || StmtsHaveRecover(sp->else_body)) return true;
      for (auto& c : sp->cases) {
        if (StmtsHaveRecover(c.body)) return true;
      }
      for (auto& c : sp->sel_cases) {
        if (StmtsHaveRecover(c.body)) return true;
      }
    }
    return false;
  }

  // A package that never itself writes `go`/`chan`/`<-` but *imports* one
  // that does (e.g. context, which has a `chan bool` struct field) still
  // needs WASIGO_NEED_CORO -- the imported package's own struct/function
  // definitions get emitted into the same translation unit either way, and
  // reference wasigo::Chan<T>/make_chan/close regardless of whether this
  // file's own code happens to touch a channel. Scanning only file_ missed
  // that entirely.
  bool FileNeedsChan(const File& f) const {
    for (auto& g : f.globals) {
      if (TypeHasChan(g.type.get())) return true;
    }
    for (auto& sd : f.structs) {
      for (auto& fd : sd.fields) {
        if (TypeHasChan(fd.type.get())) return true;
      }
    }
    for (auto& fn : f.funcs) {
      for (auto& p : fn.params) {
        if (TypeHasChan(p.type.get())) return true;
      }
      for (auto& r : fn.results) {
        if (TypeHasChan(r.get())) return true;
      }
    }
    return false;
  }

  bool NeedCoro() const {
    if (program_has_go_ || !async_free_.empty() || !async_methods_.empty()) return true;
    if (FileNeedsChan(file_)) return true;
    for (const File* f : opt_.imported_files) {
      if (f && FileNeedsChan(*f)) return true;
    }
    return false;
  }

  bool NeedSjlj() const {
    for (auto& fn : file_.funcs) {
      if (StmtsHaveDefer(fn.body) || StmtsHaveRecover(fn.body)) return true;
    }
    return false;
  }

  // This pre-pass only exists to detect channel/goroutine usage so the
  // right functions get marked async (see AnalyzeAsync) -- it must never
  // itself be the reason a program fails to compile, so every inference it
  // does is best-effort: a name it can't type just stays untyped (nullptr)
  // for the rest of the pass, same as a GenError caught below.
  void BindDeclForAnalyze(const Stmt& s) {
    try {
      if (s.kind == StmtKind::ShortVarDecl) {
        if (s.names.size() == 1 && s.rhs.size() == 1) {
          Declare(s.names[0], InferType(s.rhs[0].get()));
        } else if (s.names.size() == s.rhs.size()) {
          for (size_t i = 0; i < s.names.size(); ++i) {
            Declare(s.names[i], InferType(s.rhs[i].get()));
          }
        } else if (s.names.size() > 1 && s.rhs.size() == 1) {
          // `a, b := f()`: the real EmitMultiUnpackDecl resolves this via
          // ResolveCalledFunc/ResolveCalledIface (a comma-ok map index or
          // type assert's 2nd result is always bool, handled below too) --
          // reuse that instead of re-deriving it here.
          const Expr& rhs = *s.rhs[0];
          if (rhs.kind == ExprKind::Call) {
            const std::vector<std::unique_ptr<TypeNode>>* results = nullptr;
            if (const FuncDecl* fn = ResolveCalledFunc(rhs)) {
              results = &fn->results;
            } else if (const MethodSig* ms = ResolveCalledIface(rhs)) {
              results = &ms->results;
            }
            if (results && results->size() == s.names.size()) {
              for (size_t i = 0; i < s.names.size(); ++i) Declare(s.names[i], (*results)[i].get());
            }
          } else if (s.names.size() == 2 &&
                     (rhs.kind == ExprKind::Index || rhs.kind == ExprKind::TypeAssert)) {
            if (rhs.kind == ExprKind::Index) {
              auto baseType = InferType(rhs.x.get());
              Declare(s.names[0], baseType ? baseType->elem.get() : nullptr);
            } else {
              Declare(s.names[0], rhs.type.get());
            }
            Declare(s.names[1], SynthNamed("bool"));
          }
        }
      } else if (s.kind == StmtKind::Var) {
        for (size_t i = 0; i < s.names.size(); ++i) {
          const TypeNode* t = s.var_type.get();
          if (!t && i < s.rhs.size()) t = InferType(s.rhs[i].get());
          Declare(s.names[i], t);
        }
      }
    } catch (const GenError&) {
      // Best-effort, as above -- swallow and leave whatever wasn't
      // Declare()d untyped rather than letting a pre-pass quirk (this one
      // or one not yet found) abort the whole compile.
    }
  }

  bool AnalyzeStmts(const std::vector<std::unique_ptr<Stmt>>& stmts) {
    bool need = false;
    PushScope();
    for (auto& sp : stmts) {
      if (!sp) continue;
      BindDeclForAnalyze(*sp);
      if (StmtNeedsAwait(*sp)) need = true;
      if (AnalyzeStmts(sp->body)) need = true;
      if (AnalyzeStmts(sp->else_body)) need = true;
      for (auto& c : sp->cases) {
        if (AnalyzeStmts(c.body)) need = true;
      }
      for (auto& c : sp->sel_cases) {
        if (AnalyzeStmts(c.body)) need = true;
      }
    }
    PopScope();
    return need;
  }

  void AnalyzeAsync() {
    // StmtsNeedAwait currently calls ExprNeedsAwait which consults async_free_
    // so we iterate to a fixpoint. Locals are bound in order so `for range ch`
    // sees that `ch` is a channel.
    //
    // Scans file_ AND every imported file, not just file_ -- an imported
    // package's OWN method can itself block on a channel internally (net's
    // Conn.Read/Write do, via net.Pipe) with none of that visible in the
    // calling file's own source. Scanning only file_ (as this used to)
    // meant such a call-site never got `co_await`ed, and the resulting
    // TaskT<Result> was accessed as a plain struct at the call site (a
    // silent-until-you-look-at-the-error compile failure: "no member
    // named 'r0'/'r1'") -- same blind spot FileNeedsChan/NeedCoro already
    // had to fix for the analogous "channel-typed field, not call" case.
    bool changed = true;
    while (changed) {
      changed = false;
      std::vector<const File*> files_to_scan = {&file_};
      for (const File* f : opt_.imported_files) {
        if (f) files_to_scan.push_back(f);
      }
      for (const File* fptr : files_to_scan) {
        // A receiver/param type is written unqualified from inside its
        // own defining file (`b *Block`, not `*pem.Block`, inside
        // package pem itself) -- same rule QualifyResultType already
        // documents for return types and struct fields. When `fptr` is
        // an IMPORTED file (not `file_`), that bare name needs `fptr`'s
        // own package attached before this pass's later InferType calls
        // (e.g. resolving `b.Headers`'s field type while checking a
        // `range` statement) can find it -- otherwise a field/method
        // lookup on it incorrectly searches `file_`'s own package
        // instead, "unknown field ... on type ...". Found via
        // encoding/pem's `for k, v := range b.Headers` inside a
        // same-package method: this pass's own receiver Declare left
        // `b`'s type unqualified even though `fptr` (pem.go) isn't
        // `file_` (whichever file imports pem), which used to be masked
        // by LookupStruct's old (buggy, now-removed) cross-package
        // fallback and only surfaced once that was fixed.
        const std::string scan_pkg = fptr == &file_ ? "" : fptr->package_name;
        for (auto& fn : fptr->funcs) {
          PushScope();
          if (fn.has_receiver) {
            const TypeNode* recv_base = SynthNamed(fn.receiver_type, scan_pkg);
            if (fn.receiver_is_pointer) {
              Declare(fn.receiver_name, SynthPointer(recv_base));
            } else {
              Declare(fn.receiver_name, recv_base);
            }
          }
          for (auto& p : fn.params) {
            const TypeNode* pt = ParamGoType(p);
            if (!scan_pkg.empty()) pt = QualifyResultType(pt, scan_pkg);
            Declare(p.name, pt);
          }
          bool need = AnalyzeStmts(fn.body);
          PopScope();
          if (!need) continue;
          if (fn.has_receiver) {
            auto k = MethodKey(fn.receiver_type, fn.name);
            if (async_methods_.insert(k).second) changed = true;
          } else {
            if (async_free_.insert(fn.name).second) changed = true;
          }
        }
      }
    }
  }

  std::string ResultStructName(const FuncDecl& fn) const {
    if (fn.has_receiver) return fn.receiver_type + "_" + fn.name + "Result";
    return fn.name + "Result";
  }

  // A result type named without a package qualifier (as parsed, from inside
  // the defining file) needs `pkg` attached when a *different* package's
  // `:=` is about to spell it out as a concrete C++ type name (`pkg::Name`)
  // -- otherwise the emitted type is missing its namespace. Recurses through
  // Pointer so `func New() *T` (any container/list-shaped constructor, not
  // just by-value ones like the geom/shape examples) qualifies correctly,
  // and through Slice/Map so e.g. `func ParseAddressList(s string) ([]*T,
  // error)` qualifies the pointer *inside* the slice too (net/mail hit
  // this: the unqualified `Address` in `wasigo::Slice<Address*>` doesn't
  // resolve outside package mail).
  const TypeNode* QualifyResultType(const TypeNode* r, const std::string& pkg) {
    if (!r || pkg.empty() || pkg == file_.package_name) return r;
    if (r->kind == TypeKind::Named && r->pkg.empty() &&
        (LookupStruct(r->name, pkg) || LookupInterface(r->name, pkg) || LookupAlias(r->name, pkg))) {
      return SynthNamed(r->name, pkg);
    }
    if (r->kind == TypeKind::Pointer) {
      const TypeNode* qelem = QualifyResultType(r->elem.get(), pkg);
      if (qelem != r->elem.get()) return SynthPointer(qelem);
    }
    if (r->kind == TypeKind::Slice) {
      const TypeNode* qelem = QualifyResultType(r->elem.get(), pkg);
      if (qelem != r->elem.get()) return SynthSlice(qelem);
    }
    if (r->kind == TypeKind::Map) {
      const TypeNode* qkey = QualifyResultType(r->key.get(), pkg);
      const TypeNode* qelem = QualifyResultType(r->elem.get(), pkg);
      if (qkey != r->key.get() || qelem != r->elem.get()) return SynthMap(qkey, qelem);
    }
    return r;
  }

  const TypeNode* ResultTypeOf(const FuncDecl& fn, const std::string& pkg = "") {
    if (fn.results.empty()) return nullptr;
    if (fn.results.size() == 1) {
      return QualifyResultType(fn.results[0].get(), pkg);
    }
    return SynthNamed(ResultStructName(fn));
  }

  // Generic-call return-type substitution: a generic free function's
  // declared result type may itself be (or contain) a type parameter --
  // e.g. `func Keys[K comparable, V any](m map[K]V) []K`. InferCallType
  // needs a concrete Go type for the call (to spell a `:=` variable's C++
  // type), so unify each declared param type against the call's actual
  // argument types to recover a K/V -> concrete-type map, then substitute
  // it into the declared result type. The generated C++ call itself needs
  // no help (ordinary template argument deduction handles that); this is
  // purely for this compiler's own static Go-level type inference.
  void UnifyType(const TypeNode* declared, const TypeNode* actual,
                  const std::vector<std::string>& tparams,
                  std::map<std::string, const TypeNode*>& subst) {
    if (!declared || !actual) return;
    if (declared->kind == TypeKind::Named && declared->pkg.empty()) {
      for (auto& tp : tparams) {
        if (tp == declared->name) {
          if (!subst.count(tp)) subst[tp] = actual;
          return;
        }
      }
    }
    if (declared->kind != actual->kind) return;
    switch (declared->kind) {
      case TypeKind::Pointer:
      case TypeKind::Slice:
      case TypeKind::Array:
        UnifyType(declared->elem.get(), actual->elem.get(), tparams, subst);
        return;
      case TypeKind::Map:
        UnifyType(declared->key.get(), actual->key.get(), tparams, subst);
        UnifyType(declared->elem.get(), actual->elem.get(), tparams, subst);
        return;
      case TypeKind::Chan:
        UnifyType(declared->elem.get(), actual->elem.get(), tparams, subst);
        return;
      default:
        return;
    }
  }

  const TypeNode* SubstituteTypeParams(const TypeNode* t,
                                        const std::map<std::string, const TypeNode*>& subst) {
    if (!t) return nullptr;
    if (t->kind == TypeKind::Named && t->pkg.empty()) {
      auto it = subst.find(t->name);
      if (it != subst.end()) return it->second;
      return t;
    }
    auto c = std::make_unique<TypeNode>();
    c->kind = t->kind;
    c->name = t->name;
    c->pkg = t->pkg;
    if (t->key) c->key = CloneType(SubstituteTypeParams(t->key.get(), subst));
    if (t->elem) c->elem = CloneType(SubstituteTypeParams(t->elem.get(), subst));
    c->chan_send = t->chan_send;
    c->chan_recv = t->chan_recv;
    c->array_len = t->array_len;
    c->array_len_expr = CloneArrayLen(t->array_len_expr.get());
    c->variadic = t->variadic;
    synth_types_.push_back(std::move(c));
    return synth_types_.back().get();
  }

  const TypeNode* ResultTypeOfCall(const FuncDecl& fn, const std::string& pkg,
                                    const std::vector<std::unique_ptr<Expr>>& args) {
    const TypeNode* rt = ResultTypeOf(fn, pkg);
    if (!rt || fn.type_params.empty()) return rt;
    std::map<std::string, const TypeNode*> subst;
    for (size_t i = 0; i < fn.params.size() && i < args.size(); ++i) {
      if (!args[i]) continue;
      UnifyType(fn.params[i].type.get(), InferType(args[i].get()), fn.type_params, subst);
    }
    if (subst.empty()) return rt;
    return SubstituteTypeParams(rt, subst);
  }

  std::string ReturnCppType(const FuncDecl& fn) {
    if (fn.results.empty()) return "void";
    if (fn.results.size() == 1) return CppType(fn.results[0].get());
    return ResultStructName(fn);
  }

  std::string CoroutineCppType(const FuncDecl& fn) {
    if (fn.results.empty()) return "wasigo::Task";
    return "wasigo::TaskT<" + ReturnCppType(fn) + ">";
  }

  std::string FuncCppType(const FuncDecl& fn, bool async) {
    return async ? CoroutineCppType(fn) : ReturnCppType(fn);
  }

  std::string ParamCppType(const Param& p) {
    if (p.variadic) return "wasigo::Slice<" + CppType(p.type.get()) + ">";
    return CppType(p.type.get());
  }

  const TypeNode* ParamGoType(const Param& p) {
    if (p.variadic) return SynthSlice(p.type.get());
    return p.type.get();
  }

  bool IsMainFunc() const {
    return !in_func_lit_ && current_func_ && !current_func_->has_receiver && current_func_->name == "main";
  }

  // The result count/type EmitReturn should use for a bare/short return --
  // the func LITERAL's own, when emitting one's body (see in_func_lit_'s
  // comment), otherwise the enclosing named function's.
  size_t CurrentResultCount() const {
    if (in_func_lit_) return func_lit_results_ ? func_lit_results_->size() : 0;
    return current_func_ ? current_func_->results.size() : 0;
  }
  const TypeNode* CurrentResultType(size_t i) const {
    if (in_func_lit_) {
      return (func_lit_results_ && i < func_lit_results_->size()) ? (*func_lit_results_)[i].get() : nullptr;
    }
    return (current_func_ && i < current_func_->results.size()) ? current_func_->results[i].get() : nullptr;
  }

  // ---- Go type -> C++ type -------------------------------------------------

  std::string NamedCppType(const std::string& n) const {
    static const std::unordered_map<std::string, std::string> kBuiltins = {
        {"int", "int64_t"},     {"int8", "int8_t"},     {"int16", "int16_t"},
        {"int32", "int32_t"},   {"int64", "int64_t"},   {"uint", "uint64_t"},
        {"uint8", "uint8_t"},   {"uint16", "uint16_t"}, {"uint32", "uint32_t"},
        {"uint64", "uint64_t"}, {"uintptr", "uint64_t"}, {"byte", "uint8_t"},
        {"rune", "int32_t"},
        {"float32", "float"},   {"float64", "double"},  {"bool", "bool"},
        {"complex64", "wasigo::Complex64"}, {"complex128", "wasigo::Complex128"},
        {"string", "std::string"}, {"error", "wasigo::Error"}, {"any", "wasigo::Any"},
        {"__recovered", "wasigo::Recovered"},
    };
    auto it = kBuiltins.find(n);
    if (it != kBuiltins.end()) return it->second;
    if (current_func_) {
      for (auto& tp : current_func_->type_params) {
        if (tp == n) return n;
      }
    }
    for (auto& tp : current_type_params_) {
      if (tp == n) return n;
    }
    if (LookupStruct(n)) return n;
    if (LookupInterface(n)) return n;
    if (LookupAlias(n)) return n;
    if (result_struct_names_.count(n)) return n;
    Error("unknown type '" + n + "'");
  }

  std::string NamedCppType(const TypeNode* t) const {
    if (!t) return "void";
    // os.File is a builtin type (see wasigo::File in runtime.hpp), not a
    // struct parsed from .go source -- "os" is one of the three special
    // packages (fmt/errors/os) that don't load a real stdlib/os/*.go.
    if (t->kind == TypeKind::Named && t->pkg == "os" && t->name == "File") return "wasigo::File";
    if (t->kind == TypeKind::Named && t->pkg == "os" && t->name == "FileInfo") return "wasigo::FileInfo";
    if (t->kind == TypeKind::Named && t->pkg == "os" && t->name == "DirEntry") return "wasigo::DirEntry";
    // reflect.Value and reflect.Type are both wasigo::Any under the hood
    // (see BuildReflectBuiltinFile) -- same reasoning as os.File above.
    if (t->kind == TypeKind::Named && t->pkg == "reflect" &&
        (t->name == "Value" || t->name == "Type")) {
      return "wasigo::Any";
    }
    std::string base;
    if (t->pkg.empty() || t->pkg == file_.package_name) {
      base = NamedCppType(t->name);
    } else if (LookupStruct(t->name, t->pkg) || LookupInterface(t->name, t->pkg) ||
               LookupAlias(t->name, t->pkg)) {
      base = QualName(t->pkg, t->name);
    } else {
      Error("unknown type '" + t->pkg + "." + t->name + "'");
    }
    if (!t->type_args.empty()) {
      base += "<";
      for (size_t i = 0; i < t->type_args.size(); ++i) {
        if (i) base += ", ";
        base += CppType(t->type_args[i].get());
      }
      base += ">";
    }
    return base;
  }

  std::string CppType(const TypeNode* t) const {
    if (!t) return "void";
    switch (t->kind) {
      case TypeKind::Pointer:
        return CppType(t->elem.get()) + "*";
      case TypeKind::Slice:
        return "wasigo::Slice<" + CppType(t->elem.get()) + ">";
      case TypeKind::Map:
        return "wasigo::Map<" + CppType(t->key.get()) + ", " + CppType(t->elem.get()) + ">";
      case TypeKind::Chan:
        return "wasigo::Chan<" + CppType(t->elem.get()) + ">";
      case TypeKind::Array:
        return "std::array<" + CppType(t->elem.get()) + ", " + std::to_string(ResolvedArrayLen(t)) + ">";
      case TypeKind::Func: {
        std::string r = "void";
        if (t->func_results.size() == 1) r = CppType(t->func_results[0].get());
        else if (t->func_results.size() > 1) r = "auto";
        std::string ps;
        for (size_t i = 0; i < t->func_params.size(); ++i) {
          if (i) ps += ", ";
          ps += CppType(t->func_params[i].type.get());
        }
        return "wasigo::Func<" + r + "(" + ps + ")>";
      }
      case TypeKind::Named:
        return NamedCppType(t);
    }
    Error("unreachable type kind");
  }

  // ---- type inference -------------------------------------------------------

  const TypeNode* InferType(const Expr* e) {
    if (!e) return nullptr;
    NoteLoc(*e);
    switch (e->kind) {
      case ExprKind::IntLit: return SynthNamed("int");
      case ExprKind::FloatLit: return SynthNamed("float64");
      case ExprKind::ImagLit: return SynthNamed("complex128");
      case ExprKind::StringLit: return SynthNamed("string");
      case ExprKind::BoolLit: return SynthNamed("bool");
      case ExprKind::Nil: return nullptr;
      case ExprKind::Ident:
        if (e->strval == "iota") return SynthNamed("int");
        if (const TypeNode* t = Lookup(e->strval)) return t;
        if (const FuncDecl* f = LookupFreeFunc(e->strval)) {
          return SynthFuncType(f->params, f->results);
        }
        return nullptr;
      case ExprKind::ParenExpr: return InferType(e->x.get());
      case ExprKind::Unary: {
        if (e->strval == "!") return SynthNamed("bool");
        if (e->strval == "&") {
          auto inner = InferType(e->x.get());
          return inner ? SynthPointer(inner) : nullptr;
        }
        if (e->strval == "*") {
          auto inner = InferType(e->x.get());
          if (inner && inner->kind == TypeKind::Pointer) return inner->elem.get();
          Error("cannot dereference a non-pointer value");
        }
        return InferType(e->x.get());
      }
      case ExprKind::Binary: {
        if (IsComparisonOrLogicalOp(e->strval)) return SynthNamed("bool");
        auto lt = InferType(e->x.get());
        auto rt = InferType(e->y.get());
        if (IsComplexType(lt) || IsComplexType(rt)) {
          if (IsComplexType(lt) && lt->name == "complex64" && IsComplexType(rt) &&
              rt->name == "complex64") {
            return SynthNamed("complex64");
          }
          return SynthNamed("complex128");
        }
        return lt;
      }
      case ExprKind::Selector: {
        if (e->x->kind == ExprKind::Ident && PkgOf(e->x->strval) == "fmt") {
          Error("'fmt' is a package, not a value");
        }
        if (e->x->kind == ExprKind::Ident && PkgOf(e->x->strval) == "reflect" &&
            IsReflectKindName(e->strval)) {
          return SynthNamed("int");
        }
        if (e->x->kind == ExprKind::Ident && PkgOf(e->x->strval) == "os" && e->strval == "Args") {
          return SynthSlice(SynthNamed("string"));
        }
        if (e->x->kind == ExprKind::Ident && PkgOf(e->x->strval) == "os" &&
            (e->strval == "Stdout" || e->strval == "Stdin" || e->strval == "Stderr")) {
          return SynthNamed("File", "os");
        }
        if (e->x->kind == ExprKind::Ident && IsImportedPackage(e->x->strval)) {
          const std::string pkg = PkgOf(e->x->strval);
          if (const GlobalVarDecl* g = LookupGlobalDecl(e->strval, pkg)) {
            if (g->type) {
              auto t = CloneType(g->type.get());
              if (t->kind == TypeKind::Named && t->pkg.empty()) t->pkg = pkg;
              synth_types_.push_back(std::move(t));
              return synth_types_.back().get();
            }
            // No explicit type (`var LittleEndian = byteOrder{...}`): the
            // type comes from the initializer expression, which was
            // parsed from INSIDE the defining package's own file, so it's
            // unqualified there (correctly, relative to that file) -- but
            // that's the wrong package once returned to a caller in a
            // DIFFERENT package. Same qualification the `g->type` branch
            // above already does, just for the inferred-not-declared case.
            std::string saved_unscoped_pkg = unscoped_lookup_pkg_;
            unscoped_lookup_pkg_ = pkg;
            const TypeNode* inferred = InferType(g->init.get());
            unscoped_lookup_pkg_ = saved_unscoped_pkg;
            // Qualify through Pointer/Slice/Map too, not just a bare
            // Named result (e.g. `var GlobalSrc = NewSource(...)` infers
            // as `*Source`, a Pointer wrapping the unqualified Named --
            // same recursion QualifyResultType already does for an
            // ordinary cross-package function call's result type).
            return QualifyResultType(inferred, pkg);
          }
          Error("unknown name '" + pkg + "." + e->strval + "'");
        }
        auto baseType = InferType(e->x.get());
        if (!baseType) Error("cannot resolve the type of a selector base");
        const TypeNode* st = baseType->kind == TypeKind::Pointer ? baseType->elem.get() : baseType;
        if (st->kind != TypeKind::Named) Error("selector base is not a struct");
        const FieldDecl* fd = LookupField(st->name, e->strval, st->pkg);
        // A field's type is written unqualified from inside its own
        // struct's defining file (`List []*Node`, not `[]*ast.Node`,
        // inside package ast itself) -- accessed from a different
        // package (`f.List[0]` in main), that bare "Node" needs the
        // same qualification QualifyResultType already gives a
        // cross-package call's return type, or CppType emits an
        // unresolvable bare name. `st->pkg` is the struct's own
        // package, the right qualification target regardless of which
        // package is doing the accessing.
        if (fd) return QualifyResultType(fd->type.get(), st->pkg);
        if (const FuncDecl* m = LookupMethod(st->name, e->strval, st->pkg)) {
          if (m->results.size() > 1) {
            Error("method values with multiple results are not supported");
          }
          return SynthFuncType(m->params, m->results);
        }
        if (const MethodSig* ms = LookupIfaceMethod(st->name, e->strval, st->pkg)) {
          if (ms->results.size() > 1) {
            Error("method values with multiple results are not supported");
          }
          return SynthFuncType(ms->params, ms->results);
        }
        Error("unknown field '" + e->strval + "' on type '" + st->name + "'");
      }
      case ExprKind::Index: {
        auto baseType = ResolveUnderlying(InferType(e->x.get()));
        if (!baseType) Error("cannot resolve the type of an indexed expression");
        if (baseType->kind == TypeKind::Slice) return baseType->elem.get();
        if (baseType->kind == TypeKind::Array) return baseType->elem.get();
        if (baseType->kind == TypeKind::Map) return baseType->elem.get();
        if (baseType->kind == TypeKind::Named && baseType->name == "string") return SynthNamed("byte");
        // A pointer to an array indexes as if through the pointee (`p[i]`
        // is shorthand for `(*p)[i]` when p is `*[N]T`, per the Go spec) --
        // found while porting image/jpeg's `*block` ([64]int32) params.
        if (baseType->kind == TypeKind::Pointer) {
          const TypeNode* elemUnderlying = ResolveUnderlying(baseType->elem.get());
          if (elemUnderlying && elemUnderlying->kind == TypeKind::Array) return elemUnderlying->elem.get();
        }
        Error("cannot index this type");
      }
      case ExprKind::Call:
        return InferCallType(*e);
      case ExprKind::CompositeLit:
        return e->type.get();
      case ExprKind::Recv: {
        auto ct = InferType(e->x.get());
        if (ct && ct->kind == TypeKind::Chan) return ct->elem.get();
        Error("receive from a non-channel value");
      }
      case ExprKind::SliceExpr: {
        auto b = InferType(e->x.get());
        if (!b) Error("cannot resolve the type of a slice expression");
        if (b->kind == TypeKind::Slice) return b;
        if (b->kind == TypeKind::Array) return SynthSlice(b->elem.get());
        if (b->kind == TypeKind::Named && b->name == "string") return SynthNamed("string");
        Error("cannot slice this type");
      }
      case ExprKind::TypeAssert:
        return e->type.get();
      case ExprKind::FuncLit: {
        auto t = std::make_unique<TypeNode>();
        t->kind = TypeKind::Func;
        if (e->func_lit) {
          for (auto& p : e->func_lit->params) {
            Param np;
            np.name = p.name;
            np.type = CloneType(p.type.get());
            np.variadic = p.variadic;
            t->func_params.push_back(std::move(np));
          }
          for (auto& r : e->func_lit->results) t->func_results.push_back(CloneType(r.get()));
        }
        synth_types_.push_back(std::move(t));
        return synth_types_.back().get();
      }
    }
    Error("unreachable expression kind");
  }

  const TypeNode* TypeOfTypeExpr(const Expr& e) {
    if (e.kind == ExprKind::CompositeLit && e.type) return e.type.get();
    if (e.kind == ExprKind::Ident) return SynthNamed(e.strval);
    if (e.kind == ExprKind::Nil) return SynthNamed("nil");
    if (e.kind == ExprKind::Unary && e.strval == "*" && e.x) {
      auto inner = TypeOfTypeExpr(*e.x);
      return inner ? SynthPointer(inner) : nullptr;
    }
    if (e.kind == ExprKind::Selector && e.x && e.x->kind == ExprKind::Ident) {
      return SynthNamed(e.strval, PkgOf(e.x->strval));
    }
    Error("expected a type");
  }

  const TypeNode* InferCallType(const Expr& e) {
    if (e.callee->kind == ExprKind::CompositeLit && e.callee->type) {
      return e.callee->type.get();
    }
    if (e.callee->kind == ExprKind::Selector) {
      auto* sel = e.callee.get();
      if (sel->x->kind == ExprKind::Ident && PkgOf(sel->x->strval) == "fmt") {
        if (sel->strval == "Sprintf" || sel->strval == "Sprint" || sel->strval == "Sprintln") {
          return SynthNamed("string");
        }
        if (sel->strval == "Errorf") return SynthNamed("error");
        return nullptr;
      }
      if (sel->x->kind == ExprKind::Ident && PkgOf(sel->x->strval) == "errors") {
        if (sel->strval == "New" || sel->strval == "Unwrap" || sel->strval == "Join") {
          return SynthNamed("error");
        }
        if (sel->strval == "Is") return SynthNamed("bool");
      }
      if (sel->x->kind == ExprKind::Ident && PkgOf(sel->x->strval) == "reflect") {
        if (sel->strval == "TypeOf") return SynthNamed("Type", "reflect");
        if (sel->strval == "ValueOf") return SynthNamed("Value", "reflect");
        Error("unsupported reflect function '" + sel->strval + "' (TypeOf, ValueOf)");
      }
      if (sel->x->kind == ExprKind::Ident && PkgOf(sel->x->strval) == "os") {
        if (sel->strval == "Args") return SynthSlice(SynthNamed("string"));
        if (sel->strval == "Getenv") return SynthNamed("string");
        if (sel->strval == "WriteFile") return SynthNamed("error");
        // Open/Create/ReadFile are 2-result -- resolved through
        // ResolveCalledFunc's synthetic FuncDecl (BuildOsBuiltinFile) at
        // the `f, err := os.Open(...)` unpack site, not here.
        return nullptr;
      }
      if (sel->x->kind == ExprKind::Ident && PkgOf(sel->x->strval) == "time" && sel->strval == "Now") {
        // No Go source can read the wall clock, so time.Now() is
        // special-cased the same way os.Args is, even though "time" is an
        // ordinary loaded package otherwise -- see stdlib/time/time.go's
        // header comment and EmitCall's matching case.
        return SynthNamed("Time", "time");
      }
      if (sel->x->kind == ExprKind::Ident && IsImportedPackage(sel->x->strval)) {
        const std::string pkg = PkgOf(sel->x->strval);
        const FuncDecl* f = LookupFreeFunc(sel->strval, pkg);
        if (f) return ResultTypeOfCall(*f, pkg, e.args);
        // Not a function -- `pkg.Type(x)` (e.g. `time.Duration(n)`) is a
        // cross-package named-type conversion, not a call. EmitCall's
        // matching branch does the actual conversion.
        if (LookupAlias(sel->strval, pkg) || LookupStruct(sel->strval, pkg)) {
          return SynthNamed(sel->strval, pkg);
        }
        Error("call to undefined function '" + sel->x->strval + "." + sel->strval + "'");
      }
      auto baseType = InferType(sel->x.get());
      const TypeNode* st = baseType && baseType->kind == TypeKind::Pointer ? baseType->elem.get() : baseType;
      if (!st || st->kind != TypeKind::Named) Error("cannot resolve method call receiver type");
      // `error` is a builtin type (wasigo::Error), not a StructDecl, so it
      // has no entry for LookupMethod/LookupIfaceMethod to find -- but
      // Go's error interface *is* exactly one method, and `err.Error()` is
      // extremely common (arguably more so than printing err via fmt).
      if (st->name == "error" && st->pkg.empty() && sel->strval == "Error") {
        return SynthNamed("string");
      }
      if (const FuncDecl* m = LookupMethod(st->name, sel->strval, st->pkg)) {
        return ResultTypeOfCall(*m, st->pkg, e.args);
      }
      if (const MethodSig* ms = LookupIfaceMethod(st->name, sel->strval, st->pkg)) {
        if (ms->results.empty()) return nullptr;
        // Unlike the struct-method branch just above (which already
        // routes through ResultTypeOfCall -> QualifyResultType), an
        // interface method's result type -- as declared inside the
        // interface's OWN defining package -- is unqualified there, and
        // needs the same cross-package qualification when called from
        // outside it (e.g. `color.Model.Convert(...) Color`, called from
        // `main`, inferring the `:=` variable's type as bare "Color"
        // instead of "color.Color" -- found calling image/color's
        // `Model.Convert`).
        if (ms->results.size() == 1) return QualifyResultType(ms->results[0].get(), st->pkg);
        return SynthNamed(ms->name + "_result");
      }
      // A struct FIELD of function type, called via the same `x.f(...)`
      // selector-call syntax as a method (C++ member-function-call syntax
      // covers both a real method and a callable `std::function` field
      // identically, so this needs no different codegen shape from the
      // method case above -- just a different lookup). Found building
      // image/color's Model (`modelFunc{f: someFunc}`, `m.f(c)` inside its
      // own Convert method) -- real Go's own image/color package uses this
      // exact same struct-wraps-a-func trick, since a bare func can't have
      // methods.
      if (const FieldDecl* fd = LookupField(st->name, sel->strval, st->pkg)) {
        const TypeNode* ft = fd->type.get();
        if (ft && ft->kind == TypeKind::Func) {
          if (ft->func_results.empty()) return nullptr;
          if (ft->func_results.size() == 1) return ft->func_results[0].get();
          return SynthNamed(sel->strval + "_result");
        }
      }
      Error("unknown method '" + sel->strval + "' on type '" + st->name + "'");
    }
    if (e.callee->kind == ExprKind::Ident) {
      const std::string& name = e.callee->strval;
      if (name == "real" || name == "imag") {
        if (e.args.empty()) return SynthNamed("float64");
        auto t = InferType(e.args[0].get());
        if (IsComplexType(t) && t->name == "complex64") return SynthNamed("float32");
        return SynthNamed("float64");
      }
      if (name == "complex") {
        if (e.args.size() >= 2 && IsFloat32Type(InferType(e.args[0].get())) &&
            IsFloat32Type(InferType(e.args[1].get()))) {
          return SynthNamed("complex64");
        }
        return SynthNamed("complex128");
      }
      if (name == "len" || name == "cap" || name == "copy") return SynthNamed("int");
      if (name == "append") return e.args.empty() ? nullptr : InferType(e.args[0].get());
      if (name == "panic") return nullptr;
      if (name == "recover") return SynthNamed("__recovered");
      if (name == "min" || name == "max") {
        if (e.args.empty()) Error(name + "() expects at least one argument");
        return InferType(e.args[0].get());
      }
      if (name == "clear") return nullptr;
      if (name == "any") return SynthNamed("any");
      if (name == "close" || name == "delete") return nullptr;
      if (name == "make") {
        if (e.args.empty()) Error("make() needs a type");
        return TypeOfTypeExpr(*e.args[0]);
      }
      if (name == "new") {
        if (e.args.empty()) Error("new() needs a type");
        auto t = TypeOfTypeExpr(*e.args[0]);
        return t ? SynthPointer(t) : nullptr;
      }
      if (IsBuiltinTypeName(name)) return SynthNamed(name);
      if (const InterfaceDecl* id = LookupInterface(name)) return SynthNamed(id->name);
      if (LookupAlias(name) || LookupStruct(name)) return SynthNamed(name);
      const FuncDecl* f = LookupFreeFunc(name);
      if (f) return ResultTypeOfCall(*f, "", e.args);
      auto vt = ResolveUnderlying(Lookup(name));
      if (vt && vt->kind == TypeKind::Func) {
        if (vt->func_results.empty()) return nullptr;
        if (vt->func_results.size() == 1) return vt->func_results[0].get();
        Error("cannot call a func value that returns multiple results this way");
      }
      Error("call to undefined function '" + name + "'");
    }
    Error("cannot infer the type of this call expression");
  }

  // Emits `e`, but when `e` is Go's untyped `nil` and the surrounding context
  // supplies an expected type, renders the right C++ "empty" spelling for
  // that type (nullptr for a pointer, {} for a slice/map, "" for
  // string/error) instead of a bare `nullptr` that wouldn't type-check.
  std::string EmitAdapt(const TypeNode* iface_ty, const Expr& e) {
    auto actual = InferType(&e);
    std::string iface = CppType(iface_ty);
    if (actual && IsInterfaceType(actual)) {
      if (actual->name == iface_ty->name && actual->pkg == iface_ty->pkg) return EmitExpr(e);
      // Two distinct interface TYPE NAMES can still be the exact same
      // representation at the C++ level -- e.g. `type Value any`
      // (database/sql/driver's own alias) and `any` itself both resolve,
      // via ResolveUnderlying, to the same bare "any" node, which CppType
      // maps to `wasigo::Any` either way (a transparent alias, not a
      // distinct wrapper), so no reboxing is needed, just emit as-is.
      // Only structurally DIFFERENT interfaces (two independently
      // declared interface types that happen to share a method set)
      // still can't convert this way, since each gets its own dispatch
      // mechanism -- a real interface declaration is never found by
      // ResolveUnderlying's LookupAlias lookup, so this doesn't widen
      // that case.
      const TypeNode* actualU = ResolveUnderlying(actual);
      const TypeNode* ifaceU = ResolveUnderlying(iface_ty);
      if (actualU && ifaceU && actualU->kind == TypeKind::Named && ifaceU->kind == TypeKind::Named &&
          actualU->name == ifaceU->name && actualU->pkg == ifaceU->pkg) {
        return EmitExpr(e);
      }
      Error("cannot convert interface '" + actual->name + "' to '" + iface_ty->name + "'");
    }
    if (actual && actual->kind == TypeKind::Pointer) {
      return iface + "::adapt_ptr(" + EmitExpr(e) + ")";
    }
    if (actual) {
      return iface + "::adapt<" + CppType(actual) + ">(" + EmitExpr(e) + ")";
    }
    return iface + "::adapt(" + EmitExpr(e) + ")";
  }

  // The right "empty" C++ spelling for Go's untyped `nil` at type `t`:
  // nullptr for a pointer, {} for a slice/map/chan/func/interface/`any`,
  // "" for string. Recurses through ResolveUnderlying once before giving
  // up (falling back to `nullptr`, correct for a defined pointer-like
  // alias) so a defined alias of one of those shapes -- most notably
  // `type Value any` (database/sql/driver's own alias) -- gets the SAME
  // spelling its underlying type would, instead of wrongly falling to
  // `nullptr` just because its own bare name isn't literally "any"/
  // "error" and `LookupInterface` doesn't find a TypeAlias declaration.
  std::string NilSpellingFor(const TypeNode* t) const {
    if (!t) return "{}";
    switch (t->kind) {
      case TypeKind::Pointer: return "nullptr";
      case TypeKind::Slice:
      case TypeKind::Map:
      case TypeKind::Chan:
      case TypeKind::Func:
        return "{}";
      case TypeKind::Named: {
        if (t->name == "string") return "\"\"";
        if (t->name == "error" || t->name == "any") return "{}";
        if (LookupInterface(t->name, t->pkg)) return "{}";
        const TypeNode* underlying = ResolveUnderlying(t);
        if (underlying && underlying != t) return NilSpellingFor(underlying);
        return "nullptr";
      }
      default:
        return "{}";
    }
  }

  std::string EmitExprAs(const Expr& e, const TypeNode* expected) {
    if (e.kind == ExprKind::Nil && expected) {
      return NilSpellingFor(expected);
    }
    if (expected && IsInterfaceType(expected) && e.kind != ExprKind::Nil) {
      auto actual = InferType(&e);
      if (!actual || !IsInterfaceType(actual) || actual->name != expected->name ||
          actual->pkg != expected->pkg) {
        return EmitAdapt(expected, e);
      }
    }
    if (expected && IsComplexType(expected)) {
      auto actual = InferType(&e);
      if (!IsComplexType(actual) || actual->name != expected->name) {
        std::string conv =
            expected->name == "complex64" ? "wasigo::as_complex64" : "wasigo::as_complex128";
        return conv + std::string("(") + EmitExpr(e) + ")";
      }
    }
    return EmitExpr(e);
  }

  // `pkg` is the package the callee lives in (empty for a same-package
  // call). A parameter type as parsed inside its own defining file has no
  // need to spell its package (same rule as QualifyResultType's return-type
  // case) -- but a parameter that's an interface used here to adapt an
  // *argument* at a cross-package call boundary (e.g. `io.ReadAll(&b)`
  // adapting `&b` to `io.Reader`) does, or the emitted C++ names the
  // unqualified bare interface type instead of `pkg::Interface`.
  std::string EmitArgsFor(const std::vector<Param>& params, const std::vector<std::unique_ptr<Expr>>& args,
                           const std::string& pkg = "") {
    bool variadic = !params.empty() && params.back().variadic;
    size_t fixed = variadic ? params.size() - 1 : params.size();
    if (args.size() < fixed || (!variadic && args.size() != params.size())) {
      Error("call has " + std::to_string(args.size()) + " argument(s) but " +
            std::to_string(params.size()) + " expected");
    }
    std::ostringstream oss;
    for (size_t i = 0; i < fixed; ++i) {
      if (i) oss << ", ";
      oss << EmitExprAs(*args[i], QualifyResultType(params[i].type.get(), pkg));
    }
    if (variadic) {
      if (fixed) oss << ", ";
      const TypeNode* elem_t = QualifyResultType(params.back().type.get(), pkg);
      if (args.size() == fixed + 1 && args.back() && args.back()->ellipsis) {
        oss << EmitExpr(*args.back());
      } else {
        oss << "wasigo::Slice<" << CppType(elem_t) << ">{";
        for (size_t i = fixed; i < args.size(); ++i) {
          if (i > fixed) oss << ", ";
          if (args[i]->ellipsis) {
            Error("cannot mix unpacked and individual variadic arguments");
          }
          oss << EmitExprAs(*args[i], elem_t);
        }
        oss << "}";
      }
    }
    return oss.str();
  }

  // ---- expressions ----------------------------------------------------------

  std::string EmitExpr(const Expr& e) {
    NoteLoc(e);
    switch (e.kind) {
      // The "LL" suffix matters beyond style: an `auto`-deduced declaration
      // (e.g. `total := 0` -> `auto total = 0;`) would otherwise deduce a
      // plain (32-bit) `int` from a bare literal, silently contradicting
      // this compiler's own "Go's `int` is always 64-bit" mapping (see
      // NamedCppType) the moment such a variable accumulates a value that
      // overflows 32 bits.
      // A Go int LITERAL (as opposed to a negated one -- `-5` parses as
      // UnaryExpr(Minus, IntLit(5)), a separate node) is never negative
      // at the source level, so a negative `e.intval` here can only be
      // the bit-reinterpreted top half of the uint64 range (e.g. a hash
      // constant like 0xa54ff53a5f1d36f1, > INT64_MAX -- see the lexer's
      // stoull-based decimal/hex parse). Printing that as a signed `LL`
      // literal (e.g. "-6534734903238641935LL") is the right BIT
      // PATTERN but the wrong C++ token: a signed literal narrowing-
      // converts into a `uint64_t` brace-init list (`Slice<uint64_t>{...}`,
      // exactly how every hash/crc table here is built) which is ill-
      // formed under real narrowing-conversion rules, not just a
      // warning. Printing the unsigned decimal value with a `ULL` suffix
      // instead is correct either way: for a genuinely uint64-range
      // constant it's the literal Go source meant, and for the ordinary
      // small/positive case this branch is never taken at all.
      case ExprKind::IntLit:
        if (e.intval < 0) return std::to_string(static_cast<uint64_t>(e.intval)) + "ULL";
        return std::to_string(e.intval) + "LL";
      case ExprKind::FloatLit: return FormatDouble(e.floatval);
      case ExprKind::ImagLit:
        return "wasigo::Complex128{0.0, " + FormatDouble(e.floatval) + "}";
      case ExprKind::StringLit: return EscapeCppStringLiteral(e.strval);
      case ExprKind::BoolLit: return e.boolval ? "true" : "false";
      case ExprKind::Nil: return "nullptr";
      case ExprKind::Ident: return CppIdent(e.strval);
      case ExprKind::ParenExpr: return "(" + EmitExpr(*e.x) + ")";
      case ExprKind::Unary: {
        if (e.strval == "&") {
          // `&T{...}` must not take the address of a C++ temporary (that
          // temporary's lifetime ends at the end of the full expression,
          // so a struct literal that legitimately escapes the statement --
          // e.g. `p.next = &Ring{prev: p}` in a linked-structure
          // constructor -- would otherwise leave a dangling pointer, and
          // even `(&Ring{})` alone is ill-formed standard C++ regardless
          // of lifetime). Heap-allocate instead, the same way `new(T)`
          // already does.
          if (e.x->kind == ExprKind::CompositeLit && e.x->type &&
              e.x->type->kind == TypeKind::Named) {
            return EmitCompositeLitPtr(*e.x);
          }
          return "(&" + EmitExpr(*e.x) + ")";
        }
        if (e.strval == "*") return "(*" + EmitExpr(*e.x) + ")";
        if (e.strval == "^") return "(~" + EmitExpr(*e.x) + ")";
        return "(" + e.strval + EmitExpr(*e.x) + ")";
      }
      case ExprKind::Binary: {
        if (e.strval == "==" || e.strval == "!=") {
          bool x_nil = e.x->kind == ExprKind::Nil;
          bool y_nil = e.y->kind == ExprKind::Nil;
          if (x_nil || y_nil) {
            const Expr* other = x_nil ? e.y.get() : e.x.get();
            auto ot = InferType(other);
            std::string other_str = EmitExpr(*other);
            bool eq = e.strval == "==";
            if (ot && (ot->kind == TypeKind::Slice || ot->kind == TypeKind::Map ||
                       ot->kind == TypeKind::Chan || ot->kind == TypeKind::Func)) {
              return std::string(eq ? "wasigo::is_nil(" : "!wasigo::is_nil(") + other_str + ")";
            }
            if (ot && ot->kind == TypeKind::Named &&
                (ot->name == "error" || ot->name == "any" || ot->name == "__recovered" ||
                 ot->name == "string" || LookupInterface(ot->name, ot->pkg))) {
              if (ot->name == "string") {
                return "(" + other_str + (eq ? " == \"\")" : " != \"\")");
              }
              return std::string(eq ? "wasigo::is_nil(" : "!wasigo::is_nil(") + other_str + ")";
            }
            return "(" + other_str + (eq ? " == nullptr)" : " != nullptr)");
          }
        }
        if (e.strval == "+" && e.x->kind == ExprKind::StringLit) {
          // Go string-literal concatenation (`"a" + "b"`, often just line-
          // wrapping one long message) folds to a single constant in real
          // Go, but each side here still emits as its own raw C string
          // literal (EmitExpr(StringLit) -> `"a"`, a `const char*`/array,
          // never wrapped in std::string) -- `"a" + "b"` has no operator+
          // in C++ at all, only `std::string + const char*` does. Only the
          // left operand needs the wrap for that overload to kick in.
          return "(std::string(" + EmitExpr(*e.x) + ") + " + EmitExpr(*e.y) + ")";
        }
        {
          auto lt = InferType(e.x.get());
          auto rt = InferType(e.y.get());
          if (IsComplexType(lt) || IsComplexType(rt)) {
            if (e.strval != "+" && e.strval != "-" && e.strval != "*" && e.strval != "/" &&
                e.strval != "==" && e.strval != "!=") {
              Error("complex values only support +, -, *, /, ==, and !=");
            }
            bool c64 = IsComplexType(lt) && lt->name == "complex64" && IsComplexType(rt) &&
                       rt->name == "complex64";
            const char* conv = c64 ? "wasigo::as_complex64" : "wasigo::as_complex128";
            return "(" + std::string(conv) + "(" + EmitExpr(*e.x) + ") " + e.strval + " " +
                   conv + "(" + EmitExpr(*e.y) + "))";
          }
        }
        std::string result = e.strval == "&^"
                                  ? "(" + EmitExpr(*e.x) + " & ~" + EmitExpr(*e.y) + ")"
                                  : "(" + EmitExpr(*e.x) + " " + e.strval + " " + EmitExpr(*e.y) + ")";
        // Every int LITERAL emits as a 64-bit `LL`/`ULL` C++ token
        // regardless of its Go-level type (see EmitExpr's IntLit case),
        // so an arithmetic/bitwise/shift op on a NARROWER Go integer
        // type (int8/uint8/int16/uint16/int32/rune -- this compiler's
        // `int`/`uint`/`int64`/`uint64` are already 64-bit, so they need
        // no help) that involves a literal operand gets computed in
        // *wider* C++ arithmetic than Go's own per-operation wraparound
        // semantics require (C++'s usual arithmetic conversions widen
        // the narrower operand to match the literal's rank). A value
        // that should have wrapped at 32 (or fewer) bits mid-expression
        // silently doesn't until/unless something later narrows it back
        // -- e.g. `(uint32(x^y) - 1) >> 31` computes `0 - 1` as a plain
        // signed 64-bit -1, not the wrapped 0xFFFFFFFF Go's uint32
        // requires, so the later `>> 31` sign-extends instead of
        // shifting in zero bits (found via crypto/subtle.ConstantTimeByteEq
        // returning -1 instead of 1). Casting the WHOLE result back to
        // its real Go-level width here is safe unconditionally: for an
        // expression that was already computed at the right width (no
        // literal involved, or a same-width non-literal operand), this
        // is an identity no-op; it only changes behavior for the
        // otherwise-silently-wrong widened case.
        if (!IsComparisonOrLogicalOp(e.strval)) {
          auto t = InferType(&e);
          if (t && t->kind == TypeKind::Named && t->pkg.empty()) {
            static const std::set<std::string> kNarrowInt = {
                "int8", "uint8", "byte", "int16", "uint16", "int32", "uint32", "rune"};
            if (kNarrowInt.count(t->name)) {
              return "static_cast<" + NamedCppType(t->name) + ">(" + result + ")";
            }
          }
        }
        return result;
      }
      case ExprKind::Call: return EmitCall(e);
      case ExprKind::Selector: return EmitSelector(e);
      case ExprKind::Index: return EmitIndex(e);
      case ExprKind::CompositeLit: return EmitCompositeLit(e);
      case ExprKind::Recv: {
        std::string ch = EmitExpr(*e.x);
        return current_async_ ? ("co_await (" + ch + ").recv()") : ("(" + ch + ").recv()");
      }
      case ExprKind::SliceExpr: return EmitSliceExpr(e);
      case ExprKind::TypeAssert: {
        if (e.type_switch) Error("x.(type) is only valid as a switch tag");
        auto xt = InferType(e.x.get());
        if (!IsInterfaceType(xt)) Error("type assertion requires an interface value");
        return "(" + EmitExpr(*e.x) + ").must_cast<" + CppType(e.type.get()) + ">()";
      }
      case ExprKind::FuncLit: return EmitFuncLit(e);
    }
    Error("unreachable expression kind");
  }

  std::string EmitMethodValue(const Expr& recv, const std::string& method, bool is_ptr,
                              const std::vector<Param>& params,
                              const std::vector<std::unique_ptr<TypeNode>>& results) {
    if (results.size() > 1) Error("method values with multiple results are not supported");
    std::string rty = results.empty() ? "void" : CppType(results[0].get());
    std::string ps, pdecl, args;
    for (size_t i = 0; i < params.size(); ++i) {
      if (i) {
        ps += ", ";
        pdecl += ", ";
        args += ", ";
      }
      std::string an = "__a" + std::to_string(i);
      std::string ty = CppType(params[i].type.get());
      ps += ty;
      pdecl += ty + " " + an;
      args += an;
    }
    std::string arrow = is_ptr ? "->" : ".";
    std::ostringstream oss;
    oss << "[&]{ auto __mv = " << EmitExpr(recv) << "; return wasigo::Func<" << rty << "(" << ps
        << ")>{[=](" << pdecl << ") { ";
    if (rty != "void") oss << "return ";
    oss << "__mv" << arrow << method << "(" << args << "); }}; }()";
    return oss.str();
  }

  std::string EmitSelector(const Expr& e) {
    if (e.x->kind == ExprKind::Ident && PkgOf(e.x->strval) == "fmt") {
      Error("'fmt' can only be used as fmt.Println(...)/Printf(...)/Sprintf(...)");
    }
    if (e.x->kind == ExprKind::Ident && PkgOf(e.x->strval) == "reflect" && IsReflectKindName(e.strval)) {
      return "static_cast<int64_t>(wasigo::RKind::" + e.strval + ")";
    }
    if (e.x->kind == ExprKind::Ident && PkgOf(e.x->strval) == "os" && e.strval == "Args") {
      return "wasigo::os_args()";
    }
    if (e.x->kind == ExprKind::Ident && PkgOf(e.x->strval) == "os" && e.strval == "Stdout") {
      return "wasigo::os_stdout_file()";
    }
    if (e.x->kind == ExprKind::Ident && PkgOf(e.x->strval) == "os" && e.strval == "Stdin") {
      return "wasigo::os_stdin_file()";
    }
    if (e.x->kind == ExprKind::Ident && PkgOf(e.x->strval) == "os" && e.strval == "Stderr") {
      return "wasigo::os_stderr_file()";
    }
    if (e.x->kind == ExprKind::Ident && IsImportedPackage(e.x->strval)) {
      return QualName(PkgOf(e.x->strval), e.strval);
    }
    auto baseType = InferType(e.x.get());
    if (!baseType) Error("cannot resolve the type of a selector base");
    bool is_ptr = baseType->kind == TypeKind::Pointer;
    const TypeNode* st = is_ptr ? baseType->elem.get() : baseType;
    if (st && st->kind == TypeKind::Named) {
      if (LookupField(st->name, e.strval, st->pkg)) {
        return EmitExpr(*e.x) + (is_ptr ? "->" : ".") + FieldCppName(st->name, e.strval);
      }
      if (const FuncDecl* m = LookupMethod(st->name, e.strval, st->pkg)) {
        return EmitMethodValue(*e.x, m->name, is_ptr, m->params, m->results);
      }
      if (const MethodSig* ms = LookupIfaceMethod(st->name, e.strval, st->pkg)) {
        return EmitMethodValue(*e.x, ms->name, false, ms->params, ms->results);
      }
    }
    return EmitExpr(*e.x) + (is_ptr ? "->" : ".") + CppIdent(e.strval);
  }

  std::string EmitIndex(const Expr& e) {
    auto exprType = InferType(e.x.get());
    auto baseType = ResolveUnderlying(exprType);
    std::string base = EmitExpr(*e.x);
    std::string idx = EmitExpr(*e.y);
    // A named type wrapping []T/[N]T/map[K]V *with at least one method*
    // (EmitAliases' wrapper-struct path -- HasMethodsOn is the exact
    // same predicate EmitAliases itself uses to choose that path) only
    // exposes an implicit conversion to the underlying Slice<T>/
    // array/Map<K,V> -- operator[] isn't found through that via
    // ordinary overload resolution (unlike arithmetic/comparison
    // operators, operator[] specifically isn't part of the "surrogate"
    // built-in candidate set C++ generates from a class's own
    // conversion functions). Index through the wrapper's own `v`
    // member directly (EmitAliases always names it exactly that, and
    // it's a plain public field) rather than through the conversion
    // operator: the conversion operator returns by VALUE, so casting
    // through it would index a throwaway COPY, silently discarding any
    // write (`x[0] = 5` on a std::array-backed wrapper would write into
    // the copy and vanish -- a real regression this exact shape caught
    // when the fix first went through the conversion operator instead
    // of `.v`, on Slice<T>'s to-be-fair-usually-harmless shared_ptr
    // backing, then again on the plain-array case below). `.v` is a
    // real reference into the actual storage either way.
    //
    // Gating on HasMethodsOn specifically (not just "is a Named type")
    // also matters on its own: a plain no-method named type (`type
    // Block [64]int32`) is ALSO reported as TypeKind::Named by
    // InferType, but EmitAliases compiles it to a transparent `using
    // Block = std::array<...>;` with no wrapper struct and no `.v`
    // field at all -- exprType and baseType already agree there, base
    // already IS a real std::array/Slice/Map with a real operator[].
    bool named_container_wrapper =
        exprType && exprType->kind == TypeKind::Named && baseType &&
        (baseType->kind == TypeKind::Slice || baseType->kind == TypeKind::Array ||
         baseType->kind == TypeKind::Map) &&
        HasMethodsOn(exprType->name, exprType->pkg);
    if (named_container_wrapper) base = "(" + base + ").v";
    if (baseType && baseType->kind == TypeKind::Map) {
      return "(" + base + ")[" + idx + "]";
    }
    // `p[i]` where p is `*[N]T`: C++ needs an explicit deref (`base` is a
    // real pointer here, so plain `(base)[idx]` would be pointer
    // arithmetic one level too high, not element access) -- see
    // InferType's matching Index-through-pointer-to-array case.
    if (baseType && baseType->kind == TypeKind::Pointer) {
      const TypeNode* elemUnderlying = ResolveUnderlying(baseType->elem.get());
      if (elemUnderlying && elemUnderlying->kind == TypeKind::Array) {
        return "(*(" + base + "))[static_cast<size_t>(" + idx + ")]";
      }
    }
    // Indexing a Go string yields `byte` (uint8_t) at the Go type-system
    // level (InferType's ExprKind::Index case already says so), but the
    // underlying C++ `operator[]` on the string's std::string backing
    // returns `char`. Every other place a `byte` value flows through this
    // generator, it's already a real uint8_t -- an uncast `char` here is
    // the one place it silently isn't, and differs from uint8_t in
    // signedness. Usually harmless (implicit conversion), but building a
    // []byte composite literal directly from a string-index expression
    // (`[]byte{61, hexDigits[i], ...}`) turns that into a *narrowing*
    // conversion inside a brace-init list, which wasi-sdk clang rejects
    // as a hard error (MSVC only warns) -- found building mime/quotedprintable.
    if (baseType && baseType->kind == TypeKind::Named && baseType->pkg.empty() &&
        baseType->name == "string") {
      return "static_cast<uint8_t>((" + base + ")[static_cast<size_t>(" + idx + ")])";
    }
    return "(" + base + ")[static_cast<size_t>(" + idx + ")]";
  }

  // Pointer form of a struct composite literal (`&T{...}`): allocate on the
  // heap (see the Unary "&" case in EmitExpr for why) and set fields
  // through the resulting pointer, instead of EmitCompositeLit's by-value
  // `T{...}` which is only safe as an rvalue that never outlives the
  // statement.
  std::string EmitCompositeLitPtr(const Expr& e) {
    const TypeNode* t = e.type.get();
    const StructDecl* sd = LookupStruct(t->name, t->pkg);
    if (!sd) {
      // `type IntHeap []int; &IntHeap{5, 2, 8}` -- see EmitCompositeLit's
      // matching comment. `new T(value)` copy-constructs the temporary
      // literal into heap memory, which is fine (unlike `&T{...}`'s
      // address-of-a-temporary): the argument to `new` only needs to live
      // through the constructor call, not past the full expression.
      const TypeNode* rt = ResolveUnderlying(t);
      if (rt && rt != t && (rt->kind == TypeKind::Slice || rt->kind == TypeKind::Map)) {
        return "new " + QualName(t->pkg, t->name) + "(" + EmitCompositeLit(e) + ")";
      }
      Error("composite literal for unknown struct type '" + t->name + "'");
    }
    std::string tn = QualName(t->pkg, t->name);
    if (e.fields.empty() && e.elems.empty()) return "wasigo::New<" + tn + ">()";
    std::ostringstream oss;
    oss << LambdaCapture() << "{ auto* __p = wasigo::New<" << tn << ">(); ";
    if (!e.fields.empty()) {
      for (auto& kv : e.fields) {
        if (kv.first->kind != ExprKind::Ident) {
          Error("a struct literal's field key must be a plain field name");
        }
        const FieldDecl* fd = LookupField(t->name, kv.first->strval, t->pkg);
        if (!fd) Error("unknown field '" + kv.first->strval + "' in a '" + t->name + "' literal");
        // See EmitCompositeLit's matching comment: an embedded field is a
        // base-class subobject, not a named member.
        if (fd->embedded && fd->type && fd->type->kind == TypeKind::Named) {
          std::string embPkg = fd->type->pkg.empty() ? t->pkg : fd->type->pkg;
          oss << "static_cast<" << QualName(embPkg, fd->type->name) << "&>(*__p) = "
              << EmitExprAs(*kv.second, fd->type.get()) << "; ";
        } else {
          oss << "__p->" << FieldCppName(sd->name, kv.first->strval) << " = " << EmitExprAs(*kv.second, fd->type.get()) << "; ";
        }
      }
    } else {
      if (e.elems.size() > sd->fields.size()) {
        Error("too many values in a '" + t->name + "' composite literal");
      }
      for (size_t i = 0; i < e.elems.size(); ++i) {
        const FieldDecl& fdi = sd->fields[i];
        if (fdi.embedded && fdi.type && fdi.type->kind == TypeKind::Named) {
          std::string embPkg = fdi.type->pkg.empty() ? t->pkg : fdi.type->pkg;
          oss << "static_cast<" << QualName(embPkg, fdi.type->name) << "&>(*__p) = "
              << EmitExprAs(*e.elems[i], fdi.type.get()) << "; ";
        } else {
          oss << "__p->" << FieldCppName(sd->name, fdi.name) << " = " << EmitExprAs(*e.elems[i], fdi.type.get()) << "; ";
        }
      }
    }
    oss << "return __p; }()";
    return oss.str();
  }

  std::string EmitCompositeLit(const Expr& e) {
    const TypeNode* t = e.type.get();
    if (t->kind == TypeKind::Named) {
      // `type IntHeap []int; IntHeap{5, 2, 8}`: a defined-over-slice/map
      // type is a C++ `using` (no distinct type -- see ResolveUnderlying),
      // so a slice/map wasigo::Slice<...>{...}/wasigo::Map<...>{...} value
      // literally already *is* one; only fall through to the struct path
      // when the resolved kind isn't Slice/Map.
      const TypeNode* rt = ResolveUnderlying(t);
      if (rt && rt != t && (rt->kind == TypeKind::Slice || rt->kind == TypeKind::Map)) {
        t = rt;
      }
    }
    if (t->kind == TypeKind::Slice) {
      std::string elem_t = CppType(t->elem.get());
      std::ostringstream oss;
      oss << "wasigo::Slice<" << elem_t << ">{";
      for (size_t i = 0; i < e.elems.size(); ++i) {
        if (i) oss << ", ";
        oss << EmitExprAs(*e.elems[i], t->elem.get());
      }
      oss << "}";
      return oss.str();
    }
    if (t->kind == TypeKind::Map) {
      std::string key_t = CppType(t->key.get());
      std::string val_t = CppType(t->elem.get());
      std::ostringstream oss;
      // `map[K]V{}` (zero entries) must be a non-nil *empty* map in Go, but
      // `wasigo::Map<K,V>{}` with empty braces binds to the default ctor
      // (leaves the backing shared_ptr null / nil) rather than the
      // initializer_list ctor -- C++'s "empty braces prefer the default
      // constructor" rule. Route the zero-entry case through make() instead.
      if (e.fields.empty()) {
        oss << "wasigo::Map<" << key_t << ", " << val_t << ">::make()";
        return oss.str();
      }
      oss << "wasigo::Map<" << key_t << ", " << val_t << ">{";
      for (size_t i = 0; i < e.fields.size(); ++i) {
        if (i) oss << ", ";
        oss << "{" << EmitExprAs(*e.fields[i].first, t->key.get()) << ", "
            << EmitExprAs(*e.fields[i].second, t->elem.get()) << "}";
      }
      oss << "}";
      return oss.str();
    }
    if (t->kind == TypeKind::Chan) {
      Error("a channel composite literal is not valid Go; use make(chan T)");
    }
    if (t->kind == TypeKind::Named) {
      const StructDecl* sd = LookupStruct(t->name, t->pkg);
      if (!sd) Error("composite literal for unknown struct type '" + t->name + "'");
      // Plain QualName (no `<Args>`) is fine for a non-generic struct, but
      // a generic one (`Pair[T]{...}`, type args attached by the parser's
      // expression-position instantiation, see ParsePostfix) needs the
      // explicit template arguments here -- `Pair __s{};` relies on CTAD,
      // which fails outright since no constructor argument exists yet to
      // deduce T from. CppType/NamedCppType already knows how to append
      // `<Args>` when t->type_args is non-empty, and is a no-op string
      // (same as QualName) when it's empty.
      std::string tn = CppType(t);
      if (e.fields.empty() && e.elems.empty()) return tn + "{}";
      std::ostringstream oss;
      oss << LambdaCapture() << "{ " << tn << " __s{}; ";
      if (!e.fields.empty()) {
        for (auto& kv : e.fields) {
          if (kv.first->kind != ExprKind::Ident) {
            Error("a struct literal's field key must be a plain field name");
          }
          const FieldDecl* fd = LookupField(t->name, kv.first->strval, t->pkg);
          if (!fd) Error("unknown field '" + kv.first->strval + "' in a '" + t->name + "' literal");
          // An embedded field is a C++ base-class subobject, not a named
          // member (see EmitStructDefs) -- `__s.PublicKey = ...` doesn't
          // name anything (PublicKey there resolves to the base class's
          // own injected-class-name instead), so assign through the base
          // subobject directly, the same `static_cast<Base&>(...)` shape
          // operator== already uses for embedded-field comparison.
          if (fd->embedded && fd->type && fd->type->kind == TypeKind::Named) {
            // An unqualified embedded-field type means a type in the
            // struct's OWN declaring package (t->pkg), not the package
            // whose file is currently being generated -- QualName's
            // pkg-empty case defaults to the latter, so resolve the
            // relative case explicitly here first.
            std::string embPkg = fd->type->pkg.empty() ? t->pkg : fd->type->pkg;
            oss << "static_cast<" << QualName(embPkg, fd->type->name) << "&>(__s) = "
                << EmitExprAs(*kv.second, fd->type.get()) << "; ";
          } else {
            oss << "__s." << FieldCppName(sd->name, kv.first->strval) << " = " << EmitExprAs(*kv.second, fd->type.get()) << "; ";
          }
        }
      } else {
        if (e.elems.size() > sd->fields.size()) {
          Error("too many values in a '" + t->name + "' composite literal");
        }
        for (size_t i = 0; i < e.elems.size(); ++i) {
          const FieldDecl& fdi = sd->fields[i];
          if (fdi.embedded && fdi.type && fdi.type->kind == TypeKind::Named) {
            std::string embPkg = fdi.type->pkg.empty() ? t->pkg : fdi.type->pkg;
            oss << "static_cast<" << QualName(embPkg, fdi.type->name) << "&>(__s) = "
                << EmitExprAs(*e.elems[i], fdi.type.get()) << "; ";
          } else {
            oss << "__s." << FieldCppName(sd->name, fdi.name) << " = " << EmitExprAs(*e.elems[i], fdi.type.get()) << "; ";
          }
        }
      }
      oss << "return __s; }()";
      return oss.str();
    }
    Error("unsupported composite literal type");
  }

  // The C++ expression text for printing `e` the way Go's fmt would
  // (Print*/Sprint*/Errorf/Fprint*'s default formatting, and Printf-family
  // %v/%d/%s/%f/%t -- NOT %c, which wants an actual character; see the
  // verb loops' own %c handling):
  //  - bool -> "true"/"false": `std::cout << bool` prints C++'s default
  //    1/0 instead.
  //  - byte/uint8/int8 -> widened to a >1-byte int: those C++ types are
  //    (signed/unsigned) `char` under the hood, so `std::cout << b` picks
  //    ostream's character overload and prints e.g. 'X' instead of 88 --
  //    Go's fmt always formats a byte numerically, never as a character.
  std::string EmitFmtArg(const Expr& e) {
    auto at = InferType(&e);
    if (at && at->kind == TypeKind::Named && at->pkg.empty()) {
      if (at->name == "bool") return "((" + EmitExpr(e) + ") ? \"true\" : \"false\")";
      if (at->name == "byte" || at->name == "uint8") return "static_cast<uint32_t>(" + EmitExpr(e) + ")";
      if (at->name == "int8") return "static_cast<int32_t>(" + EmitExpr(e) + ")";
    }
    return EmitExpr(e);
  }

  // Emits ` << arg1 << " " << arg2 ...` for args[start:], space-joined the
  // way Print/Println/Sprint/Sprintln/Fprint/Fprintln join their operands.
  // Shared so Fprint/Fprintln (which have a leading writer argument to
  // skip) don't duplicate Print/Println's formatting fixes.
  void EmitJoinedArgs(std::ostringstream& oss, const std::vector<std::unique_ptr<Expr>>& args, size_t start) {
    for (size_t i = start; i < args.size(); ++i) {
      if (i > start) oss << " << \" \"";
      oss << " << " << EmitFmtArg(*args[i]);
    }
  }

  // `os.Stdout`/`os.Stderr` used directly (unaliased) as fmt.Fprint*'s
  // writer argument -- see EmitFprint's comment for why that's the only
  // form supported (os.Stdout isn't a general io.Writer value here).
  bool IsOsStreamSelector(const Expr& e, const char* field) const {
    return e.kind == ExprKind::Selector && e.x && e.x->kind == ExprKind::Ident &&
           PkgOf(e.x->strval) == "os" && e.strval == field;
  }

  // A Writer expression's `.Write(...)` needs `->` instead when its own
  // type is a pointer (e.g. `&b`/a `*bytes.Buffer` var), same rule as
  // EmitSelector's is_ptr check.
  std::string WriteArrow(const Expr& w) {
    auto t = InferType(&w);
    return (t && t->kind == TypeKind::Pointer) ? "->" : ".";
  }

  // fmt.Fprint/Fprintln: `os.Stdout`/`os.Stderr` map straight to
  // std::cout/std::cerr (real Go's fmt.Print* is *defined* as
  // Fprint*(os.Stdout, ...), so this is really the same feature two ways).
  // Anything else must be a concrete Writer value with its own `.Write` --
  // there's no general `os.Stdout` *value* usable through an `io.Writer`
  // variable or interface parameter here, only this direct textual form,
  // since making it one needs a real os.File struct with WASI-backed
  // methods (see README's stdlib tracker: os file fds are still rt/todo).
  std::string EmitFprint(const std::string& name, const std::vector<std::unique_ptr<Expr>>& args) {
    if (args.empty()) Error("fmt." + name + " needs a writer argument");
    bool nl = name == "Fprintln";
    std::ostringstream oss;
    if (IsOsStreamSelector(*args[0], "Stdout")) {
      oss << "(std::cout";
    } else if (IsOsStreamSelector(*args[0], "Stderr")) {
      oss << "(std::cerr";
    } else {
      oss << "((" << EmitExpr(*args[0]) << ")" << WriteArrow(*args[0]) << "Write(wasigo::bytes_from_string([&]{ std::ostringstream __oss; __oss";
      EmitJoinedArgs(oss, args, 1);
      if (nl) oss << " << \"\\n\"";
      oss << "; return __oss.str(); }())))";
      return oss.str();
    }
    EmitJoinedArgs(oss, args, 1);
    if (nl) oss << " << \"\\n\"";
    oss << ")";
    return oss.str();
  }

  std::string EmitBuiltinFmtCall(const std::string& name, const std::vector<std::unique_ptr<Expr>>& args) {
    if (name == "Println" || name == "Print" || name == "Sprint" || name == "Sprintln") {
      bool nl = name == "Println" || name == "Sprintln";
      bool to_str = name == "Sprint" || name == "Sprintln";
      std::ostringstream oss;
      if (to_str) oss << "([&]{ std::ostringstream __oss; __oss";
      else oss << "(std::cout";
      EmitJoinedArgs(oss, args, 0);
      if (nl) oss << " << \"\\n\"";
      if (to_str) oss << "; return __oss.str(); }())";
      else oss << ")";
      return oss.str();
    }
    if (name == "Printf") return EmitPrintf(args, /*to_stream=*/true);
    if (name == "Sprintf") return EmitPrintf(args, /*to_stream=*/false);
    if (name == "Errorf") return EmitErrorf(args);
    if (name == "Fprint" || name == "Fprintln") return EmitFprint(name, args);
    if (name == "Fprintf") return EmitFprintf(args);
    Error("unsupported fmt function '" + name + "' (Print, Println, Sprint, Sprintln, Printf, "
          "Sprintf, Errorf, Fprint, Fprintln, Fprintf -- no Scan*)");
  }

  // fmt.Fprintf: same verb formatting as Printf/Sprintf (see EmitPrintf),
  // routed to std::cout/std::cerr/.Write the way Fprint/Fprintln route
  // theirs (see EmitFprint's comment on why `os.Stdout`/`os.Stderr` must be
  // written directly, unaliased).
  std::string EmitFprintf(const std::vector<std::unique_ptr<Expr>>& args) {
    if (args.size() < 2) Error("fmt.Fprintf needs a writer and a format string");
    bool is_stdout = IsOsStreamSelector(*args[0], "Stdout");
    bool is_stderr = IsOsStreamSelector(*args[0], "Stderr");
    if (args[1]->kind != ExprKind::StringLit) {
      std::string call = EmitDynamicFormatCall(*args[1], args, 2);
      if (is_stdout) return "(std::cout << " + call + ")";
      if (is_stderr) return "(std::cerr << " + call + ")";
      return "((" + EmitExpr(*args[0]) + ")" + WriteArrow(*args[0]) + "Write(wasigo::bytes_from_string(" +
             call + ")))";
    }
    const std::string& fmt = args[1]->strval;
    std::ostringstream stream;
    if (is_stdout) {
      stream << "(std::cout";
    } else if (is_stderr) {
      stream << "(std::cerr";
    } else {
      stream << "((" << EmitExpr(*args[0]) << ")" << WriteArrow(*args[0]) << "Write(wasigo::bytes_from_string([&]{ std::ostringstream __oss; __oss";
    }
    std::string litbuf;
    auto flush = [&]() {
      if (!litbuf.empty()) {
        stream << " << " << EscapeCppStringLiteral(litbuf);
        litbuf.clear();
      }
    };
    size_t argi = 2;
    for (size_t i = 0; i < fmt.size(); ++i) {
      if (fmt[i] == '%' && i + 1 < fmt.size()) {
        char c = fmt[i + 1];
        if (c == '%') {
          litbuf += '%';
          ++i;
          continue;
        }
        if (c == 'd' || c == 's' || c == 'f' || c == 'v' || c == 't' || c == 'c') {
          flush();
          if (argi >= args.size()) Error("fmt.Fprintf: not enough arguments for its format string");
          if (c == 'c') {
            stream << " << static_cast<char>(" << EmitExpr(*args[argi]) << ")";
          } else {
            stream << " << " << EmitFmtArg(*args[argi]);
          }
          ++argi;
          ++i;
          continue;
        }
        Error(std::string("fmt.Fprintf: unsupported format verb '%") + c + "'");
      }
      litbuf += fmt[i];
    }
    flush();
    if (is_stdout || is_stderr) {
      stream << ")";
    } else {
      stream << "; return __oss.str(); }())))";
    }
    return stream.str();
  }

  // fmt.Errorf: same %d/%s/%f/%v/%t/%c formatting as Printf/Sprintf, plus
  // %w (at most one) to wrap another error for errors.Is/Unwrap to walk --
  // see Error::wrapped_ in runtime.hpp.
  std::string EmitErrorf(const std::vector<std::unique_ptr<Expr>>& args) {
    if (args.empty()) Error("fmt.Errorf needs a format string");
    if (args[0]->kind != ExprKind::StringLit) {
      // No %w wrap-tracking here: which arg is the wrapped error depends
      // on the runtime format string's own content, which codegen can't
      // see. FormatPrintf renders a %w verb as plain text (the error's own
      // message, via its ostream operator) so the message still reads
      // right; errors.Is/Unwrap just won't walk into it. Wrapping with a
      // dynamic format string would need parsing %w's position at
      // runtime too -- not worth it for how rarely Errorf's format string
      // is itself dynamic.
      return "wasigo::errors_new(" + EmitDynamicFormatCall(*args[0], args, 1) + ")";
    }
    const std::string& fmt = args[0]->strval;
    std::ostringstream stream;
    stream << "[&]{ std::ostringstream __oss; __oss";
    std::string litbuf;
    auto flush = [&]() {
      if (!litbuf.empty()) {
        stream << " << " << EscapeCppStringLiteral(litbuf);
        litbuf.clear();
      }
    };
    size_t argi = 1;
    std::string wrap_expr;
    for (size_t i = 0; i < fmt.size(); ++i) {
      if (fmt[i] == '%' && i + 1 < fmt.size()) {
        char c = fmt[i + 1];
        if (c == '%') {
          litbuf += '%';
          ++i;
          continue;
        }
        if (c == 'w') {
          flush();
          if (argi >= args.size()) Error("fmt.Errorf: not enough arguments for its format string");
          if (!wrap_expr.empty()) Error("fmt.Errorf: at most one %w verb is supported");
          wrap_expr = EmitExprAs(*args[argi], SynthNamed("error"));
          stream << " << (" << wrap_expr << ")";
          ++argi;
          ++i;
          continue;
        }
        if (c == 'd' || c == 's' || c == 'f' || c == 'v' || c == 't' || c == 'c') {
          flush();
          if (argi >= args.size()) Error("fmt.Errorf: not enough arguments for its format string");
          if (c == 'c') {
            stream << " << static_cast<char>(" << EmitExpr(*args[argi]) << ")";
          } else {
            stream << " << " << EmitFmtArg(*args[argi]);
          }
          ++argi;
          ++i;
          continue;
        }
        Error(std::string("fmt.Errorf: unsupported format verb '%") + c + "'");
      }
      litbuf += fmt[i];
    }
    flush();
    stream << "; return ";
    if (!wrap_expr.empty()) {
      stream << "wasigo::errors_new_wrap(__oss.str(), (" << wrap_expr << "))";
    } else {
      stream << "wasigo::errors_new(__oss.str())";
    }
    stream << "; }()";
    return stream.str();
  }

  // Builds `wasigo::FormatPrintf(fmtExpr, std::vector<wasigo::Any>{adapted
  // args...})` for a Printf-family call whose format string isn't a
  // compile-time literal, so the verb loop below (which walks the
  // literal's actual characters at codegen time) has nothing to walk.
  // Shared by EmitPrintf/EmitFprintf/EmitErrorf's non-literal branch.
  // `...any` spread (`log.Printf`-shaped wrapper: `func Logf(format
  // string, v ...any) { fmt.Printf(format, v...) }`) needs its own case --
  // Go's own type system only allows spreading a `[]any` into `...any`
  // here, so `v` is already `wasigo::Slice<wasigo::Any>` at the C++ level;
  // boxing IT as a single `any` (what the plain per-arg loop below would
  // do) would hand FormatPrintf one argument -- the whole boxed slice --
  // instead of `v`'s own elements, silently producing wrong output rather
  // than the intended per-element formatting.
  std::string EmitDynamicFormatCall(const Expr& fmt_expr, const std::vector<std::unique_ptr<Expr>>& args,
                                     size_t first_arg) {
    std::ostringstream oss;
    oss << "wasigo::FormatPrintf(" << EmitExpr(fmt_expr) << ", ";
    if (args.size() == first_arg + 1 && args[first_arg] && args[first_arg]->ellipsis) {
      oss << "wasigo::AnyVectorFromSlice(" << EmitExpr(*args[first_arg]) << ")";
    } else {
      oss << "std::vector<wasigo::Any>{";
      for (size_t i = first_arg; i < args.size(); ++i) {
        if (i > first_arg) oss << ", ";
        if (args[i]->ellipsis) Error("cannot mix unpacked and individual variadic arguments");
        oss << EmitAdapt(SynthNamed("any"), *args[i]);
      }
      oss << "}";
    }
    oss << ")";
    return oss.str();
  }

  std::string EmitPrintf(const std::vector<std::unique_ptr<Expr>>& args, bool to_stream) {
    if (args.empty()) Error("fmt.Printf/Sprintf needs a format string");
    if (args[0]->kind != ExprKind::StringLit) {
      std::string call = EmitDynamicFormatCall(*args[0], args, 1);
      return to_stream ? "(std::cout << " + call + ")" : call;
    }
    const std::string& fmt = args[0]->strval;
    std::ostringstream stream;
    if (to_stream) {
      stream << "(std::cout";
    } else {
      stream << "[&]{ std::ostringstream __oss; __oss";
    }
    std::string litbuf;
    auto flush = [&]() {
      if (!litbuf.empty()) {
        stream << " << " << EscapeCppStringLiteral(litbuf);
        litbuf.clear();
      }
    };
    size_t argi = 1;
    for (size_t i = 0; i < fmt.size(); ++i) {
      if (fmt[i] == '%' && i + 1 < fmt.size()) {
        char c = fmt[i + 1];
        if (c == '%') {
          litbuf += '%';
          ++i;
          continue;
        }
        if (c == 'd' || c == 's' || c == 'f' || c == 'v' || c == 't' || c == 'c') {
          flush();
          if (argi >= args.size()) Error("fmt.Printf: not enough arguments for its format string");
          // See EmitFmtArg: bool needs "true"/"false" (not C++'s bare 1/0),
          // and byte/int8 need widening so ostream doesn't print them as a
          // character -- except %c, which *wants* a character.
          if (c == 'c') {
            stream << " << static_cast<char>(" << EmitExpr(*args[argi]) << ")";
          } else {
            stream << " << " << EmitFmtArg(*args[argi]);
          }
          ++argi;
          ++i;
          continue;
        }
        Error(std::string("fmt.Printf: unsupported format verb '%") + c + "'");
      }
      litbuf += fmt[i];
    }
    flush();
    if (to_stream) {
      stream << ")";
      return stream.str();
    }
    stream << "; return __oss.str(); }()";
    return stream.str();
  }

  bool IsByteSlice(const TypeNode* t) const {
    return t && t->kind == TypeKind::Slice && t->elem && t->elem->kind == TypeKind::Named &&
           (t->elem->name == "byte" || t->elem->name == "uint8");
  }
  bool IsRuneSlice(const TypeNode* t) const {
    return t && t->kind == TypeKind::Slice && t->elem && t->elem->kind == TypeKind::Named &&
           (t->elem->name == "rune" || t->elem->name == "int32");
  }

  std::string EmitConversionTo(const TypeNode* target, const Expr& src) {
    auto src_type = InferType(&src);
    std::string src_str = EmitExpr(src);
    if (!target) Error("conversion is missing a target type");
    if (target->kind == TypeKind::Named && target->name == "string") {
      if (src_type && src_type->kind == TypeKind::Named && src_type->name == "string") return src_str;
      if (src_type && src_type->kind == TypeKind::Named &&
          (src_type->name == "byte" || src_type->name == "rune" || src_type->name == "int32" ||
           src_type->name == "uint8")) {
        return "std::string(1, static_cast<char>(" + src_str + "))";
      }
      if (IsByteSlice(src_type)) return "wasigo::string_from_bytes(" + src_str + ")";
      if (IsRuneSlice(src_type)) return "wasigo::string_from_runes(" + src_str + ")";
      Error("string(x) is only supported when x is a string, byte/rune, []byte, or []rune; "
            "use fmt.Sprintf to format a number as a string");
    }
    if (IsByteSlice(target)) {
      if (src_type && src_type->kind == TypeKind::Named && src_type->name == "string") {
        return "wasigo::bytes_from_string(" + src_str + ")";
      }
      if (IsByteSlice(src_type)) return src_str;
      Error("[]byte(x) is only supported when x is a string or []byte");
    }
    if (IsRuneSlice(target)) {
      if (src_type && src_type->kind == TypeKind::Named && src_type->name == "string") {
        return "wasigo::runes_from_string(" + src_str + ")";
      }
      if (IsRuneSlice(src_type)) return src_str;
      Error("[]rune(x) is only supported when x is a string or []rune");
    }
    if (target->kind == TypeKind::Named && IsComplexName(target->name)) {
      std::string conv =
          target->name == "complex64" ? "wasigo::as_complex64" : "wasigo::as_complex128";
      return conv + "(" + src_str + ")";
    }
    if (target->kind == TypeKind::Named) {
      return "static_cast<" + CppType(target) + ">(" + src_str + ")";
    }
    Error("unsupported type conversion");
  }

  std::string EmitTypeConversion(const std::string& target, const std::vector<std::unique_ptr<Expr>>& args) {
    if (args.size() != 1) Error("a type conversion takes exactly one argument");
    return EmitConversionTo(SynthNamed(target), *args[0]);
  }

  std::string EmitMake(const Expr& e) {
    if (e.args.empty()) Error("make() needs a type");
    const TypeNode* t = TypeOfTypeExpr(*e.args[0]);
    if (!t) Error("make() needs a type");
    if (t->kind == TypeKind::Slice) {
      std::string n = e.args.size() >= 2 ? EmitExpr(*e.args[1]) : "0";
      std::string c = e.args.size() >= 3 ? EmitExpr(*e.args[2]) : n;
      return "wasigo::make_slice<" + CppType(t->elem.get()) + ">(" + n + ", " + c + ")";
    }
    if (t->kind == TypeKind::Map) {
      return "wasigo::make_map<" + CppType(t->key.get()) + ", " + CppType(t->elem.get()) + ">()";
    }
    if (t->kind == TypeKind::Chan) {
      std::string n = e.args.size() >= 2 ? EmitExpr(*e.args[1]) : "0";
      return "wasigo::make_chan<" + CppType(t->elem.get()) + ">(" + n + ")";
    }
    Error("make() supports slice, map, and chan");
  }

  std::string EmitSliceExpr(const Expr& e) {
    std::string base = EmitExpr(*e.x);
    std::string lo = e.low ? EmitExpr(*e.low) : "0";
    auto bt = InferType(e.x.get());
    if (bt && bt->kind == TypeKind::Named && bt->name == "string") {
      std::string hi = e.high ? EmitExpr(*e.high) : (base + ".size()");
      return "(" + base + ").substr(static_cast<size_t>(" + lo + "), static_cast<size_t>((" + hi +
             ") - (" + lo + ")))";
    }
    std::string hi = e.high ? EmitExpr(*e.high) : "-1";
    if (bt && bt->kind == TypeKind::Array) {
      return "wasigo::slice_array(" + base + ", " + lo + ", " + hi + ")";
    }
    if (e.slice_3) {
      std::string mx = e.max ? EmitExpr(*e.max) : hi;
      return "(" + base + ").slice3(" + lo + ", " + hi + ", " + mx + ")";
    }
    return "(" + base + ").slice(" + lo + ", " + hi + ")";
  }

  std::string AwaitPrefix() const { return current_async_ ? "co_await " : ""; }

  std::string EmitFuncLit(const Expr& e) {
    if (!e.func_lit) Error("internal: missing func literal");
    auto& fl = *e.func_lit;
    bool async = StmtsNeedAwait(fl.body);
    std::ostringstream oss;
    oss << "[" << (async ? "=" : "&") << "](";
    for (size_t i = 0; i < fl.params.size(); ++i) {
      if (i) oss << ", ";
      oss << CppType(fl.params[i].type.get()) << " " << CppIdent(fl.params[i].name);
    }
    oss << ")";
    if (async) {
      if (fl.results.empty()) oss << " -> wasigo::Task";
      else if (fl.results.size() == 1)
        oss << " -> wasigo::TaskT<" << CppType(fl.results[0].get()) << ">";
      else
        Error("a func literal that uses channels cannot return multiple values");
    } else if (fl.results.size() == 1) oss << " -> " << CppType(fl.results[0].get());
    oss << " { ";
    // inline body as a nested emitter: we temporarily write to out_ via a
    // sub-pass by emitting statements into this ostringstream through a
    // local trick -- instead, emit as a statement-block using out_ is wrong
    // here because we're in expression context. Keep func lits as
    // immediately-invoked only when used as go args; for values, emit a
    // lambda whose body we stringify by walking with a nested indent.
    oss << "\n";
    std::string saved_out = "";
    (void)saved_out;
    // We'll emit statements into `oss` by hijacking out_ briefly.
    return EmitFuncLitToString(fl, async);
  }

  std::string EmitFuncLitToString(const FuncLit& fl, bool async) {
    std::ostringstream* prev = nullptr;
    (void)prev;
    std::ostringstream body;
    // Can't rebind out_ (it's a member ostringstream). Emit via a side channel:
    // write a unique lambda using a helper that emits stmts at current indent
    // into out_, but we're inside EmitExpr which returns a string.
    // Strategy: generate the lambda as a string by running a nested Generator
    // is too heavy. Do it by temporarily swapping out_'s content... messy.
    //
    // Practical: emit the lambda body using EmitStmt into out_ is incorrect.
    // We'll build the body with a recursive call that uses a local
    // string-stream by swapping:
    std::ostringstream nested;
    nested.swap(out_);
    int saved_indent = indent_;
    bool saved_async = current_async_;
    current_async_ = async;
    indent_ = 1;
    bool saved_in_lit = in_func_lit_;
    const std::vector<std::unique_ptr<TypeNode>>* saved_lit_results = func_lit_results_;
    in_func_lit_ = true;
    func_lit_results_ = &fl.results;
    PushScope();
    for (auto& p : fl.params) Declare(p.name, p.type.get());
    if (async) {
      if (StmtsHaveDefer(fl.body)) out_ << "  wasigo::DeferList __defers;\n";
    } else if (StmtsHaveDefer(fl.body)) {
      out_ << "  wasigo::PanicFrame __pf;\n";
      out_ << "  wasigo::DeferList __defers;\n";
    }
    for (auto& st : fl.body) EmitStmt(*st);
    if (async) {
      if (fl.results.empty()) out_ << "  co_return;\n";
      else out_ << "  co_return {};\n";
    } else if (StmtsHaveDefer(fl.body)) {
      out_ << "  __wasigo_end: ;\n";
    }
    PopScope();
    current_async_ = saved_async;
    indent_ = saved_indent;
    in_func_lit_ = saved_in_lit;
    func_lit_results_ = saved_lit_results;
    nested.swap(out_);
    std::ostringstream oss;
    oss << "[" << (async ? "=" : "&") << "](";
    for (size_t i = 0; i < fl.params.size(); ++i) {
      if (i) oss << ", ";
      oss << CppType(fl.params[i].type.get()) << " " << CppIdent(fl.params[i].name);
    }
    oss << ")";
    // An async literal captures by value ([=], above), so the closure
    // stays valid past the point the enclosing scope returns -- but a
    // non-mutable lambda's operator() is const, and Go++'s builtin types
    // (Chan, Slice, Map, ...) almost all have non-const mutating methods
    // (Chan::send in particular -- every channel-using async literal
    // calls it), so without `mutable` here any such call on a
    // by-value-captured object fails to compile with "discards
    // qualifiers". `[&]` (the non-async branch) never needs this: a
    // reference isn't const just because operator() is.
    if (async) oss << " mutable";
    if (async) {
      if (fl.results.empty()) oss << " -> wasigo::Task";
      else if (fl.results.size() == 1)
        oss << " -> wasigo::TaskT<" << CppType(fl.results[0].get()) << ">";
    } else if (fl.results.size() == 1) oss << " -> " << CppType(fl.results[0].get());
    oss << " {\n" << nested.str();
    oss << Indent() << "}";
    return oss.str();
  }

  std::string EmitCall(const Expr& e) {
    if (e.callee->kind == ExprKind::CompositeLit && e.callee->type) {
      if (e.args.size() != 1) Error("a type conversion takes exactly one argument");
      return EmitConversionTo(e.callee->type.get(), *e.args[0]);
    }
    if (e.callee->kind == ExprKind::Selector) {
      auto* sel = e.callee.get();
      if (sel->x->kind == ExprKind::Ident && PkgOf(sel->x->strval) == "fmt") {
        return EmitBuiltinFmtCall(sel->strval, e.args);
      }
      if (sel->x->kind == ExprKind::Ident && PkgOf(sel->x->strval) == "errors") {
        if (sel->strval == "New") {
          if (e.args.size() != 1) Error("errors.New() expects exactly one argument");
          return "wasigo::errors_new(" + EmitExpr(*e.args[0]) + ")";
        }
        if (sel->strval == "Is") {
          if (e.args.size() != 2) Error("errors.Is() expects two arguments");
          return "wasigo::errors_is(" + EmitExprAs(*e.args[0], SynthNamed("error")) + ", " +
                 EmitExprAs(*e.args[1], SynthNamed("error")) + ")";
        }
        if (sel->strval == "Unwrap") {
          if (e.args.size() != 1) Error("errors.Unwrap() expects exactly one argument");
          return "wasigo::errors_unwrap(" + EmitExprAs(*e.args[0], SynthNamed("error")) + ")";
        }
        if (sel->strval == "Join") {
          std::ostringstream oss;
          oss << "wasigo::errors_join({";
          for (size_t i = 0; i < e.args.size(); ++i) {
            if (i) oss << ", ";
            oss << EmitExprAs(*e.args[i], SynthNamed("error"));
          }
          oss << "})";
          return oss.str();
        }
        Error("unsupported errors function '" + sel->strval + "' (New, Is, Unwrap, Join -- no As, "
              "needs a runtime type feature this compiler doesn't have)");
      }
      if (sel->x->kind == ExprKind::Ident && PkgOf(sel->x->strval) == "time" && sel->strval == "Now") {
        if (!e.args.empty()) Error("time.Now() expects no arguments");
        // wasigo::time_now() only knows a generic (sec, nsec) pair --
        // runtime.hpp is textually included before the generated `time`
        // namespace exists, so it can't name (or construct) time::Time
        // itself. Build the real Time struct here instead.
        return "([&]{ auto __tp = wasigo::time_now(); " + QualName("time", "Time") +
               " __t{}; __t.sec = __tp.first; __t.nsec = __tp.second; return __t; }())";
      }
      if (sel->x->kind == ExprKind::Ident && PkgOf(sel->x->strval) == "reflect") {
        if (sel->strval == "TypeOf" || sel->strval == "ValueOf") {
          if (e.args.size() != 1) Error("reflect." + sel->strval + "() expects one argument");
          const FuncDecl* f = LookupFreeFunc(sel->strval, "reflect");
          // Type and Value are both just wasigo::Any (see
          // BuildReflectBuiltinFile) -- TypeOf/ValueOf are both just "box
          // the argument", the adapted `any` expression itself IS the
          // result, no wrapper construction needed.
          return EmitArgsFor(f->params, e.args, "reflect");
        }
        Error("unsupported reflect function '" + sel->strval + "' (TypeOf, ValueOf)");
      }
      if (sel->x->kind == ExprKind::Ident && PkgOf(sel->x->strval) == "os") {
        if (sel->strval == "Exit") {
          if (e.args.size() != 1) Error("os.Exit() expects one argument");
          return "wasigo::os_exit(" + EmitExpr(*e.args[0]) + ")";
        }
        if (sel->strval == "Getenv") {
          if (e.args.size() != 1) Error("os.Getenv() expects one argument");
          return "wasigo::os_getenv(" + EmitExpr(*e.args[0]) + ")";
        }
        if (sel->strval == "Open" || sel->strval == "Create" || sel->strval == "ReadFile" ||
            sel->strval == "WriteFile" || sel->strval == "Stat" || sel->strval == "ReadDir") {
          const FuncDecl* f = LookupFreeFunc(sel->strval, "os");
          std::string args = EmitArgsFor(f->params, e.args);
          std::string fn = sel->strval == "Open"       ? "os_open"
                            : sel->strval == "Create"   ? "os_create"
                            : sel->strval == "ReadFile"  ? "os_read_file"
                            : sel->strval == "WriteFile" ? "os_write_file"
                            : sel->strval == "Stat"      ? "os_stat"
                                                          : "os_read_dir";
          return "wasigo::" + fn + "(" + args + ")";
        }
        Error("unsupported os function '" + sel->strval + "' (Args, Exit, Getenv, Open, Create, "
              "ReadFile, WriteFile, Stat, ReadDir)");
      }
      if (sel->x->kind == ExprKind::Ident && PkgOf(sel->x->strval) == "gocvm") {
        if (sel->strval == "Call") {
          // ExprNeedsAwait above always makes any function calling this
          // async, so this is unconditionally a coroutine context --
          // gocvm::CallAsync suspends the calling goroutine instead of
          // blocking the whole cooperative scheduler for the call's
          // duration (see the doc comment on it in runtime.hpp).
          const FuncDecl* f = LookupFreeFunc(sel->strval, "gocvm");
          std::string args = EmitArgsFor(f->params, e.args);
          return "co_await wasigo::gocvm::CallAsync(" + args + ")";
        }
        Error("unsupported gocvm function '" + sel->strval + "' (Call)");
      }
      if (sel->x->kind == ExprKind::Ident && IsImportedPackage(sel->x->strval)) {
        const std::string pkg = PkgOf(sel->x->strval);
        const FuncDecl* f = LookupFreeFunc(sel->strval, pkg);
        if (f) {
          std::string args = EmitArgsFor(f->params, e.args, pkg);
          std::string call = QualName(pkg, sel->strval) + "(" + args + ")";
          if (IsAsyncFree(sel->strval) && current_async_) return "co_await " + call;
          return call;
        }
        // See the matching InferCallType branch: `pkg.Type(x)` is a
        // cross-package named-type conversion (e.g. `time.Duration(n)`),
        // not a call, when `sel->strval` isn't actually a function.
        if (LookupAlias(sel->strval, pkg) || LookupStruct(sel->strval, pkg)) {
          if (e.args.size() != 1) Error("a type conversion takes exactly one argument");
          return EmitConversionTo(SynthNamed(sel->strval, pkg), *e.args[0]);
        }
        Error("call to undefined function '" + sel->x->strval + "." + sel->strval + "'");
      }
      auto baseType = InferType(sel->x.get());
      if (!baseType) Error("cannot resolve the receiver type for method call '" + sel->strval + "'");
      const TypeNode* structT = baseType->kind == TypeKind::Pointer ? baseType->elem.get() : baseType;
      if (structT->kind != TypeKind::Named) Error("method call on a non-struct value");
      // See the matching InferCallType branch for why `error` (a builtin
      // wasigo::Error, not a StructDecl) needs special-casing to answer
      // `err.Error()` at all -- wasigo::Error's own accessor is named
      // `str()`, not `Error()` (avoiding exactly this collision with a Go
      // method also named Error, since C++ has no interfaces of its own to
      // separate them).
      if (structT->name == "error" && structT->pkg.empty() && sel->strval == "Error") {
        return "(" + EmitExpr(*sel->x) + ").str()";
      }
      if (const FuncDecl* m = LookupMethod(structT->name, sel->strval, structT->pkg)) {
        std::string base_str = EmitExpr(*sel->x);
        std::string arrow = baseType->kind == TypeKind::Pointer ? "->" : ".";
        std::string args = EmitArgsFor(m->params, e.args, structT->pkg);
        if (IsAsyncMethod(structT->name, sel->strval) && current_async_) {
          return "co_await " + base_str + arrow + sel->strval + "(" + args + ")";
        }
        return base_str + arrow + sel->strval + "(" + args + ")";
      }
      if (const MethodSig* ms = LookupIfaceMethod(structT->name, sel->strval, structT->pkg)) {
        std::string args = EmitArgsFor(ms->params, e.args, structT->pkg);
        return EmitExpr(*sel->x) + "." + sel->strval + "(" + args + ")";
      }
      // See the matching InferType branch's comment: a struct field of
      // function type, called via `x.f(...)`.
      if (const FieldDecl* fd = LookupField(structT->name, sel->strval, structT->pkg)) {
        const TypeNode* ft = fd->type.get();
        if (ft && ft->kind == TypeKind::Func) {
          std::string base_str = EmitExpr(*sel->x);
          std::string arrow = baseType->kind == TypeKind::Pointer ? "->" : ".";
          std::string args = EmitArgsFor(ft->func_params, e.args, structT->pkg);
          return base_str + arrow + sel->strval + "(" + args + ")";
        }
      }
      Error("unknown method '" + sel->strval + "' on type '" + structT->name + "'");
    }
    if (e.callee->kind == ExprKind::Ident) {
      const std::string& name = e.callee->strval;
      if (name == "len") {
        if (e.args.size() != 1) Error("len() expects exactly one argument");
        return "wasigo::len(" + EmitExpr(*e.args[0]) + ")";
      }
      if (name == "cap") {
        if (e.args.size() != 1) Error("cap() expects exactly one argument");
        return "wasigo::cap(" + EmitExpr(*e.args[0]) + ")";
      }
      if (name == "copy") {
        if (e.args.size() != 2) Error("copy() expects two arguments");
        return "wasigo::copy(" + EmitExpr(*e.args[0]) + ", " + EmitExpr(*e.args[1]) + ")";
      }
      if (name == "close") {
        if (e.args.size() != 1) Error("close() expects one argument");
        return "wasigo::close(" + EmitExpr(*e.args[0]) + ")";
      }
      if (name == "delete") {
        if (e.args.size() != 2) Error("delete() expects two arguments");
        return "wasigo::del(" + EmitExpr(*e.args[0]) + ", " + EmitExpr(*e.args[1]) + ")";
      }
      if (name == "panic") {
        if (e.args.size() != 1) Error("panic() expects one argument");
        return "wasigo::panic(" + EmitExpr(*e.args[0]) + ")";
      }
      if (name == "recover") {
        return "wasigo::recover()";
      }
      if (name == "min") {
        if (e.args.empty()) Error("min() expects at least one argument");
        std::ostringstream oss;
        oss << "wasigo::gmin(" << EmitExpr(*e.args[0]);
        for (size_t i = 1; i < e.args.size(); ++i) oss << ", " << EmitExpr(*e.args[i]);
        oss << ")";
        return oss.str();
      }
      if (name == "max") {
        if (e.args.empty()) Error("max() expects at least one argument");
        std::ostringstream oss;
        oss << "wasigo::gmax(" << EmitExpr(*e.args[0]);
        for (size_t i = 1; i < e.args.size(); ++i) oss << ", " << EmitExpr(*e.args[i]);
        oss << ")";
        return oss.str();
      }
      if (name == "clear") {
        if (e.args.size() != 1) Error("clear() expects exactly one argument");
        return "wasigo::gclear(" + EmitExpr(*e.args[0]) + ")";
      }
      if (name == "any") {
        if (e.args.size() != 1) Error("any() expects exactly one argument");
        return EmitAdapt(SynthNamed("any"), *e.args[0]);
      }
      if (name == "complex") {
        if (e.args.size() != 2) Error("complex() expects two arguments");
        auto t0 = InferType(e.args[0].get());
        auto t1 = InferType(e.args[1].get());
        if (IsFloat32Type(t0) && IsFloat32Type(t1)) {
          return "wasigo::Complex64{" + EmitExpr(*e.args[0]) + ", " + EmitExpr(*e.args[1]) + "}";
        }
        return "wasigo::Complex128{" + EmitExpr(*e.args[0]) + ", " + EmitExpr(*e.args[1]) + "}";
      }
      if (name == "real") {
        if (e.args.size() != 1) Error("real() expects one argument");
        if (!IsComplexType(InferType(e.args[0].get()))) Error("real() requires a complex argument");
        return "wasigo::creal(" + EmitExpr(*e.args[0]) + ")";
      }
      if (name == "imag") {
        if (e.args.size() != 1) Error("imag() expects one argument");
        if (!IsComplexType(InferType(e.args[0].get()))) Error("imag() requires a complex argument");
        return "wasigo::cimag(" + EmitExpr(*e.args[0]) + ")";
      }
      if (name == "make") return EmitMake(e);
      if (name == "new") {
        if (e.args.size() != 1) Error("new() expects one argument");
        auto t = TypeOfTypeExpr(*e.args[0]);
        return "wasigo::New<" + CppType(t) + ">()";
      }
      if (name == "append") {
        if (e.args.empty()) Error("append() expects at least one argument");
        if (e.args.size() == 2 && e.args[1]->ellipsis) {
          return "wasigo::append_range(" + EmitExpr(*e.args[0]) + ", " + EmitExpr(*e.args[1]) + ")";
        }
        std::ostringstream oss;
        oss << "wasigo::append(" << EmitExpr(*e.args[0]);
        auto t = InferType(e.args[0].get());
        for (size_t i = 1; i < e.args.size(); ++i) {
          if (e.args[i]->ellipsis) Error("append unpack (...) is only supported as the last extra argument");
          oss << ", " << (t ? EmitExprAs(*e.args[i], t->elem.get()) : EmitExpr(*e.args[i]));
        }
        oss << ")";
        return oss.str();
      }
      if (IsBuiltinTypeName(name)) return EmitTypeConversion(name, e.args);
      if (const InterfaceDecl* id = LookupInterface(name)) {
        if (e.args.size() != 1) Error("an interface conversion takes exactly one argument");
        return EmitAdapt(SynthNamed(id->name), *e.args[0]);
      }
      if (LookupAlias(name) || LookupStruct(name)) {
        if (e.args.size() != 1) Error("a type conversion takes exactly one argument");
        return EmitConversionTo(SynthNamed(name), *e.args[0]);
      }
      const FuncDecl* f = LookupFreeFunc(name);
      if (f) {
        std::string args = EmitArgsFor(f->params, e.args);
        if (IsAsyncFree(name) && current_async_) return "co_await " + name + "(" + args + ")";
        return name + "(" + args + ")";
      }
      auto vt = ResolveUnderlying(Lookup(name));
      if (vt && vt->kind == TypeKind::Func) {
        std::ostringstream oss;
        oss << "(" << CppIdent(name) << ")(";
        for (size_t i = 0; i < e.args.size(); ++i) {
          if (i) oss << ", ";
          // EmitExprAs, not EmitExpr: an untyped `nil` argument (e.g. a
          // `func(..., error) ...`-typed callback called as `fn(x, nil)`,
          // found porting go++/stdlib/path/filepath's new WalkDir) needs
          // the same nil-spelling-by-target-type NilSpellingFor already
          // gives ordinary package-function/method calls via EmitArgsFor
          // -- plain EmitExpr always spells Nil as `nullptr`, which only
          // compiles for a pointer parameter, not an error/interface/
          // slice/map one (wasigo::Error has no nullptr_t constructor).
          const TypeNode* pt = i < vt->func_params.size() ? vt->func_params[i].type.get() : nullptr;
          oss << (pt ? EmitExprAs(*e.args[i], pt) : EmitExpr(*e.args[i]));
        }
        oss << ")";
        return oss.str();
      }
      Error("call to undefined function '" + name + "'");
    }
    std::ostringstream oss;
    oss << "(" << EmitExpr(*e.callee) << ")(";
    for (size_t i = 0; i < e.args.size(); ++i) {
      if (i) oss << ", ";
      oss << EmitExpr(*e.args[i]);
    }
    oss << ")";
    return oss.str();
  }

  // ---- statements -------------------------------------------------------------

  void EmitStmtList(const std::vector<std::unique_ptr<Stmt>>& stmts) {
    PushScope();
    for (auto& s : stmts) EmitStmt(*s);
    PopScope();
  }

  void EmitStmt(const Stmt& s) {
    NoteLoc(s);
    switch (s.kind) {
      case StmtKind::Var: EmitVarStmt(s); return;
      case StmtKind::ShortVarDecl: EmitShortVarDecl(s); return;
      case StmtKind::Assign: EmitAssign(s); return;
      case StmtKind::IncDec:
        out_ << Indent() << EmitExpr(*s.lhs[0]) << s.op << ";\n";
        return;
      case StmtKind::ExprStmt:
        if (current_has_defers_ && !current_async_ && s.lhs[0] &&
            s.lhs[0]->kind == ExprKind::Call && s.lhs[0]->callee &&
            s.lhs[0]->callee->kind == ExprKind::Ident && s.lhs[0]->callee->strval == "panic" &&
            s.lhs[0]->args.size() == 1) {
          out_ << Indent() << "__pf.has_pending = true;\n";
          out_ << Indent() << "__pf.pending = " << EmitExpr(*s.lhs[0]->args[0]) << ";\n";
          out_ << Indent() << "goto __wasigo_end;\n";
          return;
        }
        out_ << Indent() << EmitExpr(*s.lhs[0]) << ";\n";
        return;
      case StmtKind::Return: EmitReturn(s); return;
      case StmtKind::If: EmitIf(s); return;
      case StmtKind::ForClassic: EmitForClassic(s); return;
      case StmtKind::ForCond: EmitForCond(s); return;
      case StmtKind::ForInfinite: EmitForInfinite(s); return;
      case StmtKind::ForRange: EmitForRange(s); return;
      case StmtKind::Block:
        out_ << Indent() << "{\n";
        indent_++;
        EmitStmtList(s.body);
        indent_--;
        out_ << Indent() << "}\n";
        return;
      case StmtKind::Break: EmitBreak(s); return;
      case StmtKind::Continue: EmitContinue(s); return;
      case StmtKind::Labeled: EmitLabeled(s); return;
      case StmtKind::Goto:
        if (s.names.empty()) Error("goto needs a label");
        out_ << Indent() << "goto " << CppIdent(s.names[0]) << ";\n";
        return;
      case StmtKind::Fallthrough:
        Error("fallthrough is only supported as the last statement of a switch case and is "
              "emitted as a goto; use adjacent cases if you meant to share a body");
        return;
      case StmtKind::Go:
        EmitGo(s);
        return;
      case StmtKind::Defer:
        EmitDefer(s);
        return;
      case StmtKind::Send:
        EmitSend(s);
        return;
      case StmtKind::Switch:
        if (s.type_switch) EmitTypeSwitch(s);
        else EmitSwitch(s);
        return;
      case StmtKind::Select:
        EmitSelect(s);
        return;
    }
  }

  void EmitGo(const Stmt& s) {
    program_has_go_ = true;
    const Expr& e = *s.lhs[0];
    if (e.kind == ExprKind::Call) {
      if (e.callee->kind == ExprKind::Ident && IsAsyncFree(e.callee->strval)) {
        const FuncDecl* f = LookupFreeFunc(e.callee->strval);
        std::string args = f ? EmitArgsFor(f->params, e.args) : "";
        out_ << Indent() << "wasigo::go(" << e.callee->strval << "(" << args << "));\n";
        return;
      }
      if (e.callee->kind == ExprKind::Selector && e.callee->x && e.callee->x->kind == ExprKind::Ident &&
          IsImportedPackage(e.callee->x->strval) && IsAsyncFree(e.callee->strval)) {
        const std::string pkg = PkgOf(e.callee->x->strval);
        const FuncDecl* f = LookupFreeFunc(e.callee->strval, pkg);
        std::string args = f ? EmitArgsFor(f->params, e.args, pkg) : "";
        out_ << Indent() << "wasigo::go(" << QualName(pkg, e.callee->strval) << "(" << args << "));\n";
        return;
      }
      // `go recv.AsyncMethod(...)` (recv.AsyncMethod itself uses channels,
      // so it returns wasigo::Task/TaskT, not the "ordinary callable" the
      // generic fallback below assumes). That fallback wraps the call in
      // `wasigo::go([=]{ recv.AsyncMethod(...); })`: a plain function-call
      // *statement* on a Task-returning method constructs the Task,
      // immediately discards it (dtor runs at the end of the full
      // expression since it was never passed to go() or co_awaited), and
      // ~Task destroys the not-yet-started coroutine frame outright --
      // the method body never runs at all, and anything co_awaiting a
      // result from it (e.g. a "done" channel it's supposed to close)
      // deadlocks forever. Pass the Task straight to wasigo::go(...)
      // instead, same as the free-function/package-function cases above.
      if (e.callee->kind == ExprKind::Selector && e.callee->x &&
          !(e.callee->x->kind == ExprKind::Ident && IsImportedPackage(e.callee->x->strval))) {
        auto baseType = InferType(e.callee->x.get());
        const TypeNode* structT =
            baseType && baseType->kind == TypeKind::Pointer ? baseType->elem.get() : baseType;
        if (structT && structT->kind == TypeKind::Named) {
          if (const FuncDecl* m = LookupMethod(structT->name, e.callee->strval, structT->pkg)) {
            if (IsAsyncMethod(structT->name, e.callee->strval)) {
              std::string base_str = EmitExpr(*e.callee->x);
              std::string arrow = baseType->kind == TypeKind::Pointer ? "->" : ".";
              std::string args = EmitArgsFor(m->params, e.args, structT->pkg);
              out_ << Indent() << "wasigo::go(" << base_str << arrow << e.callee->strval << "("
                   << args << "));\n";
              return;
            }
          }
        }
      }
      if (e.callee->kind == ExprKind::FuncLit) {
        EmitGoFuncLit(*e.callee);
        return;
      }
      // go f() for a sync f, including fmt / builtins
      out_ << Indent() << "wasigo::go([=]{ " << EmitExpr(e) << "; });\n";
      return;
    }
    if (e.kind == ExprKind::FuncLit) {
      EmitGoFuncLit(e);
      return;
    }
    out_ << Indent() << "wasigo::go([=]{ " << EmitExpr(e) << "; });\n";
  }

  // `go func(){...}()`: see the GoAsyncLit/GoAsyncLitT doc comment in
  // runtime.hpp for why a channel-using literal can't be immediately
  // invoked and handed to wasigo::go(...) directly (dangling closure).
  // A non-async literal has no such hazard once it's deferred behind an
  // ordinary (non-coroutine) wrapper -- same "go f()" fallback shape as
  // every other synchronous go target already uses -- so only the async
  // case needs the GoAsyncLit(T) indirection.
  void EmitGoFuncLit(const Expr& lit) {
    if (!lit.func_lit) Error("internal: go target is not a func literal");
    bool async = StmtsNeedAwait(lit.func_lit->body);
    if (!async) {
      out_ << Indent() << "wasigo::go([=]{ (" << EmitExpr(lit) << ")(); });\n";
      return;
    }
    const auto& results = lit.func_lit->results;
    if (results.empty()) {
      out_ << Indent() << "wasigo::go(wasigo::GoAsyncLit(" << EmitExpr(lit) << "));\n";
    } else if (results.size() == 1) {
      out_ << Indent() << "wasigo::go(wasigo::GoAsyncLitT<" << CppType(results[0].get()) << ">("
           << EmitExpr(lit) << "));\n";
    } else {
      Error("a func literal that uses channels cannot return multiple values");
    }
  }

  void EmitDefer(const Stmt& s) {
    current_has_defers_ = true;
    const Expr& e = *s.lhs[0];
    // Go evaluates the function and arguments now; the call runs later.
    if (e.kind != ExprKind::Call) {
      out_ << Indent() << "__defers.push([=]{ " << EmitExpr(e) << "; });\n";
      return;
    }
    std::vector<std::string> at;
    at.reserve(e.args.size());
    for (size_t i = 0; i < e.args.size(); ++i) {
      std::string t = "__da" + std::to_string(temp_id_++);
      out_ << Indent() << "auto " << t << " = " << EmitExpr(*e.args[i]) << ";\n";
      at.push_back(t);
    }
    std::string recv;
    if (e.callee && e.callee->kind == ExprKind::Selector) {
      recv = "__dr" + std::to_string(temp_id_++);
      out_ << Indent() << "auto " << recv << " = " << EmitExpr(*e.callee->x) << ";\n";
    }
    // `defer func(){ ... }()` -- an immediately-invoked func literal --
    // falls through to the plain EmitExpr(e) branch below, which doesn't
    // use the precomputed `at`/`recv` temps at all (it just re-emits the
    // whole call expression, literal body included). That inner literal
    // already captures its enclosing scope `[&]` (EmitExpr's normal func
    // literal rule); wrapping it in an OUTER `[=]` here breaks that --
    // the outer lambda's by-value copy of e.g. a named return variable
    // like `err` is const (default for a by-value lambda capture), and
    // the inner `[&]` ends up referring to THAT const copy, not the
    // function's real `err` -- "err = ..." inside the deferred recover()
    // handler then fails to compile ("passing const Error as this
    // argument"), or worse, would silently write through to a copy that
    // never reaches the caller if capture-by-value ever did compile.
    // Reference capture here is safe (unlike a closure that escapes via
    // return): DeferList is a local destroyed during the SAME function's
    // own unwind, strictly before any variable declared earlier in the
    // function (named returns included) is itself destroyed. Named/
    // selector-call defers still need `[=]` -- their `at`/`recv` temps
    // are intentionally value-captured so Go's "evaluate defer args now"
    // semantics survive past those temps' own (earlier) destruction.
    bool literal_call = !(e.callee && (e.callee->kind == ExprKind::Selector ||
                                        e.callee->kind == ExprKind::Ident));
    out_ << Indent() << "__defers.push([" << (literal_call ? "&" : "=") << "]{ ";
    if (e.callee && e.callee->kind == ExprKind::Selector) {
      auto baseType = InferType(e.callee->x.get());
      bool is_ptr = baseType && baseType->kind == TypeKind::Pointer;
      bool pkgfn = e.callee->x->kind == ExprKind::Ident &&
                   (PkgOf(e.callee->x->strval) == "fmt" || PkgOf(e.callee->x->strval) == "errors" ||
                    PkgOf(e.callee->x->strval) == "os" || IsImportedPackage(e.callee->x->strval));
      if (pkgfn && PkgOf(e.callee->x->strval) == "fmt") {
        out_ << "std::cout";
        for (size_t i = 0; i < at.size(); ++i) {
          if (i) out_ << " << \" \"";
          out_ << " << " << at[i];
        }
        if (e.callee->strval == "Println") out_ << " << \"\\n\"";
        out_ << ";";
      } else if (pkgfn) {
        out_ << QualName(PkgOf(e.callee->x->strval), e.callee->strval) << "(";
        for (size_t i = 0; i < at.size(); ++i) {
          if (i) out_ << ", ";
          out_ << at[i];
        }
        out_ << ");";
      } else {
        out_ << recv << (is_ptr ? "->" : ".") << CppIdent(e.callee->strval) << "(";
        for (size_t i = 0; i < at.size(); ++i) {
          if (i) out_ << ", ";
          out_ << at[i];
        }
        out_ << ");";
      }
    } else if (e.callee && e.callee->kind == ExprKind::Ident) {
      out_ << CppIdent(e.callee->strval) << "(";
      for (size_t i = 0; i < at.size(); ++i) {
        if (i) out_ << ", ";
        out_ << at[i];
      }
      out_ << ");";
    } else {
      out_ << EmitExpr(e) << ";";
    }
    out_ << " });\n";
  }

  void EmitSend(const Stmt& s) {
    std::string ch = EmitExpr(*s.lhs[0]);
    std::string v = EmitExpr(*s.rhs[0]);
    if (current_async_) {
      out_ << Indent() << "co_await (" << ch << ").send(" << v << ");\n";
    } else {
      Error("channel send is only valid in a function that uses goroutines/channels "
            "(it becomes a C++20 coroutine)");
    }
  }

  void EmitTypeSwitch(const Stmt& s) {
    if (!s.cond) Error("type switch is missing a tag");
    if (s.init) {
      out_ << Indent() << "{\n";
      indent_++;
      EmitStmt(*s.init);
    }
    PushScope();
    std::string tag = "__ts" + std::to_string(temp_id_++);
    out_ << Indent() << "auto " << tag << " = " << EmitExpr(*s.cond) << ";\n";
    std::string bound;
    if (!s.names.empty() && s.names[0] != "_") bound = s.names[0];
    int def = -1;
    bool first = true;
    for (size_t i = 0; i < s.cases.size(); ++i) {
      auto& c = s.cases[i];
      if (c.types.empty()) {
        def = static_cast<int>(i);
        continue;
      }
      if (CaseEndsWithFallthrough(c)) Error("cannot fallthrough in a type switch");
      out_ << Indent();
      if (!first) out_ << "else ";
      first = false;
      bool nil_case = c.types.size() == 1 && c.types[0] && c.types[0]->name == "nil";
      if (nil_case) {
        out_ << "if (" << tag << ".is_nil()) {\n";
        indent_++;
        PushScope();
        if (!bound.empty()) {
          auto xt = InferType(s.cond.get());
          out_ << Indent() << CppType(xt) << " " << CppIdent(bound) << " = " << tag << ";\n";
          Declare(bound, xt);
        }
        EmitStmtList(c.body);
        PopScope();
        indent_--;
        out_ << Indent() << "}\n";
        continue;
      }
      if (c.types.size() == 1) {
        std::string ct = CppType(c.types[0].get());
        std::string p = "__tp" + std::to_string(temp_id_++);
        out_ << "if (auto " << p << " = " << tag << ".try_cast<" << ct << ">(); " << p
             << ".second) {\n";
        indent_++;
        PushScope();
        if (!bound.empty()) {
          out_ << Indent() << "auto " << CppIdent(bound) << " = " << p << ".first;\n";
          Declare(bound, c.types[0].get());
        }
        EmitStmtList(c.body);
        PopScope();
        indent_--;
        out_ << Indent() << "}\n";
        continue;
      }
      out_ << "if (";
      for (size_t t = 0; t < c.types.size(); ++t) {
        if (t) out_ << " || ";
        out_ << tag << ".try_cast<" << CppType(c.types[t].get()) << ">().second";
      }
      out_ << ") {\n";
      indent_++;
      PushScope();
      if (!bound.empty()) {
        auto xt = InferType(s.cond.get());
        out_ << Indent() << CppType(xt) << " " << CppIdent(bound) << " = " << tag << ";\n";
        Declare(bound, xt);
      }
      EmitStmtList(c.body);
      PopScope();
      indent_--;
      out_ << Indent() << "}\n";
    }
    if (def >= 0) {
      out_ << Indent();
      if (!first) out_ << "else ";
      out_ << "{\n";
      indent_++;
      PushScope();
      if (!bound.empty()) {
        auto xt = InferType(s.cond.get());
        out_ << Indent() << CppType(xt) << " " << CppIdent(bound) << " = " << tag << ";\n";
        Declare(bound, xt);
      }
      EmitStmtList(s.cases[static_cast<size_t>(def)].body);
      PopScope();
      indent_--;
      out_ << Indent() << "}\n";
    }
    PopScope();
    if (s.init) {
      indent_--;
      out_ << Indent() << "}\n";
    }
  }

  static bool CaseEndsWithFallthrough(const SwitchCase& c) {
    return !c.body.empty() && c.body.back() && c.body.back()->kind == StmtKind::Fallthrough;
  }

  void EmitSwitch(const Stmt& s) {
    bool wrap = s.init != nullptr;
    if (wrap) {
      out_ << Indent() << "{\n";
      indent_++;
    }
    PushScope();
    if (s.init) EmitStmt(*s.init);
    const int id = temp_id_++;
    std::string end = "__swend" + std::to_string(id);
    std::string tag;
    if (s.cond) {
      tag = "__sw" + std::to_string(id);
      out_ << Indent() << "auto " << tag << " = " << EmitExpr(*s.cond) << ";\n";
    }
    auto caselabel = [&](size_t i) { return "__swc" + std::to_string(id) + "_" + std::to_string(i); };
    int def = -1;
    for (size_t i = 0; i < s.cases.size(); ++i) {
      auto& c = s.cases[i];
      if (c.values.empty()) {
        def = static_cast<int>(i);
        continue;
      }
      out_ << Indent() << "if (";
      for (size_t v = 0; v < c.values.size(); ++v) {
        if (v) out_ << " || ";
        if (!tag.empty()) {
          out_ << "(" << tag << " == " << EmitExpr(*c.values[v]) << ")";
        } else {
          out_ << "(" << EmitExpr(*c.values[v]) << ")";
        }
      }
      out_ << ") goto " << caselabel(i) << ";\n";
    }
    if (def >= 0) {
      out_ << Indent() << "goto " << caselabel(static_cast<size_t>(def)) << ";\n";
    } else {
      out_ << Indent() << "goto " << end << ";\n";
    }
    {
      JumpFrame sw;
      sw.brk = end;
      if (!pending_label_.empty()) {
        sw.name = pending_label_;
        pending_label_.clear();
      }
      jump_stack_.push_back(sw);
    }
    for (size_t i = 0; i < s.cases.size(); ++i) {
      auto& c = s.cases[i];
      out_ << Indent() << caselabel(i) << ": {\n";
      indent_++;
      PushScope();
      bool ft = CaseEndsWithFallthrough(c);
      for (size_t k = 0; k < c.body.size(); ++k) {
        if (ft && k + 1 == c.body.size()) break;
        EmitStmt(*c.body[k]);
      }
      if (ft) {
        if (i + 1 >= s.cases.size()) {
          Error("cannot fallthrough the last case of a switch");
        }
        out_ << Indent() << "goto " << caselabel(i + 1) << ";\n";
      } else {
        out_ << Indent() << "goto " << end << ";\n";
      }
      PopScope();
      indent_--;
      out_ << Indent() << "}\n";
    }
    out_ << Indent() << end << ": ;\n";
    jump_stack_.pop_back();
    PopScope();
    if (wrap) {
      indent_--;
      out_ << Indent() << "}\n";
    }
  }

  void EmitSelect(const Stmt& s) {
    if (!current_async_) Error("select is only valid in a coroutine (a function that uses channels)");
    struct Slot {
      std::string name;
      std::string okname;
      bool has_ok = false;
    };
    std::vector<Slot> slots(s.sel_cases.size());
    // GSelect::recv/send take `Chan<T>&` (a non-const lvalue reference), so
    // a channel *expression* that isn't already a plain variable -- e.g.
    // `case <-ctx.Done():`, a method call returning a channel -- needs
    // binding to a named local first; a call result is a C++ rvalue and
    // won't bind to that reference parameter directly. Chan<T> is a cheap
    // handle (shared_ptr-backed), so the copy into the local keeps
    // referring to the same underlying channel.
    std::vector<std::string> chan_vars(s.sel_cases.size());
    for (size_t i = 0; i < s.sel_cases.size(); ++i) {
      auto& c = s.sel_cases[i];
      if (c.is_default) continue;
      chan_vars[i] = "__sc" + std::to_string(temp_id_++);
      out_ << Indent() << "auto " << chan_vars[i] << " = " << EmitExpr(*c.chan) << ";\n";
    }
    for (size_t i = 0; i < s.sel_cases.size(); ++i) {
      auto& c = s.sel_cases[i];
      if (c.is_default || c.is_send) continue;
      auto ct = InferType(c.chan.get());
      const TypeNode* elem = (ct && ct->kind == TypeKind::Chan) ? ct->elem.get() : SynthNamed("int");
      slots[i].name = "__sv" + std::to_string(temp_id_++);
      out_ << Indent() << CppType(elem) << " " << slots[i].name << "{};\n";
      if (c.recv_ok) {
        slots[i].has_ok = true;
        slots[i].okname = "__so" + std::to_string(temp_id_++);
        out_ << Indent() << "bool " << slots[i].okname << " = false;\n";
      }
    }
    std::string idx = "__sel" + std::to_string(temp_id_++);
    out_ << Indent() << "int " << idx << " = co_await wasigo::GSelect{}";
    for (size_t i = 0; i < s.sel_cases.size(); ++i) {
      auto& c = s.sel_cases[i];
      if (c.is_default) {
        out_ << "\n" << Indent() << "  .deflt()";
      } else if (c.is_send) {
        out_ << "\n" << Indent() << "  .send(" << chan_vars[i] << ", " << EmitExpr(*c.value)
             << ")";
      } else {
        out_ << "\n" << Indent() << "  .recv(" << chan_vars[i] << ", &" << slots[i].name;
        if (slots[i].has_ok) out_ << ", &" << slots[i].okname;
        out_ << ")";
      }
    }
    out_ << ";\n";
    for (size_t i = 0; i < s.sel_cases.size(); ++i) {
      auto& c = s.sel_cases[i];
      out_ << Indent() << (i ? "else " : "") << "if (" << idx << " == " << i << ") {\n";
      indent_++;
      PushScope();
      if (!c.is_default && !c.is_send) {
        if (c.recv_define) {
          if (!c.recv_names.empty() && c.recv_names[0] != "_") {
            out_ << Indent() << "auto " << c.recv_names[0] << " = std::move(" << slots[i].name
                 << ");\n";
            auto ct = InferType(c.chan.get());
            Declare(c.recv_names[0],
                    (ct && ct->kind == TypeKind::Chan) ? ct->elem.get() : SynthNamed("int"));
          }
          if (c.recv_ok && c.recv_names.size() > 1 && c.recv_names[1] != "_") {
            out_ << Indent() << "auto " << c.recv_names[1] << " = " << slots[i].okname << ";\n";
            Declare(c.recv_names[1], SynthNamed("bool"));
          }
        } else if (!c.recv_names.empty()) {
          if (c.recv_names[0] != "_") {
            out_ << Indent() << c.recv_names[0] << " = std::move(" << slots[i].name << ");\n";
          }
          if (c.recv_ok && c.recv_names.size() > 1 && c.recv_names[1] != "_") {
            out_ << Indent() << c.recv_names[1] << " = " << slots[i].okname << ";\n";
          }
        }
      }
      EmitStmtList(c.body);
      PopScope();
      indent_--;
      out_ << Indent() << "}\n";
    }
  }

  void EmitVarStmt(const Stmt& s) {
    for (size_t i = 0; i < s.names.size(); ++i) {
      const std::string& name = s.names[i];
      bool has_init = i < s.rhs.size();
      if (has_init) {
        const TypeNode* declared = s.var_type.get();
        std::string rhs_str = declared ? EmitExprAs(*s.rhs[i], declared) : EmitExpr(*s.rhs[i]);
        if (s.is_const && i < s.rhs.size() && !const_inits_.empty()) {
          const_inits_.back()[name] = s.rhs[i].get();
        }
        if (declared) {
          out_ << Indent() << CppType(declared) << " " << CppIdent(name) << " = " << rhs_str
               << ";\n";
          Declare(name, declared);
        } else {
          out_ << Indent() << "auto " << CppIdent(name) << " = " << rhs_str << ";\n";
          Declare(name, InferType(s.rhs[i].get()));
        }
      } else {
        if (!s.var_type) Error("var '" + name + "' needs either a type or an initializer");
        out_ << Indent() << CppType(s.var_type.get()) << " " << CppIdent(name) << "{};\n";
        Declare(name, s.var_type.get());
      }
    }
  }

  void EmitShortVarDecl(const Stmt& s) {
    if (s.names.size() == 1 && s.rhs.size() == 1) {
      const TypeNode* ty = InferType(s.rhs[0].get());
      std::string rhs_str = ty ? EmitExprAs(*s.rhs[0], ty) : EmitExpr(*s.rhs[0]);
      if (ty) {
        out_ << Indent() << CppType(ty) << " " << CppIdent(s.names[0]) << " = " << rhs_str << ";\n";
      } else {
        out_ << Indent() << "auto " << CppIdent(s.names[0]) << " = " << rhs_str << ";\n";
      }
      Declare(s.names[0], ty);
      return;
    }
    if (s.names.size() == s.rhs.size()) {
      std::vector<std::string> temps;
      std::vector<const TypeNode*> types;
      for (size_t i = 0; i < s.rhs.size(); ++i) {
        std::string t = "__t" + std::to_string(temp_id_++);
        out_ << Indent() << "auto " << t << " = " << EmitExpr(*s.rhs[i]) << ";\n";
        temps.push_back(t);
        types.push_back(InferType(s.rhs[i].get()));
      }
      for (size_t i = 0; i < s.names.size(); ++i) {
        if (s.names[i] == "_") continue;
        if (DeclaredInCurrentScope(s.names[i])) {
          out_ << Indent() << CppIdent(s.names[i]) << " = std::move(" << temps[i] << ");\n";
          continue;
        }
        if (types[i]) {
          out_ << Indent() << CppType(types[i]) << " " << CppIdent(s.names[i]) << " = std::move("
               << temps[i] << ");\n";
        } else {
          out_ << Indent() << "auto " << CppIdent(s.names[i]) << " = std::move(" << temps[i]
               << ");\n";
        }
        Declare(s.names[i], types[i]);
      }
      return;
    }
    if (s.names.size() > 1 && s.rhs.size() == 1) {
      EmitMultiUnpackDecl(s.names, *s.rhs[0]);
      return;
    }
    Error("unsupported ':=' shape");
  }

  void EmitTypeAssertOkDecl(const std::vector<std::string>& names, const Expr& rhs) {
    auto xt = InferType(rhs.x.get());
    if (!IsInterfaceType(xt)) Error("type assertion requires an interface value");
    std::string tmp = "__ta" + std::to_string(temp_id_++);
    out_ << Indent() << "auto " << tmp << " = (" << EmitExpr(*rhs.x) << ").try_cast<"
         << CppType(rhs.type.get()) << ">();\n";
    if (names[0] != "_") {
      if (DeclaredInCurrentScope(names[0])) {
        out_ << Indent() << CppIdent(names[0]) << " = " << tmp << ".first;\n";
      } else {
        out_ << Indent() << CppType(rhs.type.get()) << " " << CppIdent(names[0]) << " = " << tmp
             << ".first;\n";
        Declare(names[0], rhs.type.get());
      }
    }
    if (names.size() > 1 && names[1] != "_") {
      if (DeclaredInCurrentScope(names[1])) {
        out_ << Indent() << CppIdent(names[1]) << " = " << tmp << ".second;\n";
      } else {
        out_ << Indent() << "bool " << CppIdent(names[1]) << " = " << tmp << ".second;\n";
        Declare(names[1], SynthNamed("bool"));
      }
    }
  }

  void EmitTypeAssertOkAssign(const std::vector<std::unique_ptr<Expr>>& lhs, const Expr& rhs) {
    auto xt = InferType(rhs.x.get());
    if (!IsInterfaceType(xt)) Error("type assertion requires an interface value");
    std::string tmp = "__ta" + std::to_string(temp_id_++);
    out_ << Indent() << "auto " << tmp << " = (" << EmitExpr(*rhs.x) << ").try_cast<"
         << CppType(rhs.type.get()) << ">();\n";
    if (!IsBlank(*lhs[0])) {
      out_ << Indent() << EmitExpr(*lhs[0]) << " = " << tmp << ".first;\n";
    }
    if (lhs.size() > 1 && !IsBlank(*lhs[1])) {
      out_ << Indent() << EmitExpr(*lhs[1]) << " = " << tmp << ".second;\n";
    }
  }

  void EmitAssign(const Stmt& s) {
    if (s.lhs.size() == 1 && s.rhs.size() == 1 && IsBlank(*s.lhs[0])) {
      out_ << Indent() << "(void)" << EmitExpr(*s.rhs[0]) << ";\n";
      return;
    }
    if (s.lhs.size() == 1 && s.rhs.size() == 1) {
      auto lhs_type = InferType(s.lhs[0].get());
      if (s.op == "&^=") {
        std::string lhs = EmitExpr(*s.lhs[0]);
        out_ << Indent() << lhs << " = (" << lhs << " & ~" << EmitExpr(*s.rhs[0]) << ");\n";
        return;
      }
      std::string rhs_str = s.op == "=" ? EmitExprAs(*s.rhs[0], lhs_type) : EmitExpr(*s.rhs[0]);
      out_ << Indent() << EmitExpr(*s.lhs[0]) << " " << s.op << " " << rhs_str << ";\n";
      return;
    }
    if (s.op != "=") Error("a compound assignment ('+=', ...) needs exactly one left- and right-hand side");
    if (s.lhs.size() == s.rhs.size()) {
      std::vector<std::string> temps;
      for (size_t i = 0; i < s.rhs.size(); ++i) {
        std::string t = "__t" + std::to_string(temp_id_++);
        out_ << Indent() << "auto " << t << " = " << EmitExpr(*s.rhs[i]) << ";\n";
        temps.push_back(t);
      }
      for (size_t i = 0; i < s.lhs.size(); ++i) {
        if (IsBlank(*s.lhs[i])) continue;
        out_ << Indent() << EmitExpr(*s.lhs[i]) << " = std::move(" << temps[i] << ");\n";
      }
      return;
    }
    if (s.lhs.size() > 1 && s.rhs.size() == 1) {
      EmitMultiUnpackAssign(s.lhs, *s.rhs[0]);
      return;
    }
    Error("unsupported '=' shape");
  }

  // `out_pkg`, when given, receives the package the resolved FuncDecl needs
  // qualifying against (see UnpackCallResults's QualifyResultType call --
  // same "a cross-package result type needs pkg:: attached" rule as
  // ResultTypeOfCall/EmitArgsFor use for single-return calls and
  // parameters, just not wired through here until now).
  const FuncDecl* ResolveCalledFunc(const Expr& call, std::string* out_pkg = nullptr) {
    if (call.callee->kind == ExprKind::Ident) return LookupFreeFunc(call.callee->strval);
    if (call.callee->kind == ExprKind::Selector) {
      auto* sel = call.callee.get();
      // IsImportedPackage excludes "os"/"gocvm" (builtins, not a loaded
      // stdlib/*.go -- see BuildOsBuiltinFile/BuildGocvmBuiltinFile), but
      // os.Open/Create/ReadFile/WriteFile and gocvm.Call still need their
      // synthetic FuncDecl found here so `f, err := os.Open(...)` / `s,
      // err := gocvm.Call(...)` unpack through the normal path.
      if (sel->x->kind == ExprKind::Ident &&
          (IsImportedPackage(sel->x->strval) || PkgOf(sel->x->strval) == "os" ||
           PkgOf(sel->x->strval) == "gocvm")) {
        const std::string pkg = PkgOf(sel->x->strval);
        if (out_pkg) *out_pkg = pkg;
        return LookupFreeFunc(sel->strval, pkg);
      }
      auto baseType = InferType(sel->x.get());
      const TypeNode* st = baseType && baseType->kind == TypeKind::Pointer ? baseType->elem.get() : baseType;
      if (st && st->kind == TypeKind::Named) {
        if (out_pkg) *out_pkg = st->pkg;
        return LookupMethod(st->name, sel->strval, st->pkg);
      }
    }
    return nullptr;
  }

  const MethodSig* ResolveCalledIface(const Expr& call) {
    if (call.kind != ExprKind::Call || !call.callee || call.callee->kind != ExprKind::Selector) {
      return nullptr;
    }
    auto* sel = call.callee.get();
    auto baseType = InferType(sel->x.get());
    const TypeNode* st = baseType && baseType->kind == TypeKind::Pointer ? baseType->elem.get() : baseType;
    if (!st || st->kind != TypeKind::Named) return nullptr;
    return LookupIfaceMethod(st->name, sel->strval, st->pkg);
  }

  void UnpackCallResults(const std::vector<std::string>& names,
                         const std::vector<std::unique_ptr<TypeNode>>& results, const Expr& rhs,
                         const std::string& pkg = "") {
    if (results.size() != names.size()) {
      Error("assignment mismatch: this call doesn't return " + std::to_string(names.size()) + " value(s)");
    }
    std::string tmp = "__t" + std::to_string(temp_id_++);
    out_ << Indent() << "auto " << tmp << " = " << EmitExpr(rhs) << ";\n";
    for (size_t i = 0; i < names.size(); ++i) {
      if (names[i] == "_") continue;
      const TypeNode* rt = QualifyResultType(results[i].get(), pkg);
      if (DeclaredInCurrentScope(names[i])) {
        out_ << Indent() << CppIdent(names[i]) << " = " << tmp << ".r" << i << ";\n";
        continue;
      }
      if (rt) {
        out_ << Indent() << CppType(rt) << " " << CppIdent(names[i]) << " = " << tmp << ".r" << i
             << ";\n";
      } else {
        out_ << Indent() << "auto " << CppIdent(names[i]) << " = " << tmp << ".r" << i << ";\n";
      }
      Declare(names[i], rt);
    }
  }

  void EmitMultiUnpackDecl(const std::vector<std::string>& names, const Expr& rhs) {
    if (rhs.kind == ExprKind::Call) {
      std::string pkg;
      if (const FuncDecl* fn = ResolveCalledFunc(rhs, &pkg)) {
        UnpackCallResults(names, fn->results, rhs, pkg);
        return;
      }
      if (const MethodSig* ms = ResolveCalledIface(rhs)) {
        auto* sel = rhs.callee.get();
        auto baseType = InferType(sel->x.get());
        const TypeNode* st = baseType && baseType->kind == TypeKind::Pointer ? baseType->elem.get() : baseType;
        UnpackCallResults(names, ms->results, rhs, st ? st->pkg : "");
        return;
      }
      Error("assignment mismatch: this call doesn't return " + std::to_string(names.size()) + " value(s)");
    }
    if (rhs.kind == ExprKind::Index && names.size() == 2) {
      EmitCommaOkDecl(names, rhs);
      return;
    }
    if (rhs.kind == ExprKind::Recv && names.size() == 2) {
      EmitRecvOkDecl(names, rhs);
      return;
    }
    if (rhs.kind == ExprKind::TypeAssert && names.size() == 2) {
      EmitTypeAssertOkDecl(names, rhs);
      return;
    }
    Error("unsupported multi-value ':=' shape (expected a multi-return call, type assert, or a map's comma-ok index)");
  }

  void EmitMultiUnpackAssign(const std::vector<std::unique_ptr<Expr>>& lhs, const Expr& rhs) {
    if (rhs.kind == ExprKind::Call) {
      size_t nres = 0;
      if (const FuncDecl* fn = ResolveCalledFunc(rhs)) nres = fn->results.size();
      else if (const MethodSig* ms = ResolveCalledIface(rhs)) nres = ms->results.size();
      if (nres != lhs.size()) {
        Error("assignment mismatch: this call doesn't return " + std::to_string(lhs.size()) + " value(s)");
      }
      std::string tmp = "__t" + std::to_string(temp_id_++);
      out_ << Indent() << "auto " << tmp << " = " << EmitExpr(rhs) << ";\n";
      for (size_t i = 0; i < lhs.size(); ++i) {
        if (IsBlank(*lhs[i])) continue;
        out_ << Indent() << EmitExpr(*lhs[i]) << " = " << tmp << ".r" << i << ";\n";
      }
      return;
    }
    if (rhs.kind == ExprKind::Index && lhs.size() == 2) {
      EmitCommaOkAssign(lhs, rhs);
      return;
    }
    if (rhs.kind == ExprKind::Recv && lhs.size() == 2) {
      std::string tmp = "__t" + std::to_string(temp_id_++);
      out_ << Indent() << "auto " << tmp << " = " << AwaitPrefix() << "(" << EmitExpr(*rhs.x)
           << ").recv_ok();\n";
      if (!IsBlank(*lhs[0])) out_ << Indent() << EmitExpr(*lhs[0]) << " = std::move(" << tmp << ".first);\n";
      if (!IsBlank(*lhs[1])) out_ << Indent() << EmitExpr(*lhs[1]) << " = " << tmp << ".second;\n";
      return;
    }
    if (rhs.kind == ExprKind::TypeAssert && lhs.size() == 2) {
      EmitTypeAssertOkAssign(lhs, rhs);
      return;
    }
    Error("unsupported multi-value '=' shape (expected a multi-return call, type assert, or a map's comma-ok index)");
  }

  void EmitCommaOkDecl(const std::vector<std::string>& names, const Expr& rhs) {
    auto baseType = InferType(rhs.x.get());
    if (!baseType || baseType->kind != TypeKind::Map) Error("this comma-ok form requires a map index");
    std::string it = "__it" + std::to_string(temp_id_++);
    std::string map_str = EmitExpr(*rhs.x);
    out_ << Indent() << "auto " << it << " = (" << map_str << ").lookup(" << EmitExpr(*rhs.y) << ");\n";
    if (names[0] != "_") {
      if (DeclaredInCurrentScope(names[0])) {
        out_ << Indent() << names[0] << " = " << it << ".first;\n";
      } else {
        out_ << Indent() << "auto " << names[0] << " = " << it << ".first;\n";
        Declare(names[0], baseType->elem.get());
      }
    }
    if (names[1] != "_") {
      if (DeclaredInCurrentScope(names[1])) {
        out_ << Indent() << names[1] << " = " << it << ".second;\n";
      } else {
        out_ << Indent() << "auto " << names[1] << " = " << it << ".second;\n";
        Declare(names[1], SynthNamed("bool"));
      }
    }
  }

  void EmitCommaOkAssign(const std::vector<std::unique_ptr<Expr>>& lhs, const Expr& rhs) {
    auto baseType = InferType(rhs.x.get());
    if (!baseType || baseType->kind != TypeKind::Map) Error("this comma-ok form requires a map index");
    std::string it = "__it" + std::to_string(temp_id_++);
    std::string map_str = EmitExpr(*rhs.x);
    out_ << Indent() << "auto " << it << " = (" << map_str << ").lookup(" << EmitExpr(*rhs.y) << ");\n";
    if (!IsBlank(*lhs[0])) {
      out_ << Indent() << EmitExpr(*lhs[0]) << " = " << it << ".first;\n";
    }
    if (!IsBlank(*lhs[1])) {
      out_ << Indent() << EmitExpr(*lhs[1]) << " = " << it << ".second;\n";
    }
  }

  void EmitRecvOkDecl(const std::vector<std::string>& names, const Expr& rhs) {
    std::string tmp = "__t" + std::to_string(temp_id_++);
    out_ << Indent() << "auto " << tmp << " = " << AwaitPrefix() << "(" << EmitExpr(*rhs.x)
         << ").recv_ok();\n";
    auto ct = InferType(rhs.x.get());
    const TypeNode* elem = (ct && ct->kind == TypeKind::Chan) ? ct->elem.get() : nullptr;
    if (names[0] != "_") {
      if (DeclaredInCurrentScope(names[0])) {
        out_ << Indent() << names[0] << " = std::move(" << tmp << ".first);\n";
      } else {
        out_ << Indent() << "auto " << names[0] << " = std::move(" << tmp << ".first);\n";
        Declare(names[0], elem);
      }
    }
    if (names.size() > 1 && names[1] != "_") {
      if (DeclaredInCurrentScope(names[1])) {
        out_ << Indent() << names[1] << " = " << tmp << ".second;\n";
      } else {
        out_ << Indent() << "auto " << names[1] << " = " << tmp << ".second;\n";
        Declare(names[1], SynthNamed("bool"));
      }
    }
  }

  // `return` inside a range-over-func's loop body compiles to code running
  // INSIDE the yield lambda passed to the sequence function -- a literal
  // `return` there would return from that lambda (wrong type, wrong
  // effect), not the enclosing Go function. Real Go's own compiler
  // desugars this the same way: stash the value, stop iterating (`return
  // false` from the yield), and do the real return once control is back
  // at an ordinary statement, right after the sequence-function call
  // returns (see EmitRangeOverFunc's post-call check, and
  // JumpFrame::rf_ret_var/rf_val_var's own comment for how nesting shares
  // one flag/value pair all the way out).
  void EmitRangeFuncReturn(const Stmt& s, JumpFrame& rf) {
    // main() returns `int` at the actual C++ level despite declaring no
    // Go results (see EmitReturn's own IsMainFunc() special case) --
    // ReturnCppType alone would say "void" and lose the implicit 0.
    std::string rct = IsMainFunc() ? "int" : (current_func_ ? ReturnCppType(*current_func_) : "void");
    if (rct != "void") {
      std::string val_expr;
      if (s.rhs.empty()) {
        if (IsMainFunc()) {
          val_expr = "0";
        } else if (!in_func_lit_ && HasNamedResults(current_func_)) {
          if (current_func_->results.size() == 1) {
            val_expr = CppIdent(current_func_->result_names[0]);
          } else {
            std::ostringstream oss;
            oss << "{";
            for (size_t i = 0; i < current_func_->result_names.size(); ++i) {
              if (i) oss << ", ";
              const std::string& nm = current_func_->result_names[i];
              oss << (nm.empty() || nm == "_" ? "{}" : CppIdent(nm));
            }
            oss << "}";
            val_expr = oss.str();
          }
        } else {
          val_expr = "{}";
        }
      } else if (s.rhs.size() == 1) {
        const TypeNode* rt = CurrentResultCount() == 1 ? CurrentResultType(0) : nullptr;
        val_expr = EmitExprAs(*s.rhs[0], rt);
      } else {
        std::ostringstream oss;
        oss << "{";
        for (size_t i = 0; i < s.rhs.size(); ++i) {
          if (i) oss << ", ";
          const TypeNode* rt = i < CurrentResultCount() ? CurrentResultType(i) : nullptr;
          oss << EmitExprAs(*s.rhs[i], rt);
        }
        oss << "}";
        val_expr = oss.str();
      }
      out_ << Indent() << rf.rf_val_var << " = " << val_expr << ";\n";
    }
    out_ << Indent() << rf.rf_ret_var << " = true;\n";
    out_ << Indent() << "return false;\n";
  }

  void EmitReturn(const Stmt& s) {
    for (auto it = jump_stack_.rbegin(); it != jump_stack_.rend(); ++it) {
      if (it->range_func) {
        EmitRangeFuncReturn(s, *it);
        return;
      }
    }
    const char* kw = current_async_ ? "co_return" : "return";
    if (s.rhs.empty()) {
      if (IsMainFunc() && !current_async_) {
        out_ << Indent() << "return 0;\n";
      } else if (!in_func_lit_ && HasNamedResults(current_func_)) {
        if (current_func_->results.size() == 1) {
          out_ << Indent() << kw << " " << CppIdent(current_func_->result_names[0]) << ";\n";
        } else {
          out_ << Indent() << kw << " {";
          for (size_t i = 0; i < current_func_->result_names.size(); ++i) {
            if (i) out_ << ", ";
            const std::string& nm = current_func_->result_names[i];
            out_ << (nm.empty() || nm == "_" ? "{}" : CppIdent(nm));
          }
          out_ << "};\n";
        }
      } else if (current_async_ && CurrentResultCount() > 0) {
        out_ << Indent() << "co_return {};\n";
      } else {
        out_ << Indent() << kw << ";\n";
      }
      return;
    }
    if (s.rhs.size() == 1) {
      if (!in_func_lit_ && current_func_ && current_func_->results.size() > 1 && s.rhs[0]->kind == ExprKind::Call) {
        // `return otherFunc(...)`, forwarding another multi-return call's
        // result directly: otherFunc has its *own* uniquely-named result
        // struct (every multi-return function gets one -- see
        // ResultStructName), so even when the field types line up exactly
        // with this function's own results, the two struct types are
        // unrelated as far as C++ is concerned and there's no implicit
        // conversion between them. Decompose into the callee's fields and
        // reconstruct this function's own result struct from those,
        // instead of passing the callee's struct value through directly.
        std::string tmp = "__rt" + std::to_string(temp_id_++);
        out_ << Indent() << "auto " << tmp << " = " << EmitExpr(*s.rhs[0]) << ";\n";
        out_ << Indent() << kw << " {";
        for (size_t i = 0; i < current_func_->results.size(); ++i) {
          if (i) out_ << ", ";
          out_ << tmp << ".r" << i;
        }
        out_ << "};\n";
        return;
      }
      const TypeNode* rt = CurrentResultCount() == 1 ? CurrentResultType(0) : nullptr;
      out_ << Indent() << kw << " " << EmitExprAs(*s.rhs[0], rt) << ";\n";
      return;
    }
    out_ << Indent() << kw << " {";
    for (size_t i = 0; i < s.rhs.size(); ++i) {
      if (i) out_ << ", ";
      const TypeNode* rt = i < CurrentResultCount() ? CurrentResultType(i) : nullptr;
      out_ << EmitExprAs(*s.rhs[i], rt);
    }
    out_ << "};\n";
  }

  // A plain `if` needs no extra C++ scope. `if init; cond { ... }` does --
  // Go scopes `init`'s variables to the whole if/else statement, not to the
  // enclosing block, so an extra `{ }` wrapper reproduces that (otherwise
  // they'd leak into the enclosing C++ scope and could collide with a later
  // declaration Go would have allowed).
  void EmitIf(const Stmt& s) {
    bool wrap = s.init != nullptr;
    if (wrap) {
      out_ << Indent() << "{\n";
      indent_++;
    }
    PushScope();
    if (s.init) EmitStmt(*s.init);
    out_ << Indent() << "if (" << EmitExpr(*s.cond) << ") {\n";
    indent_++;
    EmitStmtList(s.body);
    indent_--;
    out_ << Indent() << "}\n";
    if (s.has_else) {
      out_ << Indent() << "else {\n";
      indent_++;
      EmitStmtList(s.else_body);
      indent_--;
      out_ << Indent() << "}\n";
    }
    PopScope();
    if (wrap) {
      indent_--;
      out_ << Indent() << "}\n";
    }
  }

  std::string EmitSimpleStmtInline(const Stmt& s) {
    switch (s.kind) {
      case StmtKind::ShortVarDecl:
        if (s.names.size() == 1 && s.rhs.size() == 1) {
          std::string rhs_str = EmitExpr(*s.rhs[0]);
          Declare(s.names[0], InferType(s.rhs[0].get()));
          return "auto " + s.names[0] + " = " + rhs_str;
        }
        Error("a for-loop's init/post clause supports only a single simple ':=' declaration");
      case StmtKind::Assign:
        if (s.lhs.size() == 1 && s.rhs.size() == 1) {
          return EmitExpr(*s.lhs[0]) + " " + s.op + " " + EmitExpr(*s.rhs[0]);
        }
        Error("a for-loop's init/post clause supports only a single simple assignment");
      case StmtKind::IncDec:
        return EmitExpr(*s.lhs[0]) + s.op;
      case StmtKind::ExprStmt:
        return EmitExpr(*s.lhs[0]);
      default:
        Error("unsupported statement in a for-loop's init/post clause");
    }
  }

  JumpFrame BeginLoop() {
    JumpFrame j;
    j.is_loop = true;
    if (!pending_label_.empty()) {
      j.name = pending_label_;
      std::string id = CppIdent(pending_label_);
      j.brk = "__brk_" + id;
      j.cont = "__cnt_" + id;
      pending_label_.clear();
    }
    jump_stack_.push_back(j);
    return j;
  }
  void EndLoop(const JumpFrame& j) {
    jump_stack_.pop_back();
    if (!j.brk.empty()) out_ << Indent() << j.brk << ": ;\n";
  }
  void EmitContLabel() {
    if (!jump_stack_.empty() && !jump_stack_.back().cont.empty()) {
      out_ << Indent() << jump_stack_.back().cont << ": ;\n";
    }
  }

  void EmitLabeled(const Stmt& s) {
    if (s.names.empty() || s.body.empty() || !s.body[0]) Error("empty labeled statement");
    out_ << Indent() << CppIdent(s.names[0]) << ": ;\n";
    pending_label_ = s.names[0];
    EmitStmt(*s.body[0]);
    if (!pending_label_.empty()) pending_label_.clear();
  }

  void EmitBreak(const Stmt& s) {
    if (!s.names.empty()) {
      for (auto it = jump_stack_.rbegin(); it != jump_stack_.rend(); ++it) {
        if (it->name == s.names[0] && !it->brk.empty()) {
          if (it->range_func) {
            out_ << Indent() << "return false;\n";
            return;
          }
          out_ << Indent() << "goto " << it->brk << ";\n";
          return;
        }
      }
      Error("break label '" + s.names[0] + "' is not a surrounding loop or switch");
    }
    if (!jump_stack_.empty() && jump_stack_.back().range_func) {
      out_ << Indent() << "return false;\n";
      return;
    }
    if (!jump_stack_.empty() && !jump_stack_.back().brk.empty()) {
      out_ << Indent() << "goto " << jump_stack_.back().brk << ";\n";
    } else {
      out_ << Indent() << "break;\n";
    }
  }

  void EmitContinue(const Stmt& s) {
    if (!s.names.empty()) {
      for (auto it = jump_stack_.rbegin(); it != jump_stack_.rend(); ++it) {
        if (it->is_loop && it->name == s.names[0]) {
          if (it->range_func) {
            out_ << Indent() << "return true;\n";
            return;
          }
          if (!it->cont.empty()) {
            out_ << Indent() << "goto " << it->cont << ";\n";
            return;
          }
        }
      }
      Error("continue label '" + s.names[0] + "' is not a surrounding loop");
    }
    for (auto it = jump_stack_.rbegin(); it != jump_stack_.rend(); ++it) {
      if (!it->is_loop) continue;
      if (it->range_func) {
        out_ << Indent() << "return true;\n";
        return;
      }
      if (!it->cont.empty()) {
        out_ << Indent() << "goto " << it->cont << ";\n";
        return;
      }
      out_ << Indent() << "continue;\n";
      return;
    }
    Error("continue is not in a loop");
  }

  void EmitForCond(const Stmt& s) {
    auto j = BeginLoop();
    out_ << Indent() << "while (" << EmitExpr(*s.cond) << ") {\n";
    indent_++;
    EmitStmtList(s.body);
    EmitContLabel();
    indent_--;
    out_ << Indent() << "}\n";
    EndLoop(j);
  }

  void EmitForInfinite(const Stmt& s) {
    auto j = BeginLoop();
    out_ << Indent() << "while (true) {\n";
    indent_++;
    EmitStmtList(s.body);
    EmitContLabel();
    indent_--;
    out_ << Indent() << "}\n";
    EndLoop(j);
  }

  void EmitForClassic(const Stmt& s) {
    auto j = BeginLoop();
    PushScope();
    if (!j.cont.empty()) {
      out_ << Indent() << "for (";
      if (s.init) out_ << EmitSimpleStmtInline(*s.init);
      out_ << "; ";
      if (s.cond) out_ << EmitExpr(*s.cond);
      out_ << "; ) {\n";
      indent_++;
      EmitStmtList(s.body);
      EmitContLabel();
      if (s.post) out_ << Indent() << EmitSimpleStmtInline(*s.post) << ";\n";
      indent_--;
      out_ << Indent() << "}\n";
    } else {
      out_ << Indent() << "for (";
      if (s.init) out_ << EmitSimpleStmtInline(*s.init);
      out_ << "; ";
      if (s.cond) out_ << EmitExpr(*s.cond);
      out_ << "; ";
      if (s.post) out_ << EmitSimpleStmtInline(*s.post);
      out_ << ") {\n";
      indent_++;
      EmitStmtList(s.body);
      indent_--;
      out_ << Indent() << "}\n";
    }
    PopScope();
    EndLoop(j);
  }

  void EmitRangeOverFunc(const Stmt& s, const TypeNode* ft, const std::string& range_src) {
    if (!ft || ft->func_params.size() != 1 || !ft->func_params[0].type ||
        ft->func_params[0].type->kind != TypeKind::Func) {
      Error("'range' over a func requires func(yield func(...) bool)");
    }
    const TypeNode* yield = ft->func_params[0].type.get();
    if (yield->func_results.size() != 1 || !yield->func_results[0] ||
        yield->func_results[0]->kind != TypeKind::Named || yield->func_results[0]->name != "bool") {
      Error("'range' over a func requires the yield callback to return bool");
    }
    const auto& yparams = yield->func_params;
    if (yparams.size() != 1 && yparams.size() != 2) {
      Error("range-over-func yield must take 1 or 2 arguments");
    }
    if (yparams.size() == 1 && s.range_has_value) {
      Error("range over seq yields one value");
    }
    jump_stack_.back().range_func = true;
    // A `return` inside this loop's body needs somewhere to stash its
    // value and a flag to stop iterating -- see EmitRangeFuncReturn's own
    // comment. Nested inside another range-over-func, reuse ITS pair (one
    // shared escape path, `return false` propagates outward one level at
    // a time); otherwise this is the outermost one reachable, so declare
    // a fresh pair right before the call. Copied into locals (not kept as
    // a reference/pointer into jump_stack_) because EmitStmtList(s.body)
    // below can push/pop that vector for nested loops, which may
    // reallocate it.
    bool is_nested_rf = false;
    for (auto it = jump_stack_.rbegin() + 1; it != jump_stack_.rend(); ++it) {
      if (it->range_func) {
        is_nested_rf = true;
        jump_stack_.back().rf_ret_var = it->rf_ret_var;
        jump_stack_.back().rf_val_var = it->rf_val_var;
        break;
      }
    }
    std::string rct = IsMainFunc() ? "int" : (current_func_ ? ReturnCppType(*current_func_) : "void");
    if (!is_nested_rf) {
      std::string id = std::to_string(temp_id_++);
      jump_stack_.back().rf_ret_var = "__rf_ret" + id;
      out_ << Indent() << "bool " << jump_stack_.back().rf_ret_var << " = false;\n";
      if (rct != "void") {
        jump_stack_.back().rf_val_var = "__rf_val" + id;
        out_ << Indent() << rct << " " << jump_stack_.back().rf_val_var << "{};\n";
      }
    }
    std::string rf_ret_var = jump_stack_.back().rf_ret_var;
    std::string rf_val_var = jump_stack_.back().rf_val_var;
    out_ << Indent() << range_src << "(" << CppType(yield) << "{[&](";
    for (size_t i = 0; i < yparams.size(); ++i) {
      if (i) out_ << ", ";
      out_ << CppType(yparams[i].type.get()) << " __y" << i;
    }
    out_ << ") -> bool {\n";
    indent_++;
    if (yparams.size() == 1) {
      if (s.range_has_key && s.names[0] != "_") {
        out_ << Indent() << CppType(yparams[0].type.get()) << " " << CppIdent(s.names[0])
             << " = __y0;\n";
        Declare(s.names[0], yparams[0].type.get());
      }
    } else {
      if (s.range_has_key && s.names[0] != "_") {
        out_ << Indent() << CppType(yparams[0].type.get()) << " " << CppIdent(s.names[0])
             << " = __y0;\n";
        Declare(s.names[0], yparams[0].type.get());
      }
      if (s.range_has_value && s.names.size() > 1 && s.names[1] != "_") {
        out_ << Indent() << CppType(yparams[1].type.get()) << " " << CppIdent(s.names[1])
             << " = __y1;\n";
        Declare(s.names[1], yparams[1].type.get());
      }
    }
    EmitStmtList(s.body);
    out_ << Indent() << "return true;\n";
    indent_--;
    out_ << Indent() << "}});\n";
    // Escape check: a `return` inside the loop stopped iteration and set
    // rf_ret_var rather than actually returning (see EmitRangeFuncReturn).
    // If this loop is itself nested inside another range-over-func, we're
    // still lexically inside THAT one's yield lambda right here, so
    // `return false` correctly stops it too, propagating the same check
    // one level further out; otherwise this is the real function scope,
    // so do the actual C++ return here.
    if (!rf_ret_var.empty()) {
      out_ << Indent() << "if (" << rf_ret_var << ") {\n";
      indent_++;
      if (is_nested_rf) {
        out_ << Indent() << "return false;\n";
      } else {
        const char* kw = current_async_ ? "co_return" : "return";
        if (rct == "void") {
          out_ << Indent() << kw << ";\n";
        } else {
          out_ << Indent() << kw << " " << rf_val_var << ";\n";
        }
      }
      indent_--;
      out_ << Indent() << "}\n";
    }
  }

  void EmitForRange(const Stmt& s) {
    auto exprType = InferType(s.range_expr.get());
    auto baseType = ResolveUnderlying(exprType);
    if (!baseType) Error("cannot resolve the type of a 'range' expression");
    std::string range_src = EmitExpr(*s.range_expr);
    // A named type wrapping []T/[N]T/map[K]V *with at least one method*
    // (EmitAliases' wrapper-struct path, gated by HasMethodsOn exactly
    // like EmitIndex's matching fix -- see its longer comment for why
    // that gate matters and why `.v` beats casting through the
    // conversion operator) only exposes an implicit conversion to the
    // underlying Slice<T>/array/Map<K,V> -- ranging needs
    // .size()/operator[]/begin()/end() directly on range_src below,
    // none of which are found through that conversion via ordinary
    // lookup. Range over the wrapper's own `v` member directly instead.
    if (exprType && exprType->kind == TypeKind::Named &&
        (baseType->kind == TypeKind::Slice || baseType->kind == TypeKind::Array ||
         baseType->kind == TypeKind::Map) &&
        HasMethodsOn(exprType->name, exprType->pkg)) {
      range_src = "(" + range_src + ").v";
    }
    if (baseType->kind == TypeKind::Pointer) {
      auto elem = ResolveUnderlying(baseType->elem.get());
      if (elem && elem->kind == TypeKind::Array) {
        baseType = elem;
        range_src = "(*(" + range_src + "))";
      }
    }
    auto j = BeginLoop();
    PushScope();
    if (baseType->kind == TypeKind::Slice) {
      std::string rv = "__r" + std::to_string(temp_id_++);
      std::string iv = "__i" + std::to_string(temp_id_++);
      out_ << Indent() << "auto&& " << rv << " = " << range_src << ";\n";
      out_ << Indent() << "for (size_t " << iv << " = 0; " << iv << " < " << rv << ".size(); ++" << iv << ") {\n";
      indent_++;
      if (s.range_has_key) {
        out_ << Indent() << CppType(SynthNamed("int")) << " " << s.names[0] << " = static_cast<int64_t>(" << iv << ");\n";
        Declare(s.names[0], SynthNamed("int"));
      }
      if (s.range_has_value) {
        // range_has_value is only ever true when two names were written
        // (see ParseForStmt), so the value name is always at index 1 --
        // regardless of whether the key at index 0 is bound or "_".
        std::string val_name = s.names[1];
        out_ << Indent() << "auto& " << val_name << " = " << rv << "[" << iv << "];\n";
        Declare(val_name, baseType->elem.get());
      }
      EmitStmtList(s.body);
      EmitContLabel();
      indent_--;
      out_ << Indent() << "}\n";
    } else if (baseType->kind == TypeKind::Array) {
      std::string rv = "__r" + std::to_string(temp_id_++);
      std::string iv = "__i" + std::to_string(temp_id_++);
      out_ << Indent() << "auto&& " << rv << " = " << range_src << ";\n";
      out_ << Indent() << "for (size_t " << iv << " = 0; " << iv << " < " << rv << ".size(); ++" << iv
           << ") {\n";
      indent_++;
      if (s.range_has_key) {
        out_ << Indent() << CppType(SynthNamed("int")) << " " << CppIdent(s.names[0])
             << " = static_cast<int64_t>(" << iv << ");\n";
        Declare(s.names[0], SynthNamed("int"));
      }
      if (s.range_has_value) {
        std::string val_name = s.names[1];
        out_ << Indent() << "auto& " << CppIdent(val_name) << " = " << rv << "[" << iv << "];\n";
        Declare(val_name, baseType->elem.get());
      }
      EmitStmtList(s.body);
      EmitContLabel();
      indent_--;
      out_ << Indent() << "}\n";
    } else if (IsIntegerType(baseType)) {
      if (s.range_has_value) Error("range over an integer yields a single index");
      std::string n = "__n" + std::to_string(temp_id_++);
      std::string iv = "__i" + std::to_string(temp_id_++);
      out_ << Indent() << "int64_t " << n << " = " << range_src << ";\n";
      out_ << Indent() << "for (int64_t " << iv << " = 0; " << iv << " < " << n << "; ++" << iv
           << ") {\n";
      indent_++;
      if (s.range_has_key && s.names[0] != "_") {
        out_ << Indent() << CppType(SynthNamed("int")) << " " << CppIdent(s.names[0]) << " = " << iv
             << ";\n";
        Declare(s.names[0], SynthNamed("int"));
      }
      EmitStmtList(s.body);
      EmitContLabel();
      indent_--;
      out_ << Indent() << "}\n";
    } else if (baseType->kind == TypeKind::Map) {
      std::string pv = "__p" + std::to_string(temp_id_++);
      out_ << Indent() << "for (auto& " << pv << " : " << range_src << ") {\n";
      indent_++;
      if (s.range_has_key) {
        out_ << Indent() << "auto& " << s.names[0] << " = " << pv << ".first;\n";
        Declare(s.names[0], baseType->key.get());
      }
      if (s.range_has_value) {
        // range_has_value is only ever true when two names were written
        // (see ParseForStmt), so the value name is always at index 1 --
        // regardless of whether the key at index 0 is bound or "_".
        std::string val_name = s.names[1];
        out_ << Indent() << "auto& " << val_name << " = " << pv << ".second;\n";
        Declare(val_name, baseType->elem.get());
      }
      EmitStmtList(s.body);
      EmitContLabel();
      indent_--;
      out_ << Indent() << "}\n";
    } else if (baseType->kind == TypeKind::Named && baseType->name == "string") {
      std::string rv = "__r" + std::to_string(temp_id_++);
      std::string iv = "__i" + std::to_string(temp_id_++);
      std::string dv = "__d" + std::to_string(temp_id_++);
      out_ << Indent() << "auto&& " << rv << " = " << EmitExpr(*s.range_expr) << ";\n";
      out_ << Indent() << "for (int64_t " << iv << " = 0; " << iv << " < wasigo::len(" << rv
           << "); ) {\n";
      indent_++;
      out_ << Indent() << "auto " << dv << " = wasigo::decode_rune(" << rv << ", " << iv << ");\n";
      out_ << Indent() << "if (" << dv << ".size <= 0) break;\n";
      if (s.range_has_key) {
        out_ << Indent() << "int64_t " << CppIdent(s.names[0]) << " = " << iv << ";\n";
        Declare(s.names[0], SynthNamed("int"));
      }
      if (s.range_has_value) {
        out_ << Indent() << "int32_t " << CppIdent(s.names[1]) << " = " << dv << ".r;\n";
        Declare(s.names[1], SynthNamed("rune"));
      }
      out_ << Indent() << iv << " += " << dv << ".size;\n";
      EmitStmtList(s.body);
      EmitContLabel();
      indent_--;
      out_ << Indent() << "}\n";
    } else if (baseType->kind == TypeKind::Chan) {
      if (!current_async_) {
        Error("range over a channel is only valid in a function that uses goroutines/channels");
      }
      std::string cv = "__c" + std::to_string(temp_id_++);
      std::string tv = "__t" + std::to_string(temp_id_++);
      out_ << Indent() << "auto " << cv << " = " << EmitExpr(*s.range_expr) << ";\n";
      out_ << Indent() << "for (;;) {\n";
      indent_++;
      out_ << Indent() << "auto " << tv << " = co_await (" << cv << ").recv_ok();\n";
      out_ << Indent() << "if (!" << tv << ".second) break;\n";
      if (s.names.size() == 1 && s.range_has_key) {
        // `for v := range ch` -- the single name is the element, not an index.
        out_ << Indent() << "auto " << CppIdent(s.names[0]) << " = std::move(" << tv << ".first);\n";
        Declare(s.names[0], baseType->elem.get());
      } else if (s.range_has_value) {
        out_ << Indent() << "auto " << CppIdent(s.names[1]) << " = std::move(" << tv << ".first);\n";
        Declare(s.names[1], baseType->elem.get());
      }
      EmitStmtList(s.body);
      EmitContLabel();
      indent_--;
      out_ << Indent() << "}\n";
    } else if (baseType->kind == TypeKind::Func) {
      EmitRangeOverFunc(s, baseType, range_src);
    } else {
      Error("'range' requires a slice, array, map, string, channel, integer, or iterator func");
    }
    PopScope();
    EndLoop(j);
  }

  // ---- top-level emission -----------------------------------------------------

  void EmitStructForwardDecls() {
    for (auto& sd : file_.structs) {
      if (!sd.type_params.empty()) out_ << TemplatePrefixFrom(sd.type_params);
      out_ << "struct " << sd.name << ";\n";
    }
    for (auto& name : result_struct_names_) out_ << "struct " << name << ";\n";
    out_ << "\n";
  }

  std::string FreeFuncSignatureLine(const FuncDecl& fn) {
    std::ostringstream oss;
    bool async = IsAsyncFree(fn.name);
    oss << FuncCppType(fn, async) << " " << fn.name << "(";
    for (size_t i = 0; i < fn.params.size(); ++i) {
      if (i) oss << ", ";
      oss << ParamCppType(fn.params[i]) << " " << CppIdent(fn.params[i].name);
    }
    oss << ")";
    return oss.str();
  }

  void EmitFreeFuncPrototypes() {
    out_ << "void __wasigo_init();\n";
    for (auto& fn : file_.funcs) {
      if (fn.has_receiver || fn.name == "main" || IsInitFunc(fn)) continue;
      if (!fn.type_params.empty()) {
        // A generic free function still needs a forward declaration --
        // otherwise one generic calling another generic defined later in
        // the same file (e.g. slices.Contains calling slices.Index) fails
        // both ordinary and argument-dependent lookup at instantiation,
        // since a template call is still an ordinary (non-ADL-only) name
        // use when both live in the same namespace. TemplatePrefix/
        // FreeFuncSignatureLine need current_func_ set to resolve the
        // type parameters' bare names (T, K, ...) via NamedCppType.
        const FuncDecl* saved = current_func_;
        current_func_ = &fn;
        out_ << TemplatePrefix(fn) << FreeFuncSignatureLine(fn) << ";\n";
        current_func_ = saved;
        continue;
      }
      out_ << FreeFuncSignatureLine(fn) << ";\n";
    }
    out_ << "\n";
  }

  bool StmtsHaveDefer(const std::vector<std::unique_ptr<Stmt>>& stmts) const {
    for (auto& sp : stmts) {
      if (!sp) continue;
      if (sp->kind == StmtKind::Defer) return true;
      if (StmtsHaveDefer(sp->body) || StmtsHaveDefer(sp->else_body)) return true;
      for (auto& c : sp->cases) {
        if (StmtsHaveDefer(c.body)) return true;
      }
      for (auto& c : sp->sel_cases) {
        if (StmtsHaveDefer(c.body)) return true;
      }
    }
    return false;
  }

  void EmitNamedResultDecls() {
    if (!HasNamedResults(current_func_)) return;
    for (size_t i = 0; i < current_func_->result_names.size(); ++i) {
      const std::string& nm = current_func_->result_names[i];
      if (nm.empty() || nm == "_") continue;
      const TypeNode* t =
          i < current_func_->results.size() ? current_func_->results[i].get() : nullptr;
      out_ << Indent() << CppType(t) << " " << CppIdent(nm) << "{};\n";
      Declare(nm, t);
    }
  }

  void EmitStartupInits() {
    for (const File* f : opt_.imported_files) {
      if (!f || f->package_name.empty() || f->package_name == "main") continue;
      out_ << Indent() << QualName(f->package_name, "__wasigo_init") << "();\n";
    }
    out_ << Indent() << "__wasigo_init();\n";
  }

  void EmitWrappedBody(const std::vector<std::unique_ptr<Stmt>>& body, bool async, bool is_main) {
    bool wrap = StmtsHaveDefer(body);
    current_has_defers_ = wrap;
    // setjmp cannot land in a C++20 coroutine frame (the WASM mapping for
    // panic/recover). Defer still runs on the normal co_return path.
    if (is_main) EmitStartupInits();
    EmitNamedResultDecls();
    if (wrap) {
      if (!async) out_ << Indent() << "wasigo::PanicFrame __pf;\n";
      out_ << Indent() << "wasigo::DeferList __defers;\n";
    }
    for (auto& st : body) EmitStmt(*st);
    if (wrap && !async) out_ << Indent() << "__wasigo_end: ;\n";
    if (async) {
      bool last_returns = !body.empty() && body.back() && body.back()->kind == StmtKind::Return;
      if (!last_returns) {
        if (current_func_ && !current_func_->results.empty()) {
          out_ << Indent() << "co_return {};\n";
        } else {
          out_ << Indent() << "co_return;\n";
        }
      }
    } else if (is_main) {
      if (program_has_go_) out_ << Indent() << "wasigo::run();\n";
      out_ << Indent() << "return 0;\n";
    }
  }

  // Declaration only (inside the struct body) -- see EmitMethodOutOfLine's
  // comment for why the body is never emitted here.
  void EmitMethodDecl(const FuncDecl& fn) {
    bool async = IsAsyncMethod(fn.receiver_type, fn.name);
    out_ << "  " << FuncCppType(fn, async) << " " << fn.name << "(";
    for (size_t i = 0; i < fn.params.size(); ++i) {
      if (i) out_ << ", ";
      out_ << ParamCppType(fn.params[i]) << " " << CppIdent(fn.params[i].name);
    }
    out_ << ")";
    if (!fn.receiver_is_pointer) out_ << " const";
    out_ << ";\n";
  }

  // Every struct method's BODY is emitted out-of-line (`ReturnType
  // Recv::Method(...) const { ... }` at namespace scope), never inline
  // inside the struct body, and only after EVERY struct in the file has
  // already been given its field-only skeleton (EmitStructDefs, called
  // before this) -- so a method can reference ANY other struct in the
  // same file by value (a field access, a by-value parameter, a local
  // var) regardless of which one is declared textually first. Real Go
  // doesn't care about declaration order AT ALL (`type Point struct{...}`
  // with a method `func (p Point) In(r Rectangle) bool` reading `r.Min.X`
  // is completely ordinary even when `Rectangle` is declared LATER in the
  // same file -- this is literally how real Go's own image.go is
  // written), but C++ classes with inline method bodies only defer
  // parsing to right after THAT class's own closing brace ("complete-
  // class context"), never all the way to end-of-file -- so a method
  // inlined the old way could reference only types already complete by
  // ITS OWN struct's closing brace, silently working for most stdlib code
  // so far (which happens to declare dependencies first) until
  // `image.Point.In(Rectangle)` broke doing exactly the opposite. Same
  // fix shape as EmitInterfaceDefs's own out-of-line-body split just
  // above, and for the identical underlying reason -- this one didn't
  // need a deferred buffer, since it's simply called later in Run()'s own
  // sequence (right after EmitStructDefs), the same way EmitFreeFuncDefs
  // already runs separately from EmitFreeFuncPrototypes.
  // Elaborated-type-specifier ("struct Name" instead of bare "Name") for
  // a plain Named-struct type used directly as a parameter or return type
  // in an out-of-line member-function definition. At that point, ordinary
  // unqualified lookup ALSO sees every sibling member of the class being
  // defined (its declaration list, already fully emitted by
  // EmitStructDefs earlier) -- so a method whose OWN name matches ANOTHER
  // type's name (e.g. real Go's own `hash/maphash`: `Hash.Seed() Seed`
  // sitting alongside `Hash.SetSeed(seed Seed)`) shadows that type for
  // every OTHER out-of-line definition in the same struct: the compiler
  // resolves the bare "Seed" in `SetSeed`'s parameter list to the method
  // `Hash::Seed`, not the type, and errors ("non-standard syntax; use '&'
  // to create a pointer to member"). An elaborated-type-specifier
  // (`struct Seed`) always finds the TYPE regardless of what else that
  // bare name would otherwise resolve to -- the standard, general C++ fix
  // for exactly this "hidden by a non-type declaration" shape. Only
  // applied to a DIRECT Named-struct parameter/return type, not one
  // nested inside `Slice<T>`/`TaskT<T>`/a multi-result struct's own
  // fields -- narrower than fully general, but covers the shape that
  // actually broke building `hash/maphash` (an accessor method's name
  // matching another type is common real Go style: `func (c *Config)
  // Timeout() Timeout`-shaped APIs).
  std::string ElaboratedParamOrReturnType(const TypeNode* t) {
    std::string base = CppType(t);
    if (t && t->kind == TypeKind::Named && LookupStruct(t->name, t->pkg)) {
      return "struct " + base;
    }
    return base;
  }

  void EmitMethodOutOfLine(const FuncDecl& fn) {
    bool async = IsAsyncMethod(fn.receiver_type, fn.name);
    const StructDecl* recv_sd = LookupStruct(fn.receiver_type);
    std::string recv_type_name = recv_sd ? SelfTypeName(*recv_sd) : fn.receiver_type;
    std::string ret_type = (!async && fn.results.size() == 1)
                                ? ElaboratedParamOrReturnType(fn.results[0].get())
                                : FuncCppType(fn, async);
    out_ << TemplatePrefix(fn) << ret_type << " " << recv_type_name << "::" << fn.name << "(";
    for (size_t i = 0; i < fn.params.size(); ++i) {
      if (i) out_ << ", ";
      std::string pt = fn.params[i].variadic ? ParamCppType(fn.params[i])
                                              : ElaboratedParamOrReturnType(fn.params[i].type.get());
      out_ << pt << " " << CppIdent(fn.params[i].name);
    }
    out_ << ")";
    if (!fn.receiver_is_pointer) out_ << " const";
    out_ << " {\n";
    indent_ = 1;
    PushScope();
    bool saved_async = current_async_;
    current_async_ = async;
    if (fn.receiver_is_pointer) {
      out_ << Indent() << recv_type_name << "* " << fn.receiver_name << " = this;\n";
      Declare(fn.receiver_name, SynthPointer(SynthNamed(fn.receiver_type)));
    } else {
      out_ << Indent() << recv_type_name << " " << fn.receiver_name << " = *this;\n";
      Declare(fn.receiver_name, SynthNamed(fn.receiver_type));
    }
    for (auto& p : fn.params) Declare(p.name, ParamGoType(p));
    const FuncDecl* saved = current_func_;
    current_func_ = &fn;
    EmitWrappedBody(fn.body, async, false);
    current_func_ = saved;
    current_async_ = saved_async;
    PopScope();
    indent_ = 0;
    out_ << "}\n\n";
  }

  void EmitStructMethodDefs() {
    for (auto& sd : file_.structs) {
      for (auto& fn : file_.funcs) {
        if (fn.has_receiver && fn.receiver_type == sd.name) EmitMethodOutOfLine(fn);
      }
    }
  }

  void EmitAllMethodDefs() {
    EmitStructMethodDefs();
    for (auto& fn : file_.funcs) {
      if (!fn.has_receiver) continue;
      if (LookupStruct(fn.receiver_type)) continue;
      EmitMethodOutOfLine(fn);
    }
  }

  void EmitStructDefs() {
    auto saved_tp = current_type_params_;
    for (auto& sd : file_.structs) {
      current_type_params_ = sd.type_params;
      if (!sd.type_params.empty()) out_ << TemplatePrefixFrom(sd.type_params);
      out_ << "struct " << sd.name;
      bool first_base = true;
      for (auto& f : sd.fields) {
        if (!f.embedded) continue;
        if (f.type && f.type->kind == TypeKind::Named) {
          out_ << (first_base ? " : " : ", ") << "public " << f.type->name;
          first_base = false;
        }
      }
      out_ << " {\n";
      for (auto& f : sd.fields) {
        if (f.embedded && f.type && f.type->kind == TypeKind::Named) continue;
        out_ << "  " << CppType(f.type.get()) << " " << FieldCppName(sd.name, f.name) << "{};\n";
      }
      out_ << "\n";
      for (auto& fn : file_.funcs) {
        if (fn.has_receiver && fn.receiver_type == sd.name) EmitMethodDecl(fn);
      }
      bool comparable = true;
      for (auto& f : sd.fields) {
        const TypeNode* t = f.type.get();
        // A named-interface field (any included) has no operator== on the
        // generated adapter struct (self/vt/type_key -- see
        // EmitInterfaceDefs), same reason Slice/Map/Chan/Func aren't
        // comparable here either. Every earlier struct with an interface
        // field also happened to have a disqualifying Slice/Map field
        // first, so this gap stayed latent until a struct held ONLY an
        // interface field alongside plain comparable ones (see
        // encoding/csv.Writer).
        if (t && (t->kind == TypeKind::Slice || t->kind == TypeKind::Map || t->kind == TypeKind::Chan ||
                  t->kind == TypeKind::Func || IsInterfaceType(t))) {
          comparable = false;
          break;
        }
      }
      if (comparable) {
        std::string self_type = SelfTypeName(sd);
        out_ << "  bool operator==(const " << self_type << "& o) const {\n    return true";
        for (auto& f : sd.fields) {
          if (f.embedded && f.type && f.type->kind == TypeKind::Named) {
            out_ << " && static_cast<const " << f.type->name << "&>(*this) == o";
          } else if (!f.name.empty()) {
            std::string fn = FieldCppName(sd.name, f.name);
            out_ << " && " << fn << " == o." << fn;
          }
        }
        out_ << ";\n  }\n";
        out_ << "  bool operator!=(const " << self_type << "& o) const { return !(*this == o); }\n";
      }
      out_ << "};\n\n";
      EmitReflectDescribe(sd);
    }
    current_type_params_ = saved_tp;
  }

  // Per-struct reflection metadata (see runtime.hpp's has_reflect_describe/
  // has_reflect_typename traits, ADL-found from Any::adapt<T>): a free
  // function listing this struct's exported fields, by name, as their own
  // Any-boxed values, plus one returning a Go-spelled type name. Only
  // exported (capitalized) fields are described -- matches real Go's
  // encoding/json, which also only ever sees exported struct fields.
  // Embedded/anonymous fields are skipped (their promoted-field shape
  // isn't reflected through here).
  void EmitReflectDescribe(const StructDecl& sd) {
    std::string self_type = SelfTypeName(sd);
    // Generic struct (`type Pair[T any] struct {...}`): SelfTypeName
    // already yields the instantiated-looking form ("Pair<T>") used
    // inside the struct's own template body, so both free functions
    // below just need to become templates over the SAME type
    // parameters -- has_reflect_describe/has_reflect_typename
    // (runtime.hpp) find them via ADL + template argument deduction
    // exactly like any other ADL-found overload; nothing there needs
    // to change. This used to just `return` here instead, silently
    // leaving Pair[int]{...} (or any other generic struct) with no
    // reflection metadata at all -- reflect.TypeOf/.NumField/.Field
    // would see it as an opaque non-struct rather than erroring
    // loudly, since has_reflect_describe<T> is a compile-time trait: a
    // missing overload just makes the reflect.Value branch that checks
    // it silently take the "not a struct" path.
    std::string tmpl_prefix = TemplatePrefixFrom(sd.type_params);
    // "outFields", not "__out": a leading-double-underscore identifier is
    // reserved to the implementation anyway, and "__out" specifically
    // collides with a legacy Windows SDK SAL annotation macro (from
    // sal.h/specstrings.h, pulled in transitively by MSVC's own standard
    // headers) that silently mangles the parameter list it appears in --
    // MSVC then reports nonsensical "syntax error '.'" at every later use
    // of the (macro-expanded-away) parameter, not at the macro site
    // itself, which made this one take a moment to place.
    out_ << tmpl_prefix;
    out_ << "inline void wasigo_reflect_describe(" << self_type
         << "* __v, std::vector<wasigo::FieldInfo>& outFields) {\n";
    for (auto& f : sd.fields) {
      if (f.embedded || f.name.empty() || !std::isupper(static_cast<unsigned char>(f.name[0]))) continue;
      std::string json_name = ReflectFieldName(f);
      if (json_name.empty()) continue;
      std::string fn = FieldCppName(sd.name, f.name);
      // Pointer fields stay adapt_ptr(field). Value fields use adapt_ptr(&field)
      // so reflect.Value.Set* writes through to the struct (JSON Unmarshal).
      if (f.type && f.type->kind == TypeKind::Pointer) {
        out_ << "  outFields.push_back({" << EscapeCppStringLiteral(json_name)
             << ", wasigo::Any::adapt_ptr(__v->" << fn << ")});\n";
      } else {
        out_ << "  outFields.push_back({" << EscapeCppStringLiteral(json_name)
             << ", wasigo::Any::adapt_ptr(&__v->" << fn << ")});\n";
      }
    }
    out_ << "}\n";
    out_ << tmpl_prefix;
    out_ << "inline const char* wasigo_reflect_typename(const " << self_type << "*) { return \""
         << (file_.package_name.empty() || file_.package_name == "main" ? sd.name
                                                                          : file_.package_name + "." + sd.name)
         << "\"; }\n\n";
  }

  void EmitResultStructDefs() {
    for (auto& fn : file_.funcs) {
      if (fn.results.size() <= 1) continue;
      out_ << "struct " << ResultStructName(fn) << " {\n";
      for (size_t i = 0; i < fn.results.size(); ++i) {
        out_ << "  " << CppType(fn.results[i].get()) << " r" << i << "{};\n";
      }
      out_ << "};\n\n";
    }
  }

  int64_t EvalConstI64(const Expr& e, int iota, int depth = 0) const {
    if (depth > 32) Error("const cycle");
    NoteLoc(e);
    switch (e.kind) {
      case ExprKind::IntLit:
        return e.intval;
      case ExprKind::Ident: {
        if (e.strval == "iota") return iota;
        int next_iota = 0;
        if (const Expr* init = LookupConstInit(e.strval, &next_iota)) {
          return EvalConstI64(*init, next_iota, depth + 1);
        }
        Error("const '" + e.strval + "' is not a compile-time integer (iota/literal)");
      }
      case ExprKind::ParenExpr:
        return EvalConstI64(*e.x, iota, depth + 1);
      case ExprKind::Unary:
        if (e.strval == "-") return -EvalConstI64(*e.x, iota, depth + 1);
        if (e.strval == "+") return EvalConstI64(*e.x, iota, depth + 1);
        if (e.strval == "^") return ~EvalConstI64(*e.x, iota, depth + 1);
        Error("unsupported unary operator in a const");
      case ExprKind::Binary: {
        int64_t l = EvalConstI64(*e.x, iota, depth + 1);
        int64_t r = EvalConstI64(*e.y, iota, depth + 1);
        if (e.strval == "+") return l + r;
        if (e.strval == "-") return l - r;
        if (e.strval == "*") return l * r;
        if (e.strval == "/") {
          if (r == 0) Error("division by zero in a const");
          return l / r;
        }
        if (e.strval == "%") {
          if (r == 0) Error("division by zero in a const");
          return l % r;
        }
        if (e.strval == "<<") {
          if (r < 0 || r >= 64) Error("invalid shift count in a const");
          return l << r;
        }
        if (e.strval == ">>") {
          if (r < 0 || r >= 64) Error("invalid shift count in a const");
          return l >> r;
        }
        if (e.strval == "&") return l & r;
        if (e.strval == "|") return l | r;
        if (e.strval == "^") return l ^ r;
        if (e.strval == "&^") return l & ~r;
        Error("unsupported operator '" + e.strval + "' in a const");
      }
      default:
        Error("const initializer is not a compile-time integer");
    }
  }

  int64_t EvalArrayLen(const ArrayLenExpr& e, int depth = 0) const {
    if (depth > 32) Error("const cycle in array length");
    switch (e.kind) {
      case ArrayLenExpr::Kind::Lit:
        if (e.lit < 0) Error("array length must be non-negative");
        return e.lit;
      case ArrayLenExpr::Kind::Ident: {
        if (e.ident == "iota") Error("iota is not a valid array length here");
        int next_iota = 0;
        const Expr* init = LookupConstInit(e.ident, &next_iota);
        if (!init) Error("array length '" + e.ident + "' is not a const integer");
        int64_t n = EvalConstI64(*init, next_iota, depth + 1);
        if (n < 0) Error("array length must be non-negative");
        return n;
      }
      case ArrayLenExpr::Kind::Unary: {
        if (!e.x) Error("array length must be a constant integer expression");
        int64_t v = EvalArrayLen(*e.x, depth + 1);
        if (e.op == "+") return v;
        if (e.op == "-") Error("array length must be non-negative");
        Error("unsupported unary operator in array length");
      }
      case ArrayLenExpr::Kind::Binary: {
        if (!e.x || !e.y) Error("array length must be a constant integer expression");
        int64_t l = EvalArrayLen(*e.x, depth + 1);
        int64_t r = EvalArrayLen(*e.y, depth + 1);
        if (e.op == "+") return l + r;
        if (e.op == "-") return l - r;
        if (e.op == "*") return l * r;
        if (e.op == "/") {
          if (r == 0) Error("division by zero in a const");
          return l / r;
        }
        if (e.op == "<<") {
          if (r < 0 || r >= 64) Error("invalid shift count in a const");
          return l << r;
        }
        if (e.op == ">>") {
          if (r < 0 || r >= 64) Error("invalid shift count in a const");
          return l >> r;
        }
        if (e.op == "&") return l & r;
        if (e.op == "|") return l | r;
        if (e.op == "^") return l ^ r;
        Error("unsupported operator '" + e.op + "' in array length");
      }
    }
    Error("array length must be a constant integer expression");
  }

  int64_t ResolvedArrayLen(const TypeNode* t) const {
    if (!t) Error("missing array type");
    if (t->array_len_expr) return EvalArrayLen(*t->array_len_expr);
    if (t->array_len < 0) Error("array length must be non-negative");
    return t->array_len;
  }

  bool ExprMentionsIota(const Expr* e) const {
    if (!e) return false;
    if (e->kind == ExprKind::Ident && e->strval == "iota") return true;
    return ExprMentionsIota(e->x.get()) || ExprMentionsIota(e->y.get());
  }

  // A plain numeric `const` (the common case: `const bufSize = 4096`, an
  // enum-shaped `const (...)` block, ...) doesn't depend on any struct
  // type, so it can -- and, for methods, must -- be emitted before struct
  // bodies: methods are emitted *inside* their struct (see EmitStructDefs),
  // which happens before EmitGlobalDecls in Run()'s pass order (globals
  // can reference struct types in their initializer, e.g. `var p =
  // Point{1,2}`, so globals can't simply move before structs instead). An
  // inline method body referencing a same-file package-level const would
  // otherwise reference a name not yet declared at that point in the
  // translation unit -- C++'s "complete-class context" early-visibility
  // rule only reaches other class *members*, not enclosing-namespace
  // declarations written later in the file.
  bool TryConstexprGlobal(const GlobalVarDecl& g, std::string& out_init) {
    if (!g.is_const || !g.init) return false;
    if (!(ExprMentionsIota(g.init.get()) || g.init->kind == ExprKind::IntLit ||
          g.init->kind == ExprKind::Binary || g.init->kind == ExprKind::Unary ||
          g.init->kind == ExprKind::ParenExpr)) {
      return false;
    }
    try {
      out_init = std::to_string(EvalConstI64(*g.init, g.iota_value)) + "LL";
      return true;
    } catch (const GenError&) {
      return false;
    }
  }

  void EmitSimpleConstDecls() {
    for (auto& g : file_.globals) {
      std::string init_str;
      if (!TryConstexprGlobal(g, init_str)) continue;
      const TypeNode* t = g.type ? g.type.get() : globals_[g.name];
      out_ << "constexpr " << CppType(t) << " " << CppIdent(g.name) << " = " << init_str << ";\n";
    }
    out_ << "\n";
  }

  // The rest of the globals -- anything TryConstexprGlobal doesn't cover,
  // e.g. `var Canceled = errors.New("context canceled")`, needs a runtime
  // call to initialize and so can't be constexpr -- still need to be at
  // least *declared* before struct bodies, for the exact same "an inline
  // method body referencing it" reason EmitSimpleConstDecls exists for
  // (see its comment). Unlike that one, this only forward-declares
  // (`extern`); the real definition (with its initializer) stays emitted
  // in EmitGlobalDecls's existing position, after struct bodies -- moving
  // the *definition* earlier is what the struct-typed-initializer ordering
  // constraint (`var p = Point{1, 2}`) rules out, but a bare declaration
  // doesn't need Point to be a complete type, only forward-declared (which
  // EmitStructForwardDecls already guarantees by this point).
  void EmitGlobalForwardDecls() {
    for (auto& g : file_.globals) {
      std::string probe_init;
      if (TryConstexprGlobal(g, probe_init)) continue;  // already a full definition, see EmitSimpleConstDecls
      const TypeNode* t = g.type ? g.type.get() : globals_[g.name];
      out_ << "extern " << (g.is_const ? "const " : "") << CppType(t) << " " << CppIdent(g.name) << ";\n";
    }
    out_ << "\n";
  }

  void EmitGlobalDecls() {
    for (auto& g : file_.globals) {
      std::string probe_init;
      if (TryConstexprGlobal(g, probe_init)) continue;  // already emitted, see EmitSimpleConstDecls
      const TypeNode* t = g.type ? g.type.get() : globals_[g.name];
      std::string init_str;
      if (g.init) init_str = EmitExprAs(*g.init, t);
      out_ << (g.is_const ? "const " : "") << CppType(t) << " " << CppIdent(g.name);
      if (g.init) {
        out_ << " = " << init_str;
      } else {
        out_ << "{}";
      }
      out_ << ";\n";
    }
    out_ << "\n";
  }

  void EmitFreeFuncProtosOrSkipMain() {}

  std::string TemplatePrefixFrom(const std::vector<std::string>& tparams) {
    if (tparams.empty()) return "";
    std::ostringstream oss;
    oss << "template<";
    for (size_t i = 0; i < tparams.size(); ++i) {
      if (i) oss << ", ";
      oss << "typename " << tparams[i];
    }
    oss << ">\n";
    return oss.str();
  }

  std::string TemplatePrefix(const FuncDecl& fn) {
    return TemplatePrefixFrom(fn.type_params);
  }

  void EmitAliases() {
    auto saved_tp = current_type_params_;
    for (auto& a : file_.aliases) {
      current_type_params_ = a.type_params;
      if (!a.type_params.empty()) out_ << TemplatePrefixFrom(a.type_params);
      if (!a.is_alias_eq && HasMethodsOn(a.name)) {
        std::string under = CppType(a.type.get());
        out_ << "struct " << a.name << " {\n";
        out_ << "  " << under << " v{};\n";
        out_ << "  constexpr " << a.name << "() = default;\n";
        out_ << "  constexpr " << a.name << "(" << under << " x) : v(x) {}\n";
        out_ << "  constexpr operator " << under << "() const { return v; }\n";
        // reflect.TypeOf(...).Kind() -- has_reflect_kind_override in
        // runtime.hpp reads this back. A defined type with methods
        // wrapping []T/[N]T/map[K]V is the only case that gets a real,
        // distinct wrapper struct to hang this on (see the comment
        // there); a plain named type with no underlying container kind
        // (`type Celsius float64`) has nothing more specific than the
        // underlying's own Kind to report, so no override is emitted
        // and kind_of<T> falls through to that below as before.
        const TypeNode* under_resolved = ResolveUnderlying(a.type.get());
        if (under_resolved && (under_resolved->kind == TypeKind::Slice ||
                                under_resolved->kind == TypeKind::Array ||
                                under_resolved->kind == TypeKind::Map)) {
          const char* rk = under_resolved->kind == TypeKind::Slice   ? "Slice"
                            : under_resolved->kind == TypeKind::Array ? "Array"
                                                                       : "Map";
          out_ << "  static constexpr int wasigo_reflect_kind = static_cast<int>(wasigo::RKind::"
               << rk << ");\n";
        }
        for (auto& fn : file_.funcs) {
          if (fn.has_receiver && fn.receiver_type == a.name) EmitMethodDecl(fn);
        }
        out_ << "};\n";
        // reflect.TypeOf(...).Name() -- same has_reflect_typename ADL
        // trait EmitReflectDescribe wires up for ordinary structs;
        // this wrapper struct never went through that path at all
        // before, so Name() silently came back "" for every defined
        // type with methods, not just container-underlying ones.
        out_ << "inline const char* wasigo_reflect_typename(const " << SelfTypeNameForAlias(a)
             << "*) { return \""
             << (file_.package_name.empty() || file_.package_name == "main"
                     ? a.name
                     : file_.package_name + "." + a.name)
             << "\"; }\n";
      } else {
        out_ << "using " << a.name << " = " << CppType(a.type.get()) << ";\n";
      }
    }
    current_type_params_ = saved_tp;
    if (!file_.aliases.empty()) out_ << "\n";
  }

  // Same shape as SelfTypeName(const StructDecl&) but for a defined-
  // type alias's own wrapper struct (TypeAlias has no such helper yet).
  std::string SelfTypeNameForAlias(const TypeAlias& a) const {
    std::string n = a.name;
    if (!a.type_params.empty()) {
      n += "<";
      for (size_t i = 0; i < a.type_params.size(); ++i) {
        if (i) n += ", ";
        n += a.type_params[i];
      }
      n += ">";
    }
    return n;
  }

  void EmitPackageInit() {
    out_ << "void __wasigo_init() {\n";
    indent_ = 1;
    for (auto& fn : file_.funcs) {
      if (!IsInitFunc(fn)) continue;
      if (!fn.params.empty() || !fn.results.empty()) {
        Error("func init must have no parameters and no results");
      }
      if (IsAsyncFree("init")) {
        Error("func init cannot use channels/select (it is not a goroutine)");
      }
      PushScope();
      const FuncDecl* saved = current_func_;
      current_func_ = &fn;
      bool saved_async = current_async_;
      current_async_ = false;
      out_ << Indent() << "{\n";
      indent_++;
      EmitWrappedBody(fn.body, false, false);
      indent_--;
      out_ << Indent() << "}\n";
      current_async_ = saved_async;
      current_func_ = saved;
      PopScope();
    }
    indent_ = 0;
    out_ << "}\n\n";
  }

  void EmitFreeFuncDefs() {
    for (auto& fn : file_.funcs) {
      if (fn.has_receiver) continue;
      if (IsInitFunc(fn)) continue;
      bool is_main = fn.name == "main";
      bool async = IsAsyncFree(fn.name);
      const FuncDecl* saved = current_func_;
      current_func_ = &fn;
      bool saved_async = current_async_;
      current_async_ = async;

      if (is_main && async) {
        out_ << "wasigo::Task __wasigo_main() {\n";
        indent_ = 1;
        PushScope();
        EmitStartupInits();
        EmitWrappedBody(fn.body, true, false);
        PopScope();
        indent_ = 0;
        out_ << "}\n\n"
             << "int main(int argc, char** argv) {\n"
             << "  wasigo::set_os_args(argc, argv);\n"
             << "  return wasigo::run(__wasigo_main());\n"
             << "}\n\n";
        current_async_ = saved_async;
        current_func_ = saved;
        continue;
      }

      out_ << TemplatePrefix(fn);
      if (is_main) {
        out_ << "int main(int argc, char** argv) {\n";
        indent_ = 1;
        PushScope();
        out_ << Indent() << "wasigo::set_os_args(argc, argv);\n";
        for (auto& p : fn.params) Declare(p.name, ParamGoType(p));
        EmitWrappedBody(fn.body, async && !is_main, is_main);
        PopScope();
        indent_ = 0;
        out_ << "}\n\n";
        current_async_ = saved_async;
        current_func_ = saved;
        continue;
      }
      out_ << FuncCppType(fn, async) << " " << fn.name << "(";
      for (size_t i = 0; i < fn.params.size(); ++i) {
        if (i) out_ << ", ";
        out_ << ParamCppType(fn.params[i]) << " " << CppIdent(fn.params[i].name);
      }
      out_ << ") {\n";
      indent_ = 1;
      PushScope();
      for (auto& p : fn.params) Declare(p.name, ParamGoType(p));
      EmitWrappedBody(fn.body, async && !is_main, is_main);
      PopScope();
      indent_ = 0;
      out_ << "}\n\n";
      current_async_ = saved_async;
      current_func_ = saved;
    }
  }

  std::string IfaceMethodReturn(const MethodSig& m) const {
    if (m.results.size() > 1) return m.name + "_result";
    if (m.results.size() == 1) return CppType(m.results[0].get());
    return "void";
  }

  void EmitIfaceVTableLambda(const MethodSig& m, const std::string& cast_t) {
    // The self pointer needs a name distinct from any real Go parameter
    // name the method might use (e.g. `WriteString(s string)`) -- "__self"
    // matches this file's other internal-temporary convention (temp_id_'s
    // "__t0", EmitCompositeLit's "__s", ...), which are never valid Go
    // identifiers a source method could shadow.
    std::string r = IfaceMethodReturn(m);
    out_ << "      [](void* __self";
    for (auto& p : m.params) out_ << ", " << CppType(p.type.get()) << " " << CppIdent(p.name);
    out_ << ") { ";
    if (m.results.size() > 1) {
      out_ << "auto __m = static_cast<" << cast_t << ">(__self)->" << m.name << "(";
      for (size_t i = 0; i < m.params.size(); ++i) {
        if (i) out_ << ", ";
        out_ << CppIdent(m.params[i].name);
      }
      out_ << "); " << r << " __o{}; ";
      for (size_t i = 0; i < m.results.size(); ++i) {
        out_ << "__o.r" << i << " = __m.r" << i << "; ";
      }
      out_ << "return __o; },\n";
      return;
    }
    out_ << (r == "void" ? "" : "return ") << "static_cast<" << cast_t << ">(__self)->" << m.name << "(";
    for (size_t i = 0; i < m.params.size(); ++i) {
      if (i) out_ << ", ";
      out_ << CppIdent(m.params[i].name);
    }
    out_ << "); },\n";
  }

  void EmitInterfaceDefs() {
    // Forward-declare every interface in the package before any interface
    // body: an interface method with 2+ results gets a nested `_result`
    // struct (below) whose fields are plain VALUE members, and when one
    // of those results is ANOTHER interface declared later in the same
    // package (database/sql/driver's `Driver.Open(name string) (Conn,
    // error)`, `Conn` not yet even forward-declared at that point) that
    // field's type "does not name a type" at all -- not just incomplete.
    // A single-return method returning another interface already worked
    // (goes through `IfaceMethodReturn`, no `_result` struct involved),
    // so this gap was specific to the multi-return + interface-result
    // combination, never exercised before this package (every earlier
    // multi-return interface method -- image/color's `Color.RGBA()
    // (r,g,b,a uint32)` -- only ever used builtin result fields).
    for (auto& id : file_.interfaces) {
      out_ << "struct " << id.name << ";\n";
    }
    // A forward declaration alone isn't enough for the `_result` struct
    // fields themselves (a plain VALUE member needs its type COMPLETE,
    // not just declared) -- so the full interface BODIES below must be
    // emitted in dependency order (an interface referencing another as a
    // multi-return result needs that other interface's body already
    // emitted), not file order (real Go doesn't care about declaration
    // order at all, so nothing enforces this in the source). A simple
    // DFS post-order topological sort over "which other LOCAL interfaces
    // does this one name as a method result type" gets that order; a
    // genuine cycle (impossible to lay out linearly in C++ this way) is a
    // hard error rather than an infinite loop or silently wrong output.
    std::vector<const InterfaceDecl*> iface_order;
    {
      std::unordered_map<std::string, bool> visited, visiting;
      std::function<void(const InterfaceDecl*)> visit = [&](const InterfaceDecl* id_ptr) {
        if (visited[id_ptr->name]) return;
        if (visiting[id_ptr->name]) {
          Error("interface dependency cycle involving '" + id_ptr->name + "'");
        }
        visiting[id_ptr->name] = true;
        for (auto* m : FlattenIfaceMethods(id_ptr)) {
          for (auto& r : m->results) {
            if (!r || r->kind != TypeKind::Named) continue;
            for (auto& other : file_.interfaces) {
              if (other.name == r->name && &other != id_ptr) visit(&other);
            }
          }
        }
        visiting[id_ptr->name] = false;
        visited[id_ptr->name] = true;
        iface_order.push_back(id_ptr);
      };
      for (auto& id : file_.interfaces) visit(&id);
    }
    for (auto* id_ptr : iface_order) {
      auto& id = *id_ptr;
      auto methods = FlattenIfaceMethods(&id);
      out_ << "struct " << id.name << " {\n";
      for (auto* m : methods) {
        if (m->results.size() <= 1) continue;
        out_ << "  struct " << m->name << "_result {\n";
        for (size_t i = 0; i < m->results.size(); ++i) {
          out_ << "    " << CppType(m->results[i].get()) << " r" << i << "{};\n";
        }
        out_ << "  };\n";
      }
      out_ << "  struct VTable {\n";
      for (auto* m : methods) {
        std::string r = IfaceMethodReturn(*m);
        out_ << "    " << r << " (*" << m->name << ")(void*";
        for (auto& p : m->params) out_ << ", " << CppType(p.type.get());
        out_ << ");\n";
      }
      out_ << "  };\n";
      out_ << "  std::shared_ptr<void> self;\n";
      out_ << "  const VTable* vt = nullptr;\n";
      out_ << "  const void* type_key = nullptr;\n";
      out_ << "  bool is_nil() const { return !vt; }\n";
      for (auto* m : methods) {
        std::string r = IfaceMethodReturn(*m);
        // Declaration only here (no body) -- see deferred_iface_method_defs_'s
        // comment for why the body can't be inline in the class the way
        // every OTHER inline method in this generator is.
        out_ << "  " << r << " " << m->name << "(";
        for (size_t i = 0; i < m->params.size(); ++i) {
          if (i) out_ << ", ";
          out_ << CppType(m->params[i].type.get()) << " " << CppIdent(m->params[i].name);
        }
        out_ << ") const;\n";
        // `r` is `id.name`'s own nested `<Method>_result` struct when a
        // method has 2+ results (see the `_result` struct block just
        // above) -- fine unqualified INSIDE the class (the declaration
        // just emitted above), but out-of-line at namespace scope it
        // needs the enclosing interface name, or it doesn't resolve at
        // all: found alongside the interface/struct-completeness bug
        // this whole deferred-body split exists for, once a multi-result
        // interface method (image/color's `Color.RGBA() (r, g, b, a
        // uint32)`) exercised this specific combination for the first
        // time.
        std::string out_of_line_r = m->results.size() > 1 ? id.name + "::" + r : r;
        deferred_iface_method_defs_ << out_of_line_r << " " << id.name << "::" << m->name << "(";
        for (size_t i = 0; i < m->params.size(); ++i) {
          if (i) deferred_iface_method_defs_ << ", ";
          deferred_iface_method_defs_ << CppType(m->params[i].type.get()) << " " << CppIdent(m->params[i].name);
        }
        deferred_iface_method_defs_ << ") const {\n";
        deferred_iface_method_defs_ << "  if (!vt) wasigo::panic(\"nil interface\");\n";
        deferred_iface_method_defs_ << "  " << (r == "void" ? "" : "return ") << "vt->" << m->name << "(self.get()";
        for (auto& p : m->params) deferred_iface_method_defs_ << ", " << CppIdent(p.name);
        deferred_iface_method_defs_ << ");\n";
        deferred_iface_method_defs_ << "}\n";
      }
      out_ << "  template<class T>\n";
      out_ << "  T must_cast() const { return wasigo::iface_must_cast<T>(self, type_key); }\n";
      out_ << "  template<class T>\n";
      out_ << "  std::pair<T, bool> try_cast() const { return wasigo::iface_try_cast<T>(self, type_key); }\n";
      out_ << "  template<class T>\n";
      out_ << "  static " << id.name << " adapt(T v) {\n";
      out_ << "    static const VTable kvt{\n";
      for (auto* m : methods) EmitIfaceVTableLambda(*m, "T*");
      out_ << "    };\n";
      out_ << "    " << id.name << " i;\n";
      out_ << "    i.self = std::make_shared<T>(std::move(v));\n";
      out_ << "    i.vt = &kvt;\n";
      out_ << "    i.type_key = wasigo::type_key_of<T>();\n";
      out_ << "    return i;\n";
      out_ << "  }\n";
      out_ << "  template<class T>\n";
      out_ << "  static " << id.name << " adapt_ptr(T* v) {\n";
      out_ << "    if (!v) return {};\n";
      out_ << "    static const VTable kvt{\n";
      for (auto* m : methods) EmitIfaceVTableLambda(*m, "T*");
      out_ << "    };\n";
      out_ << "    " << id.name << " i;\n";
      out_ << "    i.self = std::shared_ptr<void>(static_cast<void*>(v), [](void*) {});\n";
      out_ << "    i.vt = &kvt;\n";
      out_ << "    i.type_key = wasigo::type_key_of<T*>();\n";
      out_ << "    return i;\n";
      out_ << "  }\n";
      out_ << "};\n";
      if (opt_.library && !file_.package_name.empty() && file_.package_name != "main") {
        EmitNsClose();
        out_ << "namespace wasigo {\ninline bool is_nil(const "
             << NamespacePrefix(file_.package_name) << id.name
             << "& i) { return i.is_nil(); }\n}\n";
        EmitNsOpen();
      } else {
        out_ << "namespace wasigo {\ninline bool is_nil(const " << id.name
             << "& i) { return i.is_nil(); }\n}\n\n";
      }
    }
  }

  // An interface method whose signature uses a plain STRUCT type (not a
  // pointer, not a builtin, not another interface) -- e.g. `Bounds()
  // Rectangle` in image.Image -- needs that struct COMPLETE, not just
  // forward-declared, at the point its forwarding method's BODY returns a
  // value of that type: C++'s "complete-class context" only defers a
  // class's own inline method bodies to right after ITS closing brace,
  // not all the way to end-of-TU, so emitting `Rectangle Bounds() const {
  // ...; return vt->Bounds(...); }` inline inside `struct Image` (which
  // is emitted, like every interface, near the top of the file, well
  // before EmitStructDefs) fails with "incomplete type" the moment any
  // interface method involves a plain struct instead of only builtins/
  // pointers/other interfaces -- found building image.Image (every
  // earlier interface -- hash.Hash, cipher.Block, color.Model -- only
  // ever used builtins or other interfaces in its methods). Simply
  // moving struct definitions before interface definitions isn't safe
  // either: a struct's OWN methods commonly adapt themselves TO an
  // interface (`return SomeConcreteValue` where the declared result is
  // an interface, going through `Iface::adapt<T>(...)`), which needs the
  // interface struct (and its `adapt` static method) already declared --
  // a real two-way dependency, not fixable by reordering whole blocks.
  // The fix: keep the interface DECLARATION (VTable, adapt<T> templates,
  // fields) emitted early as before -- those only ever need forward-
  // declared types, proven by the fact structs are only forward-declared
  // at that point already -- but defer each forwarding method's BODY to
  // an out-of-line definition (`ReturnType Iface::Method(...) const {
  // ... }`), buffered in `deferred_iface_method_defs_` and flushed here,
  // AFTER EmitStructDefs has made every struct (Rectangle included)
  // complete.
  void FlushDeferredIfaceMethodDefs() {
    out_ << deferred_iface_method_defs_.str();
    deferred_iface_method_defs_.str("");
  }
};

}  // namespace

std::string GenerateCpp(const File& file, const GenOptions& opt) {
  Generator g(file, opt);
  return g.Run();
}

}  // namespace wasigo
